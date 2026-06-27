// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// ErrRendererBusy is returned when the renderer is busy processing another request
var ErrRendererBusy = errors.New("renderer is busy, please try again later")

// Renderer uses a headless browser to render modern web pages
type Renderer struct {
	browser *rod.Browser
	mu      sync.Mutex
	busy    bool
}

// IsBusy returns true if the renderer is currently processing a request
func (r *Renderer) IsBusy() bool {
	return r.busy
}

// NewRenderer creates a new renderer
func NewRenderer() *Renderer {
	return &Renderer{}
}

// ensureBrowser ensures the browser is launched
func (r *Renderer) ensureBrowser() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.browser != nil {
		return nil
	}

	// Launch browser with options for stability
	// Leakless(false) prevents extraction of leakless.exe which triggers Windows Defender
	path, _ := launcher.LookPath()
	u := launcher.New().
		Bin(path).
		Headless(true).
		NoSandbox(true).
		Leakless(false).
		MustLaunch()

	r.browser = rod.New().ControlURL(u).MustConnect()
	return nil
}

// RenderPage navigates to the URL and returns the rendered HTML
func (r *Renderer) RenderPage(ctx context.Context, url string) (string, error) {
	if err := r.ensureBrowser(); err != nil {
		return "", fmt.Errorf("failed to start browser: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a stealth page to bypass bot detection (Cloudflare, etc.)
	page, err := stealth.Page(r.browser)
	if err != nil {
		return "", fmt.Errorf("failed to create stealth page: %w", err)
	}
	// Use request context for cancellation
	page = page.Context(ctx)
	defer func() {
		// Stop loading to save resources if cancelled
		if ctx.Err() != nil {
			page.StopLoading()
		}
		page.Close()
	}()

	// Set a reasonable viewport
	page.MustSetViewport(1280, 720, 1.0, false)

	// Set timeout for page load - increased to 30 seconds for Cloudflare challenges
	page = page.Timeout(30 * time.Second)

	// Hijack requests to block heavy resources (but allow JS for Cloudflare challenge)
	router := page.HijackRequests()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		switch ctx.Request.Type() {
		case proto.NetworkResourceTypeImage,
			proto.NetworkResourceTypeStylesheet,
			proto.NetworkResourceTypeFont,
			proto.NetworkResourceTypeMedia:
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	go router.Run()

	// Navigate to the URL
	err = page.Navigate(url)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	// Wait for DOM to be ready
	_ = page.WaitLoad()

	// Check for Cloudflare challenge and wait if needed
	if r.isCloudflareChallenge(page) {
		// Wait for Cloudflare challenge to complete (up to 10 seconds)
		for i := 0; i < 20; i++ {
			// Check context before sleep
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			if !r.isCloudflareChallenge(page) {
				break
			}
		}
		// Wait a bit more for page to fully load after challenge
		time.Sleep(1 * time.Second)
		_ = page.WaitLoad()
	}

	// Wait for network idle
	done := make(chan struct{})
	go func() {
		defer close(done)
		page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)()
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-done:
	case <-time.After(3 * time.Second): // Hard cap
	}

	// Get the rendered HTML
	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return html, nil
}

// RenderPageFull navigates to the URL and returns the rendered HTML with CSS/JS intact
// This is used for Modern mode where we want to preserve the layout
func (r *Renderer) RenderPageFull(ctx context.Context, url string) (string, error) {
	if err := r.ensureBrowser(); err != nil {
		return "", fmt.Errorf("failed to start browser: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a stealth page
	page, err := stealth.Page(r.browser)
	if err != nil {
		return "", fmt.Errorf("failed to create stealth page: %w", err)
	}
	// Use request context
	page = page.Context(ctx)
	defer func() {
		if ctx.Err() != nil {
			page.StopLoading()
		}
		page.Close()
	}()

	page.MustSetViewport(1280, 720, 1.0, false)
	page = page.Timeout(30 * time.Second)

	// Only block heavy resources (images/media), keep CSS/JS for layout
	router := page.HijackRequests()
	router.MustAdd("*", func(ctx *rod.Hijack) {
		switch ctx.Request.Type() {
		case proto.NetworkResourceTypeImage,
			proto.NetworkResourceTypeMedia:
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	go router.Run()

	// Navigate
	err = page.Navigate(url)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	_ = page.WaitLoad()

	// Check for Cloudflare and wait
	if r.isCloudflareChallenge(page) {
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			if !r.isCloudflareChallenge(page) {
				break
			}
		}
		time.Sleep(1 * time.Second)
		_ = page.WaitLoad()
	}

	// Wait for network idle
	done := make(chan struct{})
	go func() {
		defer close(done)
		page.WaitRequestIdle(500*time.Millisecond, nil, nil, nil)()
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-done:
	case <-time.After(3 * time.Second):
	}

	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("failed to get HTML: %w", err)
	}

	return html, nil
}

// isCloudflareChallenge checks if the current page is a Cloudflare challenge
func (r *Renderer) isCloudflareChallenge(page *rod.Page) bool {
	html, err := page.HTML()
	if err != nil {
		return false
	}
	// Common Cloudflare challenge indicators
	cfIndicators := []string{
		"Just a moment...",
		"Checking your browser",
		"cf-browser-verification",
		"cf_chl_opt",
		"cloudflare",
		"Verifying you are human",
		"turnstile",
	}
	htmlLower := strings.ToLower(html)
	for _, indicator := range cfIndicators {
		if strings.Contains(htmlLower, strings.ToLower(indicator)) {
			return true
		}
	}
	return false
}

// LayoutResult contains the extracted layout data and page title
type LayoutResult struct {
	Title    string
	Elements []LayoutElement
}

// RenderPageWithLayout extracts layout data from rendered page for HTML 3.2 New mode
func (r *Renderer) RenderPageWithLayout(ctx context.Context, url string) (*LayoutResult, error) {
	if err := r.ensureBrowser(); err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a stealth page
	page, err := stealth.Page(r.browser)
	if err != nil {
		return nil, fmt.Errorf("failed to create stealth page: %w", err)
	}
	page = page.Context(ctx)
	defer func() {
		if ctx.Err() != nil {
			page.StopLoading()
		}
		page.Close()
	}()

	// Set viewport
	page.MustSetViewport(1280, 720, 1.0, false)
	page = page.Timeout(30 * time.Second)

	// Navigate
	err = page.Navigate(url)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	_ = page.WaitLoad()

	// Check for Cloudflare and wait
	if r.isCloudflareChallenge(page) {
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			if !r.isCloudflareChallenge(page) {
				break
			}
		}
		time.Sleep(1 * time.Second)
		_ = page.WaitLoad()
	}

	// Wait for network idle
	done := make(chan struct{})
	go func() {
		defer close(done)
		page.WaitRequestIdle(1*time.Second, nil, nil, nil)()
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
	case <-time.After(5 * time.Second):
	}

	// Additional wait for JS-rendered content (important for news sites)
	time.Sleep(500 * time.Millisecond)

	// Extract layout data using JavaScript (Block-based approach)
	jsCode := `() => {
		const result = [];
		const uniqueElements = new Set();
		
		// Helper: Check if element is a block-level container
		const isBlock = (el) => {
			const style = window.getComputedStyle(el);
			const display = style.display;
			const tag = el.tagName.toLowerCase();
			
			// Semantic fallback: treat article content elements as blocks regardless of CSS
			const semanticBlocks = ['article', 'section', 'main', 'aside', 'nav', 'header', 'footer', 'p', 'div', 'li', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'pre', 'figcaption', 'figure'];
			const isSemanticBlock = semanticBlocks.includes(tag);
			
			// CSS-based block detection
			const isDisplayBlock = (display === 'block' || display === 'flex' || display === 'grid' || display === 'table' || display === 'table-cell' || display === 'list-item' || display === 'inline-block' || display === '-webkit-box');
			
			// Must be visible
			const isVisible = style.visibility !== 'hidden' && style.opacity !== '0' && display !== 'none';
			
			return (isDisplayBlock || isSemanticBlock) && isVisible;
		};

		// Helper: Check if element contains any block-level children
		const hasBlockChildren = (el) => {
			for (let child of el.children) {
				if (isBlock(child)) return true;
			}
			return false;
		};

		// Helper: Get priority of element based on its container context
		// Priority 1 = Main content (article, main, .article-body, etc.)
		// Priority 2 = Secondary content (aside, sidebar, comments, related)
		// Priority 3 = Navigation/UI (nav, header, footer, ads)
		const getPriority = (el) => {
			// Check ancestors to determine context
			let current = el;
			while (current && current !== document.body) {
				const tag = current.tagName.toLowerCase();
				const className = (current.className || '').toLowerCase();
				const id = (current.id || '').toLowerCase();
				const role = current.getAttribute('role') || '';
				
				// Priority 1: Main content containers
				if (tag === 'article' || tag === 'main' || 
					className.includes('article') || className.includes('post-content') ||
					className.includes('news_view') || className.includes('content_body') ||
					id.includes('articleBody') || id.includes('article') ||
					role === 'main' || role === 'article') {
					return 1;
				}
				
				// Priority 3: Navigation/UI elements (check before secondary)
				if (tag === 'nav' || tag === 'header' || tag === 'footer' ||
					className.includes('nav') || className.includes('menu') ||
					className.includes('header') || className.includes('footer') ||
					className.includes('ad') || className.includes('banner') ||
					className.includes('popup') || className.includes('modal') ||
					role === 'navigation' || role === 'banner') {
					return 3;
				}
				
				// Priority 2: Secondary content
				if (tag === 'aside' || 
					className.includes('sidebar') || className.includes('aside') ||
					className.includes('ranking') || className.includes('recommend') ||
					className.includes('related') || className.includes('comment') ||
					className.includes('most-viewed') || className.includes('popular') ||
					role === 'complementary') {
					return 2;
				}
				
				current = current.parentElement;
			}
			
			// Default: treat as secondary (conservative)
			return 2;
		};

		// 1. Find Leaf Blocks (Blocks containing only inline content)
		const allElements = document.body.querySelectorAll('*');
		
		allElements.forEach(el => {
			if (!isBlock(el)) return;
			
			// Check if this block itself is a link or is inside a link
			let href = '';
			const parentLink = el.closest('a');
			if (el.tagName.toLowerCase() === 'a') {
				href = el.href;
			} else if (parentLink) {
				href = parentLink.href;
			}

			// CASE 1: Container Block (Mixed Content)
			// If it has block children, we usually skip it in the Leaf Block logic.
			// BUT we must rescue any direct text nodes (orphans) that sit between blocks.
			if (hasBlockChildren(el)) {
				el.childNodes.forEach(node => {
					if (node.nodeType === 3) { // Text Node
						const orphanText = node.nodeValue.trim();
						if (orphanText.length > 0) {
							const range = document.createRange();
							range.selectNode(node);
							const rect = range.getBoundingClientRect();
							if (rect.width > 0 && rect.height > 0) {
								result.push({
									tag: 'span', // Treat as span
									x: rect.x,
									y: rect.y,
									w: rect.width,
									h: rect.height,
									text: orphanText, // Use unique name just in case
									html: '',
									href: href,
									src: '',
									alt: '',
									hidden: false,
									isBlock: false,
									fontSize: parseFloat(window.getComputedStyle(el).fontSize) || 14,
									color: window.getComputedStyle(el).color,
									bgColor: 'transparent',
									textAlign: window.getComputedStyle(el).textAlign,
									priority: getPriority(el)
								});
							}
						}
					}
				});
				return; // Skip the container element itself
			}

			// CASE 2: Leaf Block
			// Filter out empty blocks
			const text = el.innerText || el.textContent || '';
			if (text.trim() === '' && !el.querySelector('img')) return;

			const rect = el.getBoundingClientRect();
			if (rect.width <= 0 || rect.height <= 0) return;
			if (rect.bottom < 0 || rect.top > window.innerHeight * 10) return; // Limit scan height

			const style = window.getComputedStyle(el);
			
			// Extract tag for semantic meaning
			let tag = el.tagName.toLowerCase();
			// Normalize generic blocks
			if (!['h1','h2','h3','h4','h5','h6','li','p','pre'].includes(tag)) {
				tag = 'div';
			}

			// Add to result
			uniqueElements.add(el);
			result.push({
				tag: tag,
				x: rect.x,
				y: rect.y,
				w: rect.width,
				h: rect.height,
				text: text,
				html: el.innerHTML,
				href: href,
				src: '',
				alt: '',
				hidden: false,
				isBlock: true,
				fontSize: parseFloat(style.fontSize) || 14,
				color: style.color,
				bgColor: style.backgroundColor,
				textAlign: style.textAlign,
				priority: getPriority(el)
			});
		});

		// 2. Rescue orphan images (images not inside the captured leaf blocks)
		// Images inside captured leaf blocks have their HTML preserved.
		// But images that are inside skipped containers need explicit capture.
		document.querySelectorAll('img').forEach(img => {
			// Skip if already inside a captured element
			let parent = img.parentElement;
			let isInCaptured = false;
			while (parent && parent !== document.body) {
				if (uniqueElements.has(parent)) {
					isInCaptured = true;
					break;
				}
				parent = parent.parentElement;
			}
			if (isInCaptured) return;

			const rect = img.getBoundingClientRect();
			if (rect.width <= 10 || rect.height <= 10) return; // Skip tiny images (likely icons/trackers)
			if (rect.bottom < 0 || rect.top > window.innerHeight * 10) return;

			// Get src from various attributes
			let src = img.src || img.dataset.src || img.dataset.original || img.dataset.lazySrc || '';
			if (!src || src.startsWith('data:')) return;

			const alt = img.alt || '';

			result.push({
				tag: 'img',
				x: rect.x,
				y: rect.y,
				w: rect.width,
				h: rect.height,
				text: '',
				html: img.outerHTML,
				href: '',
				src: src,
				alt: alt,
				hidden: false,
				isBlock: true,
				fontSize: 0,
				color: '',
				bgColor: '',
				textAlign: '',
				priority: getPriority(img)
			});
		});
		
		// 3. HR Extraction
		document.querySelectorAll('hr').forEach(hr => {
			const rect = hr.getBoundingClientRect();
			if (rect.width > 0) {
				result.push({
					tag: 'hr',
					x: rect.x,
					y: rect.y,
					w: rect.width,
					h: rect.height,
					text: '',
					html: '',
					href: '',
					src: '',
					alt: '',
					hidden: false,
					isBlock: true,
					fontSize: 0,
					color: '',
					bgColor: '',
					textAlign: '',
					priority: 2
				});
			}
		});

		return {
			title: document.title || 'Untitled',
			elements: result
		};
	}`

	var layoutResult struct {
		Title    string          `json:"title"`
		Elements []LayoutElement `json:"elements"`
	}

	// Use MustEval with panic recovery
	var evalErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				evalErr = fmt.Errorf("JavaScript panic: %v", rec)
			}
		}()
		result := page.MustEval(jsCode)
		evalErr = result.Unmarshal(&layoutResult)
	}()

	if evalErr != nil {
		return nil, fmt.Errorf("failed to extract layout: %w", evalErr)
	}

	return &LayoutResult{
		Title:    layoutResult.Title,
		Elements: layoutResult.Elements,
	}, nil
}

// RenderPageWithScreenshot renders the page and captures a screenshot
func (r *Renderer) RenderPageWithScreenshot(ctx context.Context, url string) (string, []byte, error) {
	if err := r.ensureBrowser(); err != nil {
		return "", nil, fmt.Errorf("failed to start browser: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a stealth page to bypass bot detection
	page, err := stealth.Page(r.browser)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create stealth page: %w", err)
	}
	// Use request context
	page = page.Context(ctx)
	defer func() {
		if ctx.Err() != nil {
			page.StopLoading()
		}
		page.Close()
	}()

	page.MustSetViewport(1280, 720, 1.0, false)
	page = page.Timeout(30 * time.Second)

	err = page.Navigate(url)
	if err != nil {
		return "", nil, fmt.Errorf("failed to navigate: %w", err)
	}

	page.WaitLoad()

	// Check for Cloudflare challenge and wait if needed
	if r.isCloudflareChallenge(page) {
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				return "", nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			if !r.isCloudflareChallenge(page) {
				break
			}
		}
		time.Sleep(1 * time.Second)
		page.WaitLoad()
	}

	time.Sleep(2 * time.Second)
	page.WaitRequestIdle(time.Second, nil, nil, nil)()

	html, err := page.HTML()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get HTML: %w", err)
	}

	quality := 80
	// Capture screenshot as JPEG for legacy browser compatibility
	screenshot, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: &quality,
	})
	if err != nil {
		return html, nil, fmt.Errorf("failed to capture screenshot: %w", err)
	}

	return html, screenshot, nil
}

// LinkRect defines a clickable area
type LinkRect struct {
	X, Y, W, H float64
	Href       string
}

// InputRect represents an input text field area
type InputRect struct {
	X, Y, W, H float64
	Name       string
	XPath      string
}

// CaptureScreenshotAndLinks navigates to the URL, takes a full page screenshot, and extracts link coordinates
func (r *Renderer) CaptureScreenshotAndLinks(ctx context.Context, urlStr string) ([]byte, []LinkRect, []InputRect, string, error) {
	return r.captureLogic(ctx, urlStr, nil)
}

// Helper for capture logic to be reused by SubmitInput
func (r *Renderer) captureLogic(ctx context.Context, urlStr string, interaction func(*rod.Page) error) (imageData []byte, links []LinkRect, inputs []InputRect, currentURL string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic in captureLogic: %v", rec)
		}
	}()

	if err = r.ensureBrowser(); err != nil {
		err = fmt.Errorf("failed to start browser: %w", err)
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create a stealth page to bypass bot detection
	page, err := stealth.Page(r.browser)
	if err != nil {
		err = fmt.Errorf("failed to create stealth page: %w", err)
		return
	}
	// Use request context
	page = page.Context(ctx)
	defer func() {
		if ctx.Err() != nil {
			page.StopLoading()
		}
		page.Close()
	}()

	// Initial Viewport
	if err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 1024, Height: 768, DeviceScaleFactor: 1.0, Mobile: false}); err != nil {
		err = fmt.Errorf("failed to set viewport: %w", err)
		return
	}

	page = page.Timeout(60 * time.Second)

	if err = page.Navigate(urlStr); err != nil {
		err = fmt.Errorf("failed to navigate: %w", err)
		return
	}

	if err = page.WaitLoad(); err != nil {
		err = fmt.Errorf("failed to wait load: %w", err)
		return
	}

	// Check for Cloudflare challenge and wait if needed
	if r.isCloudflareChallenge(page) {
		for i := 0; i < 20; i++ {
			select {
			case <-ctx.Done():
				err = ctx.Err()
				return
			case <-time.After(500 * time.Millisecond):
			}
			if !r.isCloudflareChallenge(page) {
				break
			}
		}
		time.Sleep(1 * time.Second)
		page.WaitLoad()
	}

	// Handle interaction if provided
	if interaction != nil {
		if err = interaction(page); err != nil {
			err = fmt.Errorf("interaction failed: %w", err)
			return
		}
		time.Sleep(1 * time.Second)
		page.WaitLoad()
	}

	// Get Current URL
	info, _ := page.Info()
	currentURL = info.URL

	// 1. Measure Full Height & Trigger lazy loads gradually
	// Scroll down gradually to trigger lazy-loaded assets
	_, _ = page.Eval(`async () => {
		await new Promise((resolve) => {
			let totalHeight = 0;
			const distance = 800;
			const timer = setInterval(() => {
				const scrollHeight = Math.max(document.body.scrollHeight, document.documentElement.scrollHeight);
				window.scrollBy(0, distance);
				totalHeight += distance;
				if (totalHeight >= scrollHeight) {
					clearInterval(timer);
					resolve();
				}
			}, 50);
		});
		window.scrollTo(0, 0);
	}`)
	time.Sleep(300 * time.Millisecond) // Allow images to start loading
	_ = page.WaitRequestIdle(400*time.Millisecond, nil, nil, nil) // Wait for network requests to finish

	res, err := page.Eval("() => Math.max(document.body.scrollHeight, document.documentElement.scrollHeight, document.body.offsetHeight, document.documentElement.offsetHeight)")
	if err != nil {
		err = fmt.Errorf("failed to get page height: %w", err)
		return
	}
	fullHeight := int(res.Value.Int())

	// Cap height to prevent crashes (e.g., 15000px)
	const MaxHeight = 15000
	if fullHeight > MaxHeight {
		fullHeight = MaxHeight
	}
	if fullHeight < 768 {
		fullHeight = 768
	}

	// 2. Resize Viewport to Full Height
	// This ensures sticky elements stay at the top/bottom and don't repeat
	if err = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 1024, Height: fullHeight, DeviceScaleFactor: 1.0, Mobile: false}); err != nil {
		err = fmt.Errorf("failed to set viewport (full): %w", err)
		return
	}
	time.Sleep(150 * time.Millisecond) // Allow layout to settle

	// Hide scrollbars to prevent them from appearing in screenshots and throwing off coordinates
	_, _ = page.Eval(`() => {
		const style = document.createElement('style');
		style.id = 'drp-hide-scrollbars';
		style.innerHTML = 'html, body { overflow: hidden !important; }';
		document.head.appendChild(style);
	}`)
	time.Sleep(50 * time.Millisecond) // Allow style changes to take effect

	// After hiding scrollbars and resizing, the document height might change (e.g. text wraps differently, or more content is revealed)
	// We measure the height again to ensure the viewport perfectly matches the content size
	res2, err := page.Eval("() => Math.max(document.body.scrollHeight, document.documentElement.scrollHeight, document.body.offsetHeight, document.documentElement.offsetHeight)")
	if err == nil {
		actualHeight := int(res2.Value.Int())
		if actualHeight > MaxHeight {
			actualHeight = MaxHeight
		}
		if actualHeight < 768 {
			actualHeight = 768
		}
		if actualHeight != fullHeight {
			fullHeight = actualHeight
			_ = page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{Width: 1024, Height: fullHeight, DeviceScaleFactor: 1.0, Mobile: false})
			time.Sleep(100 * time.Millisecond)
		}
	}

	// 3. Extract Coordinates (Now consistent because viewport covers everything)
	jsScript := `() => {
		const results = { links: [], inputs: [] };
		try {
			// Get all links
			document.querySelectorAll('a[href]').forEach(a => {
				const rect = a.getBoundingClientRect();
				if (rect.width > 0 && rect.height > 0) {
					results.links.push({
						x: rect.left + window.pageXOffset,
						y: rect.top + window.pageYOffset,
						w: rect.width,
						h: rect.height,
						href: a.href
					});
				}
			});
			
			// Get all input fields
			document.querySelectorAll('input[type="text"], input[type="search"], input[type="password"], input:not([type]), textarea').forEach((inp, idx) => {
				const rect = inp.getBoundingClientRect();
				if (rect.width > 0 && rect.height > 0) {
					// Generate XPath with multiple fallback strategies
					let xpath = '';
					if (inp.id) {
						xpath = '//*[@id="' + inp.id + '"]';
					} else if (inp.name) {
						const tagName = inp.tagName.toLowerCase();
						xpath = '//' + tagName + '[@name="' + inp.name + '"]';
					} else if (inp.placeholder) {
						const tagName = inp.tagName.toLowerCase();
						xpath = '//' + tagName + '[@placeholder="' + inp.placeholder.replace(/"/g, '\\"') + '"]';
					} else if (inp.className) {
						const tagName = inp.tagName.toLowerCase();
						const firstClass = inp.className.split(' ')[0];
						xpath = '//' + tagName + '[contains(@class,"' + firstClass + '")]';
					} else {
						// Fallback: generate positional XPath
						const tagName = inp.tagName.toLowerCase();
						let parent = inp.parentNode;
						let siblings = Array.from(parent.querySelectorAll(tagName));
						let position = siblings.indexOf(inp) + 1;
						xpath = '//' + tagName + '[' + position + ']';
					}
					
					results.inputs.push({
						x: rect.left + window.pageXOffset,
						y: rect.top + window.pageYOffset,
						w: rect.width,
						h: rect.height,
						name: inp.name || inp.placeholder || 'input',
						xpath: xpath
					});
				}
			});
		} catch(e) {
			results.error = e.toString();
		}
		return JSON.stringify(results);
	}`

	jsonRes, _ := page.Eval(jsScript)
	if jsonRes != nil {
		var data struct {
			Links []struct {
				X, Y, W, H float64
				Href       string `json:"href"`
			} `json:"links"`
			Inputs []struct {
				X, Y, W, H float64
				Name       string `json:"name"`
				XPath      string `json:"xpath"`
			} `json:"inputs"`
		}
		if err := json.Unmarshal([]byte(jsonRes.Value.String()), &data); err == nil {
			for _, l := range data.Links {
				links = append(links, LinkRect{l.X, l.Y, l.W, l.H, l.Href})
			}
			for _, inp := range data.Inputs {
				inputs = append(inputs, InputRect{inp.X, inp.Y, inp.W, inp.H, inp.Name, inp.XPath})
			}
		}
	}

	// 4. Capture Full Page Screenshot
	// We use JPEG with high quality and disable fullPage screenshot (false)
	// because the viewport is already resized to the full height.
	quality := 90
	imageData, err = page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format:  proto.PageCaptureScreenshotFormatJpeg,
		Quality: &quality,
	})
	if err != nil {
		err = fmt.Errorf("failed to capture screenshot: %w", err)
		return
	}

	return imageData, links, inputs, currentURL, nil
}

// SubmitInput navigates to the page, inputs text, optionall presses enter, and captures the result
// SubmitInput navigates to the page, inputs text, optionall presses enter, and captures the result
func (r *Renderer) SubmitInput(ctx context.Context, urlStr, xpath, text string, doEnter bool) ([]byte, []LinkRect, []InputRect, string, error) {
	return r.captureLogic(ctx, urlStr, func(page *rod.Page) error {
		// Wait for element
		// We use a small race free wait?
		// xpath selector
		el, err := page.ElementX(xpath)
		if err != nil {
			return fmt.Errorf("element not found: %w", err)
		}

		// Input text
		if err := el.Input(text); err != nil {
			return fmt.Errorf("failed to input text: %w", err)
		}

		if doEnter {
			if err := page.Keyboard.Press(input.Enter); err != nil {
				return fmt.Errorf("failed to press enter: %w", err)
			}
			// Wait for navigation or change?
			page.WaitLoad()
			// Extra wait for results
			time.Sleep(1 * time.Second)
		}

		return nil
	})
}

// Close closes the browser
func (r *Renderer) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.browser != nil {
		r.browser.Close()
		r.browser = nil
	}
	return nil
}
