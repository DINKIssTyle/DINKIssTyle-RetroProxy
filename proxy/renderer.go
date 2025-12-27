// Created by DINKIssTyle on 2025. Copyright (C) 2025 DINKI'ssTyle. All rights reserved.

package proxy

import (
	"encoding/json"
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

// Renderer uses a headless browser to render modern web pages
type Renderer struct {
	browser *rod.Browser
	mu      sync.Mutex
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
func (r *Renderer) RenderPage(url string) (string, error) {
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
	defer page.Close()

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
			time.Sleep(500 * time.Millisecond)
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

// RenderPageWithScreenshot renders the page and captures a screenshot
func (r *Renderer) RenderPageWithScreenshot(url string) (string, []byte, error) {
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
	defer page.Close()

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
			time.Sleep(500 * time.Millisecond)
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
func (r *Renderer) CaptureScreenshotAndLinks(urlStr string) ([]byte, []LinkRect, []InputRect, string, error) {
	return r.captureLogic(urlStr, nil)
}

// Helper for capture logic to be reused by SubmitInput
func (r *Renderer) captureLogic(urlStr string, interaction func(*rod.Page) error) (imageData []byte, links []LinkRect, inputs []InputRect, currentURL string, err error) {
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
	defer page.Close()

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
			time.Sleep(500 * time.Millisecond)
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

	// 1. Measure Full Height
	// Scroll to bottom to trigger lazy loads
	page.Eval(`() => window.scrollTo(0, document.body.scrollHeight)`)
	time.Sleep(500 * time.Millisecond)
	page.Eval(`() => window.scrollTo(0, 0)`)
	time.Sleep(200 * time.Millisecond)

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
	time.Sleep(200 * time.Millisecond) // Allow layout to settle

	// 3. Extract Coordinates (Now consistent because viewport covers everything)
	jsScript := `() => {
		const results = { links: [], inputs: [] };
		try {
			// Get all links
			document.querySelectorAll('a[href]').forEach(a => {
				const rect = a.getBoundingClientRect();
				if (rect.width > 0 && rect.height > 0) {
					results.links.push({
						x: rect.left, // No scroll needed since viewport is full
						y: rect.top,
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
						x: rect.left,
						y: rect.top,
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
	// We use PNG for raw capture to avoid compression artifacts before slicing
	// Server side will slice and compress to JPEG
	imageData, err = page.Screenshot(true, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		err = fmt.Errorf("failed to capture screenshot: %w", err)
		return
	}

	return imageData, links, inputs, currentURL, nil
}

// SubmitInput navigates to the page, inputs text, optionall presses enter, and captures the result
func (r *Renderer) SubmitInput(urlStr, xpath, text string, doEnter bool) ([]byte, []LinkRect, []InputRect, string, error) {
	return r.captureLogic(urlStr, func(page *rod.Page) error {
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
