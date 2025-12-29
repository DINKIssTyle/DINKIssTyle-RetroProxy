// Created by DINKIssTyle on 2025. Copyright (C) 2025 DINKI'ssTyle. All rights reserved.
// HTML 3.2 뉴모드 베타.. 구현 중
package proxy

import (
	"bytes"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// LayoutElement represents an element with its computed layout from the browser
type LayoutElement struct {
	Tag      string  `json:"tag"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Text     string  `json:"text"`
	HTML     string  `json:"html"` // New field for block content
	Href     string  `json:"href"`
	Src      string  `json:"src"`
	Alt      string  `json:"alt"`
	Hidden   bool    `json:"hidden"`
	IsBlock  bool    `json:"isBlock"`
	FontSize float64 `json:"fontSize"`
	Color    string  `json:"color"`     // New computed style
	BgColor  string  `json:"bgColor"`   // New computed style
	Align    string  `json:"textAlign"` // New computed style
	Priority int     `json:"priority"`  // 1=main content, 2=secondary, 3=navigation
}

// LayoutBlock represents a grouped block of elements at similar Y position
type LayoutBlock struct {
	Y        float64
	Elements []LayoutElement
}

// Simplifier320New converts browser layout data to HTML 3.2 tables
type Simplifier320New struct{}

func NewSimplifier320New() *Simplifier320New {
	return &Simplifier320New{}
}

// Simplify implements the Simplifier interface (fallback - not used for layout mode)
func (s *Simplifier320New) Simplify(inputHTML string, pageURL string, debugMode bool) (string, error) {
	// This simplifier needs layout data, so we use a different entry point
	// This method is just a fallback that returns the input unchanged
	return inputHTML, nil
}

// SimplifyFromLayout converts layout elements to HTML 3.2
func (s *Simplifier320New) SimplifyFromLayout(elements []LayoutElement, pageURL string, pageTitle string, debugMode bool) string {
	if len(elements) == 0 {
		return s.wrapHTML32("Empty Page", "<p>No content found</p>", pageURL, debugMode)
	}

	// Parse page URL for link resolution
	parsedPage, _ := url.Parse(pageURL)

	// Filter out hidden elements and very small elements
	visible := s.filterVisibleElements(elements)

	// [PHASE 2] Sort by Priority first (1=main, 2=secondary, 3=navigation), then by Y within each priority
	sort.Slice(visible, func(i, j int) bool {
		// Priority takes precedence
		if visible[i].Priority != visible[j].Priority {
			return visible[i].Priority < visible[j].Priority
		}
		// Within same priority, sort by Y (reading order)
		if abs(visible[i].Y-visible[j].Y) < 10 {
			return visible[i].X < visible[j].X
		}
		return visible[i].Y < visible[j].Y
	})

	// Separate priority 3 (navigation) elements - append them at the end or skip
	var mainContent []LayoutElement
	var secondaryContent []LayoutElement
	for _, el := range visible {
		if el.Priority == 3 {
			// Skip navigation elements entirely for cleaner output
			continue
		} else if el.Priority == 2 {
			secondaryContent = append(secondaryContent, el)
		} else {
			mainContent = append(mainContent, el)
		}
	}

	// Build HTML: Main content first, then secondary content at bottom
	var sb strings.Builder

	// Main content (Priority 1)
	if len(mainContent) > 0 {
		mainBlocks := s.groupIntoBlocks(mainContent)
		sb.WriteString(s.blocksToHTML32(mainBlocks, parsedPage, debugMode))
	}

	// Secondary content (Priority 2) - after horizontal rule
	if len(secondaryContent) > 0 {
		sb.WriteString("<hr><font size=\"2\" color=\"#666666\"><b>관련 컨텐츠</b></font><br>\n")
		secondaryBlocks := s.groupIntoBlocks(secondaryContent)
		sb.WriteString(s.blocksToHTML32(secondaryBlocks, parsedPage, debugMode))
	}

	return s.wrapHTML32(pageTitle, sb.String(), pageURL, debugMode)
}

// filterVisibleElements removes hidden and tiny elements
func (s *Simplifier320New) filterVisibleElements(elements []LayoutElement) []LayoutElement {
	var result []LayoutElement
	for _, el := range elements {
		// Skip hidden elements
		if el.Hidden {
			continue
		}
		// Skip very small elements (likely decorative)
		if el.W < 5 || el.H < 5 {
			continue
		}
		// Skip elements with no content
		if el.Text == "" && el.Src == "" && el.Tag != "hr" {
			continue
		}
		result = append(result, el)
	}
	return result
}

// groupIntoBlocks groups elements by similar Y position
func (s *Simplifier320New) groupIntoBlocks(elements []LayoutElement) []LayoutBlock {
	if len(elements) == 0 {
		return nil
	}

	var blocks []LayoutBlock
	threshold := 25.0 // Y threshold for same "row"

	currentBlock := LayoutBlock{Y: elements[0].Y}

	for _, el := range elements {
		if abs(el.Y-currentBlock.Y) > threshold {
			// Start new block
			if len(currentBlock.Elements) > 0 {
				blocks = append(blocks, currentBlock)
			}
			currentBlock = LayoutBlock{Y: el.Y}
		}
		currentBlock.Elements = append(currentBlock.Elements, el)
	}

	// Add last block
	if len(currentBlock.Elements) > 0 {
		blocks = append(blocks, currentBlock)
	}

	return blocks
}

// blocksToHTML32 converts layout blocks to HTML 3.2 table structure
func (s *Simplifier320New) blocksToHTML32(blocks []LayoutBlock, pageBase *url.URL, debugMode bool) string {
	var sb strings.Builder

	sb.WriteString("<table width=\"100%\" border=\"0\" cellpadding=\"5\" cellspacing=\"0\">\n")

	for _, block := range blocks {
		sb.WriteString("<tr><td>")

		// Sort elements in block by X (left to right)
		sort.Slice(block.Elements, func(i, j int) bool {
			return block.Elements[i].X < block.Elements[j].X
		})

		// Detect if this looks like a multi-column layout
		if s.isMultiColumn(block.Elements) {
			sb.WriteString(s.renderMultiColumn(block.Elements, pageBase, debugMode))
		} else {
			sb.WriteString(s.renderSingleColumn(block.Elements, pageBase, debugMode))
		}

		sb.WriteString("</td></tr>\n")
	}

	sb.WriteString("</table>\n")
	return sb.String()
}

// isMultiColumn checks if elements appear to be in multiple columns
func (s *Simplifier320New) isMultiColumn(elements []LayoutElement) bool {
	if len(elements) < 2 {
		return false
	}

	// Check if there's significant X gap between elements
	minX := elements[0].X
	maxX := elements[0].X + elements[0].W

	for _, el := range elements[1:] {
		if el.X > maxX+50 { // Gap of 50px suggests separate column
			return true
		}
		if el.X < minX {
			minX = el.X
		}
		if el.X+el.W > maxX {
			maxX = el.X + el.W
		}
	}

	return false
}

// renderMultiColumn renders elements as a nested table with columns
func (s *Simplifier320New) renderMultiColumn(elements []LayoutElement, pageBase *url.URL, debugMode bool) string {
	var sb strings.Builder

	sb.WriteString("<table width=\"100%\" border=\"0\" cellpadding=\"2\" cellspacing=\"0\"><tr>")

	// Simple 2-column split: left (main) and right (sidebar)
	var leftCol, rightCol []LayoutElement
	midpoint := 0.0
	for _, el := range elements {
		midpoint += el.X + el.W/2
	}
	midpoint /= float64(len(elements))

	for _, el := range elements {
		if el.X+el.W/2 < midpoint {
			leftCol = append(leftCol, el)
		} else {
			rightCol = append(rightCol, el)
		}
	}

	// Render left column (main content)
	sb.WriteString("<td valign=\"top\" width=\"70%\">")
	sb.WriteString(s.renderSingleColumn(leftCol, pageBase, debugMode))
	sb.WriteString("</td>")

	// Render right column (sidebar)
	sb.WriteString("<td valign=\"top\" width=\"30%\">")
	sb.WriteString(s.renderSingleColumn(rightCol, pageBase, debugMode))
	sb.WriteString("</td>")

	sb.WriteString("</tr></table>")
	return sb.String()
}

// renderSingleColumn renders elements in a single column layout
func (s *Simplifier320New) renderSingleColumn(elements []LayoutElement, pageBase *url.URL, debugMode bool) string {
	var sb strings.Builder

	for i, el := range elements {
		if i > 0 {
			prev := elements[i-1]
			// Check if we need a line break
			// If Y difference is significant, force break
			if el.Y > prev.Y+12 { // 12px tolerance for same line
				sb.WriteString("<br>\n")
			} else {
				// Same line, add spacing
				// Check X gap
				gap := el.X - (prev.X + prev.W)
				if gap > 10 {
					sb.WriteString("&nbsp; ")
				} else {
					sb.WriteString(" ")
				}
			}
		}
		sb.WriteString(s.renderElement(el, pageBase, debugMode))
	}

	return sb.String()
}

// renderElement converts a single LayoutElement to HTML 3.2
func (s *Simplifier320New) renderElement(el LayoutElement, pageBase *url.URL, debugMode bool) string {
	// 1. If we have raw HTML content (from block extraction), process it
	if el.HTML != "" {
		processed := s.processBlockHTML(el.HTML, pageBase, debugMode)
		if processed == "" {
			return ""
		}

		var sb strings.Builder

		// Font/Color
		size := "3"
		if el.FontSize >= 32 {
			size = "6"
		} else if el.FontSize >= 24 {
			size = "5"
		} else if el.FontSize >= 18 {
			size = "4"
		} else if el.FontSize <= 10 {
			size = "2"
		}

		colorAttr := ""
		if el.Color != "" && el.Color != "rgb(0, 0, 0)" && el.Color != "rgba(0, 0, 0, 0)" {
			if strings.HasPrefix(el.Color, "#") {
				colorAttr = fmt.Sprintf(" color=\"%s\"", el.Color)
			}
		}

		// Only add font tag if needed
		if size != "3" || colorAttr != "" {
			sb.WriteString(fmt.Sprintf("<font size=\"%s\"%s>", size, colorAttr))
		}

		sb.WriteString(processed)

		if size != "3" || colorAttr != "" {
			sb.WriteString("</font>")
		}

		result := sb.String()
		if el.Href != "" {
			href := s.resolveURL(el.Href, pageBase)
			if debugMode && href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(href, "javascript:") && !strings.HasPrefix(href, "/_drp") {
				href = "/debug?url=" + url.QueryEscape(href) + "&mode=3.2new"
			}
			return fmt.Sprintf("<a href=\"%s\">%s</a>", href, result)
		}

		return result
	}

	// 2. Fallback for elements without HTML (e.g. standalone images)
	switch el.Tag {
	case "img":
		src := s.resolveURL(el.Src, pageBase)
		// Fix: If resolved URL contains /_drp with wrong domain, extract just the /_drp path
		if strings.Contains(src, "/_drp/") {
			if idx := strings.Index(src, "/_drp/"); idx != -1 {
				src = src[idx:] // Extract "/_drp/..." part
			}
		}

		alt := el.Alt
		if alt == "" {
			alt = "[Image]"
		}
		return fmt.Sprintf("<img src=\"%s\" alt=\"%s\" border=\"0\">", src, alt) // Removed <br> here too

	case "hr":
		return "<hr>\n"

	default:
		// Text content fallback
		text := strings.TrimSpace(el.Text)
		if text == "" {
			return ""
		}
		return text // Removed <br> here too, let renderSingleColumn handle it
	}
}

// processBlockHTML parses and sanitizes HTML for 3.2 compatibility
func (s *Simplifier320New) processBlockHTML(rawHTML string, pageBase *url.URL, debugMode bool) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML // Fallback
	}

	var buf bytes.Buffer
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
			return
		} else if n.Type == html.ElementNode {
			tag := n.Data

			// Allowed tags
			switch tag {
			case "b", "strong":
				buf.WriteString("<b>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				buf.WriteString("</b>")
			case "i", "em":
				buf.WriteString("<i>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				buf.WriteString("</i>")
			case "u":
				buf.WriteString("<u>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				buf.WriteString("</u>")
			case "br":
				buf.WriteString("<br>")
			case "a":
				href := ""
				for _, a := range n.Attr {
					if a.Key == "href" {
						href = a.Val
						break
					}
				}
				if href != "" {
					resolved := s.resolveURL(href, pageBase)
					if debugMode && !strings.HasPrefix(resolved, "#") && !strings.HasPrefix(resolved, "javascript:") && !strings.HasPrefix(resolved, "/_drp") {
						resolved = "/debug?url=" + url.QueryEscape(resolved) + "&mode=3.2new"
					}
					buf.WriteString(fmt.Sprintf("<a href=\"%s\">", resolved))
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						walk(c)
					}
					buf.WriteString("</a>")
				} else {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						walk(c)
					}
				}
			case "img":
				src := ""
				alt := ""
				// Check for lazy loading attributes
				lazySrc := ""

				for _, a := range n.Attr {
					key := strings.ToLower(a.Key)
					val := a.Val

					if key == "src" {
						src = val
					} else if key == "alt" {
						alt = val
					} else if key == "data-src" || key == "data-original" || key == "data-lazy-src" {
						if lazySrc == "" { // prioritize first found
							lazySrc = val
						}
					}
				}

				// Use lazy src if real src is missing or placeholder
				if lazySrc != "" {
					if src == "" || strings.HasPrefix(src, "data:") || len(src) < 100 {
						src = lazySrc
					}
				}

				if src != "" {
					resolved := s.resolveURL(src, pageBase)

					// Fix: If resolved URL contains /_drp with wrong domain, extract just the /_drp path
					if strings.Contains(resolved, "/_drp/") {
						if idx := strings.Index(resolved, "/_drp/"); idx != -1 {
							resolved = resolved[idx:] // Extract "/_drp/..." part
						}
					}

					// Rewrite to Image Proxy URL if not already local
					// This handles HTTPS/TLS issues for legacy browsers
					if !strings.HasPrefix(resolved, "/_drp") && (strings.HasPrefix(resolved, "http://") || strings.HasPrefix(resolved, "https://")) {
						resolved = "/_drp/image?url=" + url.QueryEscape(resolved)
					}

					buf.WriteString(fmt.Sprintf("<img src=\"%s\" alt=\"%s\" border=\"0\">", resolved, alt))
				}
			case "p", "div":
				buf.WriteString("<br>")
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				buf.WriteString("<br>")
			case "script", "style", "noscript", "iframe", "svg", "meta", "link", "head", "title":
				// Ignore completely (do not walk children)
			default:
				// Strip tag, keep content (for span, font, etc.)
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
		} else {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}

	// Skip html/body/head wrapper from Parse
	if doc.FirstChild != nil {
		// html -> body -> ...
		var body *html.Node
		for c := doc.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "html" {
				for gc := c.FirstChild; gc != nil; gc = gc.NextSibling {
					if gc.Type == html.ElementNode && gc.Data == "body" {
						body = gc
						break
					}
				}
			}
		}
		if body != nil {
			for c := body.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		} else {
			walk(doc)
		}
	}

	return buf.String()
}

// resolveURL converts relative URLs to absolute and downgrades HTTPS
func (s *Simplifier320New) resolveURL(href string, pageBase *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(strings.ToLower(href), "data:") {
		return href
	}

	// Preserve internal proxy paths - don't resolve against external base URL
	if strings.HasPrefix(href, "/_drp") {
		return href
	}

	// Protocol-relative
	if strings.HasPrefix(href, "//") {
		return "http:" + href
	}

	// Already HTTPS, convert to HTTP
	if strings.HasPrefix(href, "https://") {
		return "http://" + href[8:]
	}

	// Already HTTP
	if strings.HasPrefix(href, "http://") {
		return href
	}

	// Resolve relative URL
	if pageBase != nil {
		u, err := url.Parse(href)
		if err != nil {
			return href
		}
		resolved := pageBase.ResolveReference(u)
		if resolved.Scheme == "https" {
			resolved.Scheme = "http"
		}
		return resolved.String()
	}

	return href
}

// wrapHTML32 wraps content in HTML 3.2 document structure
func (s *Simplifier320New) wrapHTML32(title, content, pageURL string, debugMode bool) string {
	var sb strings.Builder

	sb.WriteString("<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\">\n")
	sb.WriteString("<html>\n<head>\n")
	sb.WriteString(fmt.Sprintf("<title>%s</title>\n", title))
	sb.WriteString("<meta http-equiv=\"Content-Type\" content=\"text/html; charset=utf-8\">\n")
	sb.WriteString("</head>\n")
	sb.WriteString("<body bgcolor=\"#ffffff\" text=\"#000000\" link=\"#0000EE\" vlink=\"#551A8B\">\n")

	// Debug toolbar note
	if debugMode {
		sb.WriteString("<font size=\"1\" color=\"gray\">Mode: HTML 3.2 New (Layout-based) | ")
		sb.WriteString(fmt.Sprintf("Source: %s</font><hr>\n", pageURL))
	}

	sb.WriteString(content)

	sb.WriteString("\n<hr>\n")
	sb.WriteString(fmt.Sprintf("<font size=\"1\">%s (HTML 3.2 New)</font>\n", FooterText))
	sb.WriteString("</body>\n</html>")

	return sb.String()
}

// abs returns absolute value
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
