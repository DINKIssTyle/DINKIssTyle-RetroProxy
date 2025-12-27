package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Simplifier401 converts modern HTML5 to HTML 4.01 compatible format
type Simplifier401 struct {
	// Options
	WrapBody        bool // Default false for 4.01 (div layout preferred)
	ConvertButtons  bool // <button> -> <input type="submit" value="...">
	DropLinkTargets bool // remove target/rel on <a>
	KeepImgDims     bool // keep width/height on <img> when parseable
	DropImgLazy     bool // remove loading/decoding/fetchpriority
	ForceHTTP       bool // https -> http on converted urls
}

func NewSimplifier401() *Simplifier401 {
	return &Simplifier401{
		WrapBody:        false,
		ConvertButtons:  true,
		DropLinkTargets: true,
		KeepImgDims:     true,
		DropImgLazy:     true,
		ForceHTTP:       true,
	}
}

// Simplify converts HTML5 to HTML 4.01 compatible format
func (s *Simplifier401) Simplify(inputHTML string, pageURL string, debugMode bool) (string, error) {
	doc, err := html.Parse(strings.NewReader(inputHTML))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	parsedPage, _ := url.Parse(pageURL)
	canResolve := parsedPage != nil && parsedPage.Scheme != "" && parsedPage.Host != ""

	// Remove existing doctypes
	removeAllDoctypes401(doc)

	// DFS
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode {
				// Drop Check
				if shouldDropElement401(c) {
					n.RemoveChild(c)
					c = next
					continue
				}
				// Meta/Link drops
				if c.Data == "meta" && shouldDropMeta401(c) {
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "link" && shouldDropLink401(c) {
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "base" {
					n.RemoveChild(c)
					c = next
					continue
				}

				// Structural Conversions
				if c.Data == "picture" {
					img := findFirstElementDesc401(c, "img")
					if img != nil {
						detachNode401(img)
						n.InsertBefore(img, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "source" {
					n.RemoveChild(c)
					c = next
					continue
				}
				if c.Data == "video" || c.Data == "audio" || c.Data == "iframe" {
					rep := mediaToLinkBlock401(c)
					if rep != nil {
						if canResolve {
							convertURLsOnNode401(rep, parsedPage, s.ForceHTTP, debugMode)
						}
						n.InsertBefore(rep, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}

				if s.ConvertButtons && c.Data == "button" {
					rep := buttonToSubmitInput401(c)
					if rep != nil {
						n.InsertBefore(rep, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}

				// Tag Rewrite
				rewriteTagName401(c)

				// Attr Filter
				filterAttrsInPlace401(c, s)
				simplifyFormsInPlace401(c)

				if s.DropLinkTargets && c.Data == "a" {
					removeAttrLower401(c, "target")
					removeAttrLower401(c, "rel")
					// HTML4 has no download/ping/referrerpolicy
					removeAttrLower401(c, "download")
					removeAttrLower401(c, "ping")
					removeAttrLower401(c, "referrerpolicy")
				}
				if c.Data == "img" {
					normalizeImgAttrs401(c, s)
				}

				// URL Convert
				if canResolve {
					convertURLsOnNode401(c, parsedPage, s.ForceHTTP, debugMode)
				}

				// Clean <html>
				if c.Data == "html" && len(c.Attr) > 0 {
					c.Attr = c.Attr[:0]
				}
			}
			walk(c)
			c = next
		}
	}
	walk(doc)

	// No global table wrapping for 4.01 by default

	// Render
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render HTML: %w", err)
	}

	// Prepend HTML 4.01 Transitional doctype
	doctype := "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 4.01 Transitional//EN\" \"http://www.w3.org/TR/html4/loose.dtd\">\n"
	return doctype + buf.String(), nil
}

// Helpers with 401 suffix to avoid collision

func shouldDropElement401(n *html.Node) bool {
	switch n.Data {
	case "style", "script", "noscript", "template", "slot":
		return true
	case "canvas", "svg", "object", "embed", "applet":
		return true
	case "progress", "meter", "output", "datalist", "dialog":
		return true
	default:
		return false
	}
}

func shouldDropMeta401(n *html.Node) bool {
	if n.Data != "meta" {
		return false
	}
	if hasAttrLower401(n, "charset") {
		return true
	}
	if strings.EqualFold(getAttrLower401(n, "name"), "viewport") {
		return true
	}
	return false
}

func shouldDropLink401(n *html.Node) bool {
	if n.Data != "link" {
		return false
	}
	rel := strings.ToLower(strings.TrimSpace(getAttrLower401(n, "rel")))
	if rel == "" {
		return false
	}
	// Drop modern link types
	toks := strings.Fields(rel)
	for _, t := range toks {
		switch t {
		case "modulepreload", "manifest":
			return true
		}
	}
	return false
}

func rewriteTagName401(n *html.Node) {
	switch n.Data {
	case "header", "nav", "main", "section", "article", "aside", "footer", "figure", "details", "summary", "dialog":
		n.Data = "div"
	case "figcaption":
		n.Data = "p" // p is safer for 4.01
	case "time", "mark":
		n.Data = "span"
	}
}

func filterAttrsInPlace401(n *html.Node, s *Simplifier401) {
	if n.Type != html.ElementNode || len(n.Attr) == 0 {
		return
	}
	dst := n.Attr[:0]
	for _, a := range n.Attr {
		k := strings.ToLower(a.Key)
		// Drops
		if k == "role" || k == "fetchpriority" || k == "inert" || k == "decoding" || k == "srcset" || k == "sizes" {
			continue
		}
		if strings.HasPrefix(k, "data-") || strings.HasPrefix(k, "aria-") || strings.HasPrefix(k, "on") {
			continue
		}
		if s.DropImgLazy && (k == "loading" || k == "fetchpriority") {
			continue
		}
		// Modern Form Attrs
		switch k {
		case "placeholder", "required", "pattern", "min", "max", "step", "autocomplete", "autofocus", "form":
			continue
		}
		dst = append(dst, a)
	}
	n.Attr = dst
}

func simplifyFormsInPlace401(n *html.Node) {
	if n.Type != html.ElementNode || n.Data != "input" {
		return
	}
	t := strings.ToLower(getAttrLower401(n, "type"))
	switch t {
	case "email", "url", "number", "date", "datetime-local", "time", "week", "month", "search", "tel", "color", "range":
		setAttrLower401(n, "type", "text")
	}
}

func normalizeImgAttrs401(img *html.Node, s *Simplifier401) {
	if !s.KeepImgDims {
		removeAttrLower401(img, "width")
		removeAttrLower401(img, "height")
		return
	}

	// Fix for stretching in legacy browsers (which respect attributes strictly):
	// If 'width' is present, remove 'height' to allow the browser to scale proportionally.
	// This handles cases where modern sites use width=... height=... + CSS object-fit:cover,
	// and we stripped the CSS, leaving potentially disproportionate attributes.
	if hasAttrLower401(img, "width") {
		removeAttrLower401(img, "height")
	}
}

func convertURLsOnNode401(n *html.Node, pageBase *url.URL, forceHTTP bool, debugMode bool) {
	for i := range n.Attr {
		k := strings.ToLower(n.Attr[i].Key)
		if k != "href" && k != "src" && k != "action" {
			continue
		}
		val := strings.TrimSpace(n.Attr[i].Val)
		if val == "" {
			continue
		}
		abs := makeAbsoluteURL401(val, pageBase)
		if forceHTTP {
			abs = httpsToHttp401(abs)
		}
		n.Attr[i].Val = abs

		if debugMode && k == "href" {
			if strings.HasPrefix(val, "#") || strings.EqualFold(val, "javascript:") {
				continue
			}
			if strings.HasPrefix(abs, "/_drp") || strings.HasPrefix(abs, "/debug") {
				continue
			}
			n.Attr[i].Val = "/debug?url=" + url.QueryEscape(abs)
		}
	}
}

func makeAbsoluteURL401(href string, pageBase *url.URL) string {
	// Simple absolute logic duplicate
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
		return href
	}
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	u, err := url.Parse(href)
	if err != nil || pageBase == nil {
		return href
	}
	return pageBase.ResolveReference(u).String()
}

func httpsToHttp401(u string) string {
	if strings.HasPrefix(u, "https://") {
		return "http://" + u[len("https://"):]
	}
	return u
}

func mediaToLinkBlock401(media *html.Node) *html.Node {
	tag := media.Data
	src := strings.TrimSpace(getAttrLower401(media, "src"))
	if src == "" {
		if s := findFirstElementDesc401(media, "source"); s != nil {
			src = strings.TrimSpace(getAttrLower401(s, "src"))
		}
	}
	if src == "" {
		return nil
	}
	p := el401("p")
	p.AppendChild(txt401("[" + tag + ": "))
	a := el401("a")
	setAttrLower401(a, "href", src)
	a.AppendChild(txt401("Open Media"))
	p.AppendChild(a)
	p.AppendChild(txt401("]"))
	return p
}

func buttonToSubmitInput401(btn *html.Node) *html.Node {
	// Simplified button conv
	val := "Submit"
	in := el401("input")
	setAttrLower401(in, "type", "submit")
	setAttrLower401(in, "value", val)
	return in
}

// DOM Utils 401
func el401(name string) *html.Node { return &html.Node{Type: html.ElementNode, Data: name} }
func txt401(s string) *html.Node   { return &html.Node{Type: html.TextNode, Data: s} }
func getAttrLower401(n *html.Node, key string) string {
	key = strings.ToLower(key)
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == key {
			return a.Val
		}
	}
	return ""
}
func hasAttrLower401(n *html.Node, key string) bool { return getAttrLower401(n, key) != "" }
func setAttrLower401(n *html.Node, key, val string) {
	key = strings.ToLower(key)
	for i := range n.Attr {
		if strings.ToLower(n.Attr[i].Key) == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}
func removeAttrLower401(n *html.Node, key string) {
	key = strings.ToLower(key)
	dst := n.Attr[:0]
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) != key {
			dst = append(dst, a)
		}
	}
	n.Attr = dst
}
func detachNode401(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}
func removeAllDoctypes401(root *html.Node) {
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
func findFirstElementDesc401(root *html.Node, tag string) *html.Node {
	// simplified DFS
	var found *html.Node
	var dfs func(n *html.Node)
	dfs = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
	return found
}
