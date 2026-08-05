// Created by DINKIssTyle on 2025. Copyright (C) 2025 DINKI'ssTyle. All rights reserved.

package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// SimplifierPassthrough keeps the original HTML but converts all HTTPS URLs to HTTP
// This is for modern browsers that don't support SSL/TLS
type SimplifierPassthrough struct{}

func NewSimplifierPassthrough() *SimplifierPassthrough {
	return &SimplifierPassthrough{}
}

func (s *SimplifierPassthrough) Simplify(inputHTML string, pageURL string, debugMode bool) (string, error) {
	doc, err := html.Parse(strings.NewReader(inputHTML))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	parsedPage, _ := url.Parse(pageURL)
	canResolve := parsedPage != nil && parsedPage.Scheme != "" && parsedPage.Host != ""

	// Walk through all nodes and convert HTTPS to HTTP in URLs
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Convert URLs in common attributes
			for i := range n.Attr {
				key := strings.ToLower(n.Attr[i].Key)
				if key == "href" || key == "src" || key == "action" || key == "poster" || key == "data" {
					href := n.Attr[i].Val

					// Debug mode: rewrite links to go through debug viewer
					if debugMode && key == "href" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "/_drp") && !strings.HasPrefix(href, "/debug") {
						if canResolve {
							href = makeAbsoluteURLPassthrough(href, parsedPage)
						}
						n.Attr[i].Val = "/debug?url=" + url.QueryEscape(href) + "&mode=modern"
					} else {
						// Normal mode: just convert HTTPS to HTTP
						n.Attr[i].Val = convertHTTPStoHTTP(href, parsedPage, canResolve)
					}
				}
				// Also handle srcset attribute
				if key == "srcset" {
					n.Attr[i].Val = convertSrcsetToHTTP(n.Attr[i].Val, parsedPage, canResolve)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Render the modified HTML
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", fmt.Errorf("failed to render HTML: %w", err)
	}

	return buf.String(), nil
}

// convertHTTPStoHTTP converts a URL from HTTPS to HTTP
func convertHTTPStoHTTP(href string, pageBase *url.URL, canResolve bool) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(strings.ToLower(href), "data:") {
		return href
	}

	// Protocol-relative URLs
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}

	// Already HTTPS, convert to HTTP
	if strings.HasPrefix(href, "https://") {
		return "http://" + href[8:]
	}

	// Already HTTP, keep as is
	if strings.HasPrefix(href, "http://") {
		return href
	}

	// Relative URL - resolve against page base
	if canResolve && pageBase != nil {
		u, err := url.Parse(href)
		if err != nil {
			return href
		}
		resolved := pageBase.ResolveReference(u)
		// Convert resolved URL to HTTP
		if resolved.Scheme == "https" {
			resolved.Scheme = "http"
		}
		return resolved.String()
	}

	return href
}

// convertSrcsetToHTTP converts all URLs in a srcset attribute to HTTP
func convertSrcsetToHTTP(srcset string, pageBase *url.URL, canResolve bool) string {
	parts := strings.Split(srcset, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// srcset format: "url descriptor"
		fields := strings.Fields(part)
		if len(fields) >= 1 {
			fields[0] = convertHTTPStoHTTP(fields[0], pageBase, canResolve)
			result = append(result, strings.Join(fields, " "))
		}
	}
	return strings.Join(result, ", ")
}

// makeAbsoluteURLPassthrough resolves a URL to absolute
func makeAbsoluteURLPassthrough(href string, pageBase *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
		return href
	}

	// Protocol-relative
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}

	// Already absolute
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		if strings.HasPrefix(href, "https://") {
			return "http://" + href[8:]
		}
		return href
	}

	// Resolve relative
	u, err := url.Parse(href)
	if err != nil || pageBase == nil {
		return href
	}
	resolved := pageBase.ResolveReference(u)
	if resolved.Scheme == "https" {
		resolved.Scheme = "http"
	}
	return resolved.String()
}
