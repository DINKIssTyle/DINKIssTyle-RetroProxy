package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// SimplifierText converts HTML to a text-only representation (HTML structure kept but stripped down)
type SimplifierText struct{}

func NewSimplifierText() *SimplifierText {
	return &SimplifierText{}
}

func (s *SimplifierText) Simplify(inputHTML string, pageURL string, debugMode bool) (string, error) {
	doc, err := html.Parse(strings.NewReader(inputHTML))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	parsedPage, _ := url.Parse(pageURL)
	canResolve := parsedPage != nil && parsedPage.Scheme != "" && parsedPage.Host != ""

	// Remove Doctypes
	removeAllDoctypesText(doc)

	// DFS to strip non-text elements
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling

			if c.Type == html.ElementNode {
				// Drop non-text/structural tags
				switch c.Data {
				case "script", "style", "noscript", "iframe", "object", "embed", "canvas", "svg", "video", "audio", "link", "meta", "base", "input", "button", "select", "textarea", "form":
					n.RemoveChild(c)
					c = next
					continue
				case "img":
					// Replace img with [Image: alt] text
					alt := getAttrLowerText(c, "alt")
					if alt != "" {
						textNode := &html.Node{Type: html.TextNode, Data: " [Image: " + alt + "] "}
						n.InsertBefore(textNode, c)
					}
					n.RemoveChild(c)
					c = next
					continue
				}

				// Simplify attributes (strip logic)
				// We only keep href for <a>. All other styles are gone.
				if c.Data == "a" {
					// Keep href, remove target/rel/style/class
					href := getAttrLowerText(c, "href")
					c.Attr = nil // clear all
					if href != "" {
						if canResolve {
							href = makeAbsoluteURLText(href, parsedPage)
						}
						// Debug mode rewrite
						if debugMode && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "/_drp") && !strings.HasPrefix(href, "/debug") {
							href = "/debug?url=" + url.QueryEscape(href)
						}
						c.Attr = append(c.Attr, html.Attribute{Key: "href", Val: href})
					}
				} else {
					// Strip all attributes for other tags (style, class, etc.)
					c.Attr = nil
				}
			}
			walk(c)
			c = next
		}
	}
	walk(doc)

	// Render
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render HTML: %w", err)
	}

	// Simple Pre-formatting style for text mode
	style := `<style>body{font-family:monospace;white-space:pre-wrap;word-wrap:break-word;background:#fff;color:#000;} a{text-decoration:underline;color:blue;}</style>`
	return "<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\"><html><head><title>Text Mode</title>" + style + "</head><body>" + buf.String() + "</body></html>", nil
}

// Helpers
func removeAllDoctypesText(root *html.Node) {
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

func getAttrLowerText(n *html.Node, key string) string {
	key = strings.ToLower(key)
	for _, a := range n.Attr {
		if strings.ToLower(a.Key) == key {
			return a.Val
		}
	}
	return ""
}

func makeAbsoluteURLText(href string, pageBase *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
		return href
	}

	// Downgrade explicit HTTPS or Protocol-relative URLs
	if strings.HasPrefix(href, "https://") {
		href = "http://" + href[8:]
	} else if strings.HasPrefix(href, "//") {
		href = "http:" + href
	}

	// If absolute HTTP now, return
	if strings.HasPrefix(href, "http://") {
		return href
	}

	// Resolve relative path
	u, err := url.Parse(href)
	if err != nil || pageBase == nil {
		return href
	}
	resolved := pageBase.ResolveReference(u).String()

	// Ensure resolved URL is also downgraded
	if strings.HasPrefix(resolved, "https://") {
		return "http://" + resolved[8:]
	}
	return resolved
}
