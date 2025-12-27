package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Simplifier320 converts modern HTML5 to HTML 3.2-ish compatible format (best effort, 1-pass tree transform)
type Simplifier320 struct {
	// Options
	WrapBody             bool // wrap body with <center><table ...>
	SkipIfAlreadyWrapped bool // if body already has our wrapper, don't wrap again
	ConvertButtons       bool // <button> -> <input type="submit" value="...">
	DropLinkTargets      bool // remove target/rel on <a>
	KeepImgDims          bool // keep width/height on <img> when parseable
	DropImgLazy          bool // remove loading/decoding/fetchpriority/etc (also handled by attr filter)
	ForceHTTP            bool // https -> http on converted urls
}

func NewSimplifier320() *Simplifier320 {
	return &Simplifier320{
		WrapBody:             true,
		SkipIfAlreadyWrapped: true,
		ConvertButtons:       true,
		DropLinkTargets:      true,
		KeepImgDims:          true,
		DropImgLazy:          true,
		ForceHTTP:            true,
	}
}

// Simplify converts HTML5 to HTML 3.2 compatible format (high-performance net/html version)
func (s *Simplifier320) Simplify(inputHTML string, pageURL string, debugMode bool) (string, error) {
	doc, err := html.Parse(strings.NewReader(inputHTML))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	parsedPage, _ := url.Parse(pageURL) // best effort
	canResolve := parsedPage != nil && parsedPage.Scheme != "" && parsedPage.Host != ""

	// Remove existing doctype nodes (we will prepend our own)
	removeAllDoctypes(doc)

	// 1) Walk & transform (single DFS)
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling

			if c.Type == html.ElementNode {
				// === Removals (fast-path) ===
				if shouldDropElement(c) {
					n.RemoveChild(c)
					c = next
					continue
				}

				// Head modern removals
				if c.Data == "meta" && shouldDropMeta(c) {
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "link" && shouldDropLink(c) {
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "base" {
					n.RemoveChild(c)
					c = next
					continue
				}

				// === Structural conversions ===

				// <picture> -> first <img>
				if c.Data == "picture" {
					img := findFirstElementDesc(c, "img")
					if img != nil {
						detachNode(img)
						n.InsertBefore(img, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}
				// <source> -> remove
				if c.Data == "source" {
					n.RemoveChild(c)
					c = next
					continue
				}

				// video/audio/iframe -> link block
				if c.Data == "video" || c.Data == "audio" || c.Data == "iframe" {
					rep := mediaToLinkBlock(c)
					if rep != nil {
						// resolve URL on generated <a href> later (we'll do it now too)
						if canResolve {
							convertURLsOnNode(rep, parsedPage, s.ForceHTTP, debugMode)
						}
						n.InsertBefore(rep, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}

				// button -> input submit (optional)
				if s.ConvertButtons && c.Data == "button" {
					rep := buttonToSubmitInput(c)
					if rep != nil {
						n.InsertBefore(rep, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}

				// === Tag rewrites (semantic -> div etc.) ===
				rewriteTagName(c)

				// === Attribute filtering / form simplification / URLs / special per-tag cleanup ===
				filterAttrsInPlace(c, s)

				simplifyFormsInPlace(c)

				// Per-tag rules
				if s.DropLinkTargets && c.Data == "a" {
					// strip target/rel (and other modern-only attrs if any slipped through)
					removeAttrLower(c, "target")
					removeAttrLower(c, "rel")
					removeAttrLower(c, "referrerpolicy")
					removeAttrLower(c, "ping")
					removeAttrLower(c, "download") // optional: can keep if you want
				}
				if c.Data == "img" {
					normalizeImgAttrs(c, s)
				}

				// Convert URLs on this node (href/src/action) (best effort)
				if canResolve {
					convertURLsOnNode(c, parsedPage, s.ForceHTTP, debugMode)
				}

				// Make <html ...> -> <html> (strip attrs)
				if c.Data == "html" && len(c.Attr) > 0 {
					c.Attr = c.Attr[:0]
				}
				// (Optional) make <body> attrs minimal: we’ll set legacy colors later
			}

			walk(c)
			c = next
		}
	}
	walk(doc)

	// 2) Wrap body content in legacy table + set body colors
	if body := findFirstElementDesc(doc, "body"); body != nil {
		if s.WrapBody {
			if !s.SkipIfAlreadyWrapped || !alreadyWrapped(body) {
				wrapBodyWithCenterTable(body)
			}
		}
		setBodyLegacyColors(body)
	}

	// 3) Render
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render HTML: %w", err)
	}

	// 4) Prepend HTML 3.2 doctype (best effort)
	doctype := "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\">\n"
	return doctype + buf.String(), nil
}

/* ----------------------------- rules / helpers ----------------------------- */

func shouldDropElement(n *html.Node) bool {
	switch n.Data {
	// CSS / JS
	case "style", "script", "noscript":
		return true
	// Unsupported / modern media-heavy
	case "canvas", "svg", "object", "embed", "applet":
		return true
	// HTML5 widgets (drop entirely)
	case "progress", "meter", "output", "datalist", "dialog", "template", "slot":
		return true
	default:
		return false
	}
}

func shouldDropMeta(n *html.Node) bool {
	if n.Data != "meta" {
		return false
	}
	// Drop viewport, charset
	if hasAttrLower(n, "charset") {
		return true
	}
	if strings.EqualFold(getAttrLower(n, "name"), "viewport") {
		return true
	}
	return false
}

func shouldDropLink(n *html.Node) bool {
	if n.Data != "link" {
		return false
	}
	rel := strings.ToLower(strings.TrimSpace(getAttrLower(n, "rel")))
	if rel == "" {
		return false
	}
	toks := strings.Fields(rel)
	for _, t := range toks {
		switch t {
		case "stylesheet", "preload", "prefetch", "modulepreload", "icon", "manifest":
			return true
		}
	}
	return false
}

func rewriteTagName(n *html.Node) {
	switch n.Data {
	case "header", "nav", "main", "section", "article", "aside", "footer", "figure", "details", "summary":
		n.Data = "div"
		return
	case "figcaption":
		n.Data = "small"
		return
	case "time", "mark":
		n.Data = "span"
		return
	}
}

/* ------------------------------ attrs filter ------------------------------- */

var removeExact = map[string]struct{}{
	// presentation
	"style": {}, "class": {},
	// modern / perf / a11y
	"role": {}, "fetchpriority": {},
	// images
	"srcset": {}, "sizes": {},
	"decoding": {}, // modern
	// misc
	"inert": {},
}

var removePrefixes = []string{"data-", "aria-", "on"}

func filterAttrsInPlace(n *html.Node, s *Simplifier320) {
	if n.Type != html.ElementNode || len(n.Attr) == 0 {
		return
	}

	// add some optional drops via options
	// (loading is common on img/iframe)
	// We'll implement via checks inside filter loop.
	dst := n.Attr[:0]

	for _, a := range n.Attr {
		k := strings.ToLower(a.Key)

		// exact
		if _, ok := removeExact[k]; ok {
			continue
		}

		// prefix
		drop := false
		for _, p := range removePrefixes {
			if strings.HasPrefix(k, p) {
				drop = true
				break
			}
		}
		if drop {
			continue
		}

		// option-based
		if s.DropImgLazy {
			if k == "loading" || k == "fetchpriority" {
				continue
			}
		}

		// Modern form attrs (even if not form element, harmless to drop)
		switch k {
		case "placeholder", "required", "pattern", "min", "max", "step", "autocomplete", "autofocus", "form":
			continue
		}

		dst = append(dst, a)
	}
	n.Attr = dst
}

/* --------------------------- forms simplification -------------------------- */

var modernInputTypes = map[string]struct{}{
	"email": {}, "url": {}, "number": {}, "date": {}, "datetime-local": {},
	"time": {}, "week": {}, "month": {}, "search": {}, "tel": {},
	"color": {}, "range": {},
}

func simplifyFormsInPlace(n *html.Node) {
	if n.Type != html.ElementNode {
		return
	}
	if n.Data == "input" {
		t := strings.ToLower(getAttrLower(n, "type"))
		if t == "" {
			return
		}
		if _, ok := modernInputTypes[t]; ok {
			setAttrLower(n, "type", "text")
		}
	}
}

/* --------------------------- per-tag normalization ------------------------- */

func normalizeImgAttrs(img *html.Node, s *Simplifier320) {
	// If width/height are non-numeric or percent, drop them (legacy browsers vary)
	// If KeepImgDims is true, keep positive integer dims.
	if !s.KeepImgDims {
		removeAttrLower(img, "width")
		removeAttrLower(img, "height")
		return
	}

	for _, key := range []string{"width", "height"} {
		v := strings.TrimSpace(getAttrLower(img, key))
		if v == "" {
			continue
		}
		// allow only positive int
		if strings.Contains(v, "%") {
			removeAttrLower(img, key)
			continue
		}
		if _, err := strconv.Atoi(v); err != nil {
			// sometimes "640px"
			v2 := strings.TrimSuffix(v, "px")
			if _, err2 := strconv.Atoi(v2); err2 != nil {
				removeAttrLower(img, key)
			} else {
				setAttrLower(img, key, v2)
			}
		}
	}

	// alt/title are fine; keep
	// border is useful for legacy “linked image” look sometimes; leave as is if present
}

/* ------------------------------ URL conversion ----------------------------- */

func convertURLsOnNode(n *html.Node, pageBase *url.URL, forceHTTP bool, debugMode bool) {
	for i := range n.Attr {
		k := strings.ToLower(n.Attr[i].Key)
		if k != "href" && k != "src" && k != "action" {
			continue
		}
		val := strings.TrimSpace(n.Attr[i].Val)
		if val == "" {
			continue
		}
		abs := makeAbsoluteURL(val, pageBase)
		if forceHTTP {
			abs = httpsToHttp(abs)
		}
		n.Attr[i].Val = abs

		// Debug Mode: Rewrite href links to go through debug viewer
		if debugMode && k == "href" {
			// Skip anchors, javascript, empty
			if strings.HasPrefix(val, "#") || strings.EqualFold(val, "javascript:") {
				continue
			}
			// Skip if already a debug link (shouldn't happen in clean parse, but safe to check)
			if strings.HasPrefix(abs, "/_drp") || strings.HasPrefix(abs, "/debug") {
				continue
			}

			// Wrap in debug viewer URL
			n.Attr[i].Val = "/debug?url=" + url.QueryEscape(abs)
		}
	}
}

func makeAbsoluteURL(href string, pageBase *url.URL) string {
	href = strings.TrimSpace(href)
	l := strings.ToLower(href)
	if href == "" ||
		strings.HasPrefix(href, "#") ||
		strings.HasPrefix(l, "javascript:") ||
		strings.HasPrefix(l, "mailto:") ||
		strings.HasPrefix(l, "tel:") ||
		strings.HasPrefix(l, "data:") {
		return href
	}
	// protocol-relative
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}
	// already absolute
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	u, err := url.Parse(href)
	if err != nil || pageBase == nil {
		return href
	}
	return pageBase.ResolveReference(u).String()
}

func httpsToHttp(u string) string {
	if strings.HasPrefix(u, "https://") {
		return "http://" + u[len("https://"):]
	}
	return u
}

/* ---------------------- media -> link block conversion --------------------- */

func mediaToLinkBlock(media *html.Node) *html.Node {
	tag := media.Data

	src := strings.TrimSpace(getAttrLower(media, "src"))
	if src == "" {
		// try <source src="...">
		if s := findFirstElementDesc(media, "source"); s != nil {
			src = strings.TrimSpace(getAttrLower(s, "src"))
		}
	}
	if src == "" {
		return nil
	}

	// <p>[Video: <a href="...">Open</a>]</p>
	p := el("p")
	p.AppendChild(txt("["))
	p.AppendChild(txt(cap1(tag)))
	p.AppendChild(txt(": "))

	a := el("a")
	setAttrLower(a, "href", src)
	a.AppendChild(txt("Open"))
	p.AppendChild(a)

	p.AppendChild(txt("]"))
	return p
}

func buttonToSubmitInput(btn *html.Node) *html.Node {
	// Use button text (flattened)
	label := strings.TrimSpace(extractText(btn))
	if label == "" {
		label = "Submit"
	}

	in := el("input")
	setAttrLower(in, "type", "submit")
	setAttrLower(in, "value", label)

	// Preserve name/value if present (helps forms)
	if name := strings.TrimSpace(getAttrLower(btn, "name")); name != "" {
		setAttrLower(in, "name", name)
	}
	if val := strings.TrimSpace(getAttrLower(btn, "value")); val != "" {
		// if original has a specific value, prefer it
		setAttrLower(in, "value", val)
	}
	return in
}

func extractText(n *html.Node) string {
	var b strings.Builder
	var dfs func(x *html.Node)
	dfs = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			b.WriteString(" ")
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

/* ------------------------- body wrapping / styling ------------------------- */

func alreadyWrapped(body *html.Node) bool {
	// Detect: body has a single <center> child containing a <table width="95%" ...>
	// (best effort; won't false-positive too much)
	firstEl := firstElementChild(body)
	if firstEl == nil || strings.ToLower(firstEl.Data) != "center" {
		return false
	}
	tbl := firstElementChild(firstEl)
	if tbl == nil || strings.ToLower(tbl.Data) != "table" {
		return false
	}
	w := strings.TrimSpace(getAttrLower(tbl, "width"))
	return w == "95%"
}

func wrapBodyWithCenterTable(body *html.Node) {
	// Build: <center><table width="95%" border="0" cellspacing="0" cellpadding="5"><tr><td>...old children...</td></tr></table></center>
	center := el("center")
	table := el("table")
	setAttrLower(table, "width", "95%")
	setAttrLower(table, "border", "0")
	setAttrLower(table, "cellspacing", "0")
	setAttrLower(table, "cellpadding", "5")

	tr := el("tr")
	td := el("td")

	// Move all existing body children into td
	for c := body.FirstChild; c != nil; {
		next := c.NextSibling
		detachNode(c)
		td.AppendChild(c)
		c = next
	}

	tr.AppendChild(td)
	table.AppendChild(tr)
	center.AppendChild(table)

	body.AppendChild(center)
}

func setBodyLegacyColors(body *html.Node) {
	setAttrLower(body, "bgcolor", "#ffffff")
	setAttrLower(body, "text", "#000000")
	setAttrLower(body, "link", "#0000ee")
	setAttrLower(body, "vlink", "#551a8b")
	setAttrLower(body, "alink", "#ff0000")
}

/* ------------------------------ DOM utilities ------------------------------ */

func el(name string) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: strings.ToLower(name)}
}

func txt(s string) *html.Node {
	return &html.Node{Type: html.TextNode, Data: s}
}

func getAttrLower(n *html.Node, key string) string {
	key = strings.ToLower(key)
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == key {
			return a.Val
		}
	}
	return ""
}

func hasAttrLower(n *html.Node, key string) bool {
	key = strings.ToLower(key)
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == key {
			return true
		}
	}
	return false
}

func setAttrLower(n *html.Node, key, val string) {
	key = strings.ToLower(key)
	for i := range n.Attr {
		if strings.ToLower(n.Attr[i].Key) == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

func removeAttrLower(n *html.Node, key string) {
	key = strings.ToLower(key)
	if len(n.Attr) == 0 {
		return
	}
	dst := n.Attr[:0]
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == key {
			continue
		}
		dst = append(dst, a)
	}
	n.Attr = dst
}

func detachNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}

func findFirstElementDesc(root *html.Node, tag string) *html.Node {
	tag = strings.ToLower(tag)
	var found *html.Node
	var dfs func(n *html.Node)
	dfs = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == tag {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
			if found != nil {
				return
			}
		}
	}
	dfs(root)
	return found
}

func firstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

func removeAllDoctypes(root *html.Node) {
	var dfs func(n *html.Node)
	dfs = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.DoctypeNode {
				n.RemoveChild(c)
			} else {
				dfs(c)
			}
			c = next
		}
	}
	dfs(root)
}
