// RendererPool manages a pool of Renderer instances for concurrent page rendering
package proxy

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Server represents the proxy server
type Server struct {
	port             int
	server           *http.Server
	rendererPool     *RendererPool
	simplifier       Simplifier
	encoder          *Encoder
	imageConverter   *ImageConverter
	debugMode        bool
	running          bool
	mu               sync.RWMutex
	logger           func(string)
	shutdownCallback func()
	restartCallback  func()

	// Image Map Mode
	proxyMode  string // "html", "text", "image"
	imageTiles map[string][]Tile
	muTiles    sync.RWMutex
}

// NewServer creates a new proxy server
func NewServer() *Server {
	return &Server{
		port:           8080,
		rendererPool:   NewRendererPool(3), // Pool of 3 browser instances
		simplifier:     NewSimplifier320(),
		encoder:        NewEncoder(),
		imageConverter: NewImageConverter(),
		debugMode:      false,
		logger:         func(s string) { fmt.Println(s) },
		proxyMode:      "html",
		imageTiles:     make(map[string][]Tile),
	}
}

// SetLogger sets the logger callback
func (s *Server) SetLogger(logger func(string)) {
	s.logger = logger
}

// SetShutdownCallback sets the shutdown callback
func (s *Server) SetShutdownCallback(cb func()) {
	s.shutdownCallback = cb
}

// SetRestartCallback sets the restart callback
func (s *Server) SetRestartCallback(cb func()) {
	s.restartCallback = cb
}

// log logs a message using the logger callback
func (s *Server) log(msg string) {
	if s.logger != nil {
		s.logger(msg)
	}
}

// Start starts the proxy server on the specified port
func (s *Server) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server is already running")
	}

	s.port = port

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      http.HandlerFunc(s.handleProxy),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log(fmt.Sprintf("Server error: %v", err))
		}
	}()

	s.running = true
	s.log(fmt.Sprintf("Server started on port %d", port))
	return nil
}

// Stop stops the proxy server (graceful with timeout, then force)
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server is not running")
	}

	// Close all browser processes in the pool
	s.rendererPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.server.Shutdown(ctx)
	if err != nil {
		// Graceful shutdown failed, force close
		s.log("Graceful shutdown timeout, forcing close...")
		s.server.Close()
	}

	s.running = false
	s.log("Server stopped")
	return nil
}

// ForceStop immediately stops the server without waiting for connections
func (s *Server) ForceStop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server is not running")
	}

	// Close all browser processes in the pool
	s.rendererPool.Close()

	// Force close immediately
	err := s.server.Close()
	s.running = false
	s.log("Server force stopped")
	return err
}

// IsRunning returns true if the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetPort returns the current port
func (s *Server) GetPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.port
}

// GetEncodings returns available encoding options
func (s *Server) GetEncodings() []EncodingOption {
	return s.encoder.GetAvailableEncodings()
}

// SetEncoding sets the forced encoding
func (s *Server) SetEncoding(encoding string) {
	s.encoder.SetForcedEncoding(encoding)
}

// GetCurrentEncoding returns the current encoding setting
func (s *Server) GetCurrentEncoding() string {
	enc := s.encoder.GetForcedEncoding()
	if enc == "" {
		return "auto"
	}
	return enc
}

// GetImageFormats returns available image format options
func (s *Server) GetImageFormats() []ImageFormatOption {
	return s.imageConverter.GetAvailableFormats()
}

// SetImageFormat sets the image format
func (s *Server) SetImageFormat(format string) {
	s.imageConverter.SetFormat(format)
}

// GetCurrentImageFormat returns the current image format setting
func (s *Server) GetCurrentImageFormat() string {
	return s.imageConverter.GetFormat()
}

// SetDebugMode enables or disables debug mode
func (s *Server) SetDebugMode(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debugMode = enabled
}

// IsDebugMode returns whether debug mode is enabled
func (s *Server) IsDebugMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.debugMode
}

// HTMLVersionOption represents an HTML version option
type HTMLVersionOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// GetHTMLVersions returns available HTML versions
func (s *Server) GetHTMLVersions() []HTMLVersionOption {
	return []HTMLVersionOption{
		{Value: "modern", Label: "Modern (No SSL)"},
		{Value: "3.2", Label: "HTML 3.2 (Legacy, Table Layout)"},
		{Value: "3.2new", Label: "HTML 3.2 New (Layout-based)"},
		{Value: "4.01", Label: "HTML 4.01 (Standard, Div Layout)"},
		{Value: "text", Label: "Text Only (Fast, No Images)"},
		{Value: "image", Label: "Image Map (Full Render, Slow)"},
	}
}

// SetHTMLVersion sets the HTML version simplifier
func (s *Server) SetHTMLVersion(version string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch version {
	case "modern":
		s.proxyMode = "html"
		s.simplifier = NewSimplifierPassthrough()
	case "3.2new":
		s.proxyMode = "layout"
		s.simplifier = NewSimplifier320New()
	case "4.01":
		s.proxyMode = "html"
		s.simplifier = NewSimplifier401()
	case "text":
		s.proxyMode = "text"
		s.simplifier = NewSimplifierText()
	case "image":
		s.proxyMode = "image"
		// Simplifier not used in image mode
	default:
		s.proxyMode = "html"
		s.simplifier = NewSimplifier320() // Default to 3.2
	}
}

// GetCurrentHTMLVersion returns the current version
func (s *Server) GetCurrentHTMLVersion() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check proxyMode first
	if s.proxyMode == "image" {
		return "image"
	}
	if s.proxyMode == "text" {
		return "text"
	}
	if s.proxyMode == "layout" {
		return "3.2new"
	}

	// Check type of s.simplifier
	switch s.simplifier.(type) {
	case *SimplifierPassthrough:
		return "modern"
	case *Simplifier320New:
		return "3.2new"
	case *Simplifier401:
		return "4.01"
	default:
		return "3.2"
	}
}

// generateDebugToolbar creates the debug toolbar HTML to inject into pages
func (s *Server) generateDebugToolbar(currentURL string) string {
	return `<div id="owp-debug-toolbar" style="position:fixed;top:0;left:0;right:0;z-index:999999;background:#1a1d23;color:#fff;padding:8px 15px;font-family:Arial,sans-serif;font-size:12px;display:flex;gap:10px;align-items:center;box-shadow:0 2px 10px rgba(0,0,0,0.5);">
<span style="font-weight:bold;color:#3b82f6;">🌐 DKST RetroProxy</span>
<select id="owp-enc" style="padding:4px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;" onchange="location.href='/_drp/set?enc='+this.value+'&url='+encodeURIComponent('` + currentURL + `')">` + s.getEncodingOptions() + `</select>
<select id="owp-img" style="padding:4px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;" onchange="location.href='/_drp/set?img='+this.value+'&url='+encodeURIComponent('` + currentURL + `')">` + s.getImageOptions() + `</select>
<input id="owp-url" type="text" value="` + currentURL + `" style="flex:1;padding:4px 8px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;">
<button onclick="location.href=document.getElementById('owp-url').value" style="padding:4px 12px;background:#3b82f6;color:#fff;border:none;border-radius:4px;cursor:pointer;">Go</button>
<button onclick="location.reload()" style="padding:4px 12px;background:#22c55e;color:#fff;border:none;border-radius:4px;cursor:pointer;">↻</button>
</div>
<div style="height:45px;"></div>`
}

func (s *Server) getEncodingOptions() string {
	current := s.encoder.GetForcedEncoding()
	if current == "" {
		current = "auto"
	}
	return GenerateOptionsHTML(AvailableEncodings, current)
}

func (s *Server) getImageOptions() string {
	current := s.imageConverter.GetFormat()
	return GenerateOptionsHTML(AvailableImageFormats, current)
}

// handleProxy handles incoming proxy requests
// Standard HTTP proxy: client sends full URL in request line
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log(fmt.Sprintf("Panic in handleProxy: %v", rec))
			http.Error(w, "Internal Server Error (Panic)", http.StatusInternalServerError)
		}
	}()

	// Handle Management Page (setting / server)
	checkHost := r.Host
	if strings.Contains(checkHost, ":") {
		h, _, err := net.SplitHostPort(checkHost)
		if err == nil {
			checkHost = h
		}
	}
	if checkHost == "setting" || checkHost == "settings" || checkHost == "server" ||
		checkHost == "server.com" || checkHost == "www.server.com" {
		s.handleManagementPage(w, r)
		return
	}

	// Handle CONNECT method (for HTTPS tunneling - not supported for rendering)
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}

	// Handle debug view endpoint - /debug?url=...
	if strings.HasPrefix(r.URL.Path, "/debug") {
		s.handleDebugView(w, r)
		return
	}

	// Handle debug API endpoint - /_drp/set
	if strings.HasPrefix(r.URL.Path, "/_drp/set") {
		s.handleDebugAPI(w, r)
		return
	}

	// Handle input UI - /_drp/input
	if strings.HasPrefix(r.URL.Path, "/_drp/input") {
		s.handleInputUI(w, r)
		return
	}

	// Handle input action - /_drp/action_input
	if strings.HasPrefix(r.URL.Path, "/_drp/action_input") {
		s.handleInputAction(w, r)
		return
	}

	// Handle Image Tile endpoint - /_drp/tile/{uuid}/{index}
	if strings.HasPrefix(r.URL.Path, "/_drp/tile/") {
		s.handleImageTile(w, r)
		return
	}

	// Handle Test Logic - /_drp/test/retry
	if strings.HasPrefix(r.URL.Path, "/_drp/test/retry") {
		s.serveRetryPage(w, "/debug", "This is a test of the Busy/Retry page.")
		return
	}

	// Get the target URL
	var targetURL string

	// Check if this is a proxy request (absolute URL) or direct request
	if r.URL.IsAbs() {
		// Standard proxy request: GET http://example.com/path HTTP/1.1
		targetURL = r.URL.String()
	} else if r.Host != "" {
		// Reconstruct URL from Host header
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		targetURL = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.RequestURI())
	} else {
		// Direct access to proxy itself - show help page
		s.serveHomePage(w)
		return
	}

	// Skip favicon and other local requests
	if strings.HasPrefix(r.URL.Path, "/favicon") {
		http.NotFound(w, r)
		return
	}
	s.log(fmt.Sprintf("Proxying: %s", targetURL))

	// Validate URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Check if it's an image request - proxy directly without rendering
	if s.isImageRequest(targetURL) {
		s.proxyImage(w, targetURL)
		return
	}

	// Check if it's CSS, JS, or other non-HTML resources - block them
	if s.isBlockedResource(targetURL) {
		// Return empty response for blocked resources
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// For HTTPS URLs, render with headless browser
	// Check Proxy Mode
	s.mu.RLock()
	mode := s.proxyMode
	s.mu.RUnlock()

	var html string

	if mode == "image" {
		html, err = s.renderImageMode(r.Context(), targetURL, false)
		if err != nil {
			// Check if renderer is busy
			if errors.Is(err, ErrRendererBusy) || errors.Is(err, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			s.serveRetryPage(w, targetURL, fmt.Sprintf("Image Render Failed: %v", err))
			return
		}
		// Return directly (no simplification needed for image mode HTML)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
		return
	}

	// Layout Mode (HTML 3.2 New)
	if mode == "layout" {
		layoutResult, err := s.rendererPool.RenderPageWithLayout(r.Context(), targetURL)
		if err != nil {
			if errors.Is(err, ErrRendererBusy) || errors.Is(err, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			s.serveRetryPage(w, targetURL, fmt.Sprintf("Layout extraction failed: %v", err))
			return
		}

		// Convert layout to HTML 3.2
		simplifier := NewSimplifier320New()
		html = simplifier.SimplifyFromLayout(layoutResult.Elements, targetURL, layoutResult.Title, false)

		// Detect legacy browser and convert encoding if needed
		userAgent := r.Header.Get("User-Agent")
		browserInfo := s.encoder.DetectLegacyBrowser(userAgent)

		var responseBytes []byte
		var contentType string

		if browserInfo.IsLegacy {
			responseBytes, _ = s.encoder.ConvertToEncoding(html, browserInfo.Encoding)
			contentType = "text/html; charset=" + browserInfo.Encoding
		} else {
			responseBytes = []byte(html)
			contentType = "text/html; charset=utf-8"
		}

		w.Header().Set("Content-Type", contentType)
		w.Write(responseBytes)
		return
	}

	// Normal HTML Mode (or Text Mode)
	// Check if we're in Modern mode (need CSS/JS intact)
	s.mu.RLock()
	_, isModern := s.simplifier.(*SimplifierPassthrough)
	s.mu.RUnlock()

	if isModern {
		// Modern mode: use full rendering with CSS/JS
		html, err = s.rendererPool.RenderPageFull(r.Context(), targetURL)
	} else {
		// Other HTML modes: block CSS for faster loading
		html, err = s.rendererPool.RenderPage(r.Context(), targetURL)
	}

	if err != nil {
		// Check if renderer is busy
		if errors.Is(err, ErrRendererBusy) {
			s.serveRetryPage(w, targetURL, "The server is currently busy processing another page.")
			return
		}
		// Fallback: try direct fetch
		html, err = s.fetchDirect(targetURL)
		if err != nil {
			s.serveRetryPage(w, targetURL, fmt.Sprintf("Failed to fetch page: %v", err))
			return
		}
	}

	// Simplify HTML for legacy browsers
	s.mu.RLock()
	simplifier := s.simplifier
	s.mu.RUnlock()

	simplifiedHTML, err := simplifier.Simplify(html, targetURL, false)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to simplify HTML: %v", err), http.StatusInternalServerError)
		return
	}

	// Update links to go through proxy (for standard proxy, links should work as-is)
	simplifiedHTML = s.rewriteLinksForProxy(simplifiedHTML, parsedURL)

	// Inject debug toolbar if debug mode is enabled
	if s.IsDebugMode() {
		toolbar := s.generateDebugToolbar(targetURL)
		// Insert toolbar after <body> tag
		simplifiedHTML = strings.Replace(simplifiedHTML, "<body>", "<body>"+toolbar, 1)
		simplifiedHTML = strings.Replace(simplifiedHTML, "<body ", "<body>"+toolbar+"<div ", 1)
	}

	// Detect legacy browser and convert encoding if needed
	userAgent := r.Header.Get("User-Agent")
	browserInfo := s.encoder.DetectLegacyBrowser(userAgent)

	// Append Footer (Unified)
	if strings.Contains(simplifiedHTML, "</body>") {
		footer := fmt.Sprintf("\n<br><hr><center><font size=\"1\">%s</font></center>", FooterText)
		simplifiedHTML = strings.Replace(simplifiedHTML, "</body>", footer+"</body>", 1)
	}

	var responseBytes []byte
	var contentType string

	if browserInfo.IsLegacy && !s.IsDebugMode() {
		s.log(fmt.Sprintf("Legacy browser detected: %s (encoding: %s)", browserInfo.Name, browserInfo.Encoding))
		responseBytes, _ = s.encoder.ConvertToEncoding(simplifiedHTML, browserInfo.Encoding)
		contentType = "text/html; charset=" + browserInfo.Encoding
	} else {
		responseBytes = []byte(simplifiedHTML)
		contentType = "text/html; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(responseBytes)))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(responseBytes)
}

// handleDebugAPI handles debug toolbar API requests
func (s *Server) handleDebugAPI(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	redirectURL := query.Get("url")
	nextURL := query.Get("next")

	// Update settings based on query parameters
	if enc := query.Get("enc"); enc != "" {
		s.encoder.SetForcedEncoding(enc)
		s.log("Debug: Encoding set to " + enc)
	}
	if img := query.Get("img"); img != "" {
		s.imageConverter.SetFormat(img)
		s.log("Debug: Image format set to " + img)
	}

	// Redirect back to the original URL or next page
	if redirectURL != "" {
		http.Redirect(w, r, "/_drp/view?url="+url.QueryEscape(redirectURL), http.StatusFound)
	} else if nextURL != "" {
		http.Redirect(w, r, nextURL, http.StatusFound)
	} else {
		w.Write([]byte("Settings updated"))
	}
}

// handleDebugView fetches a URL, processes it, and displays with debug toolbar
func (s *Server) handleDebugView(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		// Show debug home page with URL input
		s.serveDebugHomePage(w)
		return
	}

	// Normalize URL: ensure it has a scheme
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "http://" + targetURL
	}

	// Parse mode
	mode := r.URL.Query().Get("mode")

	// Start timing
	start := time.Now()

	s.log(fmt.Sprintf("Debug view: %s (Mode: %s)", targetURL, mode))

	var html string
	var err error
	var stats string

	if mode == "image" {
		// Image Map Mode
		renderStart := time.Now()
		html, err = s.renderImageMode(r.Context(), targetURL, true)
		duration := time.Since(renderStart)
		if err != nil {
			if errors.Is(err, ErrRendererBusy) || errors.Is(err, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			http.Error(w, fmt.Sprintf("Failed to render image mode: %v", err), http.StatusBadGateway)
			return
		}
		stats = fmt.Sprintf("Image Render: %v", duration.Round(time.Millisecond))
		// Note: No simplification needed for image mode as it returns fresh HTML
		// But we need to inject the toolbar.
		// simplifiedHTML is just html here.
	} else if mode == "3.2new" {
		// HTML 3.2 New (Layout-based) Mode
		renderStart := time.Now()
		layoutResult, renderErr := s.rendererPool.RenderPageWithLayout(r.Context(), targetURL)
		renderDuration := time.Since(renderStart)
		if renderErr != nil {
			if errors.Is(renderErr, ErrRendererBusy) || errors.Is(renderErr, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			http.Error(w, fmt.Sprintf("Failed to extract layout: %v", renderErr), http.StatusBadGateway)
			return
		}

		simplifyStart := time.Now()
		simplifier := NewSimplifier320New()
		html = simplifier.SimplifyFromLayout(layoutResult.Elements, targetURL, layoutResult.Title, true)
		simplifyDuration := time.Since(simplifyStart)
		totalDuration := time.Since(start)
		stats = fmt.Sprintf("Render: %v, Simplify: %v, Total: %v", renderDuration.Round(time.Millisecond), simplifyDuration.Round(time.Millisecond), totalDuration.Round(time.Millisecond))
	} else if mode == "modern" {
		// Modern (No SSL) Mode - use RenderPageFull to keep CSS/JS
		renderStart := time.Now()
		renderHTML, renderErr := s.rendererPool.RenderPageFull(r.Context(), targetURL)
		renderDuration := time.Since(renderStart)
		if renderErr != nil {
			if errors.Is(renderErr, ErrRendererBusy) || errors.Is(renderErr, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			http.Error(w, fmt.Sprintf("Failed to render: %v", renderErr), http.StatusBadGateway)
			return
		}

		simplifyStart := time.Now()
		simplifier := NewSimplifierPassthrough()
		simplifiedHTML, simplifyErr := simplifier.Simplify(renderHTML, targetURL, true)
		simplifyDuration := time.Since(simplifyStart)
		if simplifyErr != nil {
			http.Error(w, fmt.Sprintf("Failed to simplify: %v", simplifyErr), http.StatusInternalServerError)
			return
		}
		html = simplifiedHTML
		totalDuration := time.Since(start)
		stats = fmt.Sprintf("Render: %v, Simplify: %v, Total: %v", renderDuration.Round(time.Millisecond), simplifyDuration.Round(time.Millisecond), totalDuration.Round(time.Millisecond))
	} else {
		// Standard HTML 3.2 Mode
		renderStart := time.Now()
		renderHTML, renderErr := s.rendererPool.RenderPage(r.Context(), targetURL)
		renderDuration := time.Since(renderStart)
		if renderErr != nil {
			if errors.Is(renderErr, ErrRendererBusy) || errors.Is(renderErr, context.Canceled) {
				s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
				return
			}
			http.Error(w, fmt.Sprintf("Failed to render: %v", renderErr), http.StatusBadGateway)
			return
		}

		// Simplify HTML
		simplifyStart := time.Now()

		// Choose Simplifier based on mode
		var simplifier Simplifier
		switch mode {
		case "html4":
			simplifier = NewSimplifier401()
		case "text":
			simplifier = NewSimplifierText()
		default: // "html" or empty
			simplifier = NewSimplifier320()
		}

		// Use debugMode=true to rewrite links for the viewer
		// But wait, if we are in debug view, Simplifier rewrites links to /_drp/view?url=...
		// We need it to preserve the 'mode' parameter if possible.
		// Simplifier interface: Simplify(html, url, debugMode).
		// It doesn't know about 'mode'.
		// We might need to post-process links or just let the toolbar global "mode" button handle switches?
		// If user clicks a link in HTML mode, they go to /_drp/view?url=NEW_URL. Mode defaults to empty (HTML).
		// If user wants to browse in Image mode, maybe we need to update Simplifier?
		// Or simpler: Just let them switch mode manually if they want.
		// OR: Update Simplifier to support appending extra query params? Too complex for now.
		// Let's stick to standard behavior: Links reset mode to default (HTML) unless we change Simplifier.
		// However, for Image Mode, 'renderImageMode' generates the map.
		// In previous step, I hardcoded links in renderImageMode to just link to the href.
		// In Debug View, those links will take us OUT of the debug viewer!
		// Logic in renderImageMode:
		// href := link.Href
		// sb.WriteString(... href ...)
		// This means clicking a link in Image Mode Debug View -> Goes to real site (or proxy).
		// It should go to /_drp/view?url=LINK&mode=image

		// I need to intercept this. renderImageMode is generic for Proxy & Debug.
		// If I'm using it for Debug, I need links to be debug links.

		// Challenge: renderImageMode doesn't take 'debugMode' param.
		// I should update renderImageMode signature? Or handle it?
		// Let's first finish handleDebugView structure.

		simplifiedHTML, simplifyErr := simplifier.Simplify(renderHTML, targetURL, true)
		simplifyDuration := time.Since(simplifyStart)
		if simplifyErr != nil {
			http.Error(w, fmt.Sprintf("Failed to simplify: %v", simplifyErr), http.StatusInternalServerError)
			return
		}
		html = simplifiedHTML
		totalDuration := time.Since(start)
		stats = fmt.Sprintf("Render: %v, Simplify: %v, Total: %v", renderDuration.Round(time.Millisecond), simplifyDuration.Round(time.Millisecond), totalDuration.Round(time.Millisecond))
	}

	s.log(fmt.Sprintf("Performance: %s", stats))

	// Inject debug toolbar with stats
	toolbar := s.generateDebugToolbarForView(targetURL, stats, mode)

	// Injection logic
	if strings.Contains(html, "<body") {
		// Insert after body tag start
		// Simple regex-like replacement
		// If <body ...> replace with <body ...>toolbar
		// If <body> replace with <body>toolbar
		// Be careful not to break tags

		// Find <body> or <body ...>
		// We can use a simple replace for standard cases
		if strings.Contains(html, "<body>") {
			html = strings.Replace(html, "<body>", "<body>"+toolbar, 1)
		} else {
			// Try with attributes
			// Replace "<body" with "<body" ... wait, logic is harder.
			// Just prepend to body content?
			// Use generic replacement if simple one fails.
			// Actually my renderImageMode produces <body ...> without >? No, it produces valid HTML.

			// For HTML mode, simplifier produces good HTML.
			// Let's use string replace for <body
			html = strings.Replace(html, "<body ", "<body>"+toolbar+"<div ", 1) // Hacky but usually works if body has attributes
			// Wait, the previous logic was:
			// simplifiedHTML = strings.Replace(simplifiedHTML, "<body>", "<body>"+toolbar, 1)
			// simplifiedHTML = strings.Replace(simplifiedHTML, "<body ", "<body>"+toolbar+"<div ", 1)

			// I'll stick to that logic but apply to 'html' variable.
			html = strings.Replace(html, "<body ", "<body "+toolbar+"<div style='display:none'></div>", 1) // Prevent breaking attributes?
			// Actually standard logic:
			idx := strings.Index(html, "<body")
			if idx != -1 {
				endIdx := strings.Index(html[idx:], ">")
				if endIdx != -1 {
					insertPos := idx + endIdx + 1
					html = html[:insertPos] + toolbar + html[insertPos:]
				}
			}
		}
	} else {
		// No body tag? Prepend.
		html = toolbar + html
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html))
}

// serveDebugHomePage shows a page where user can enter URL to view
func (s *Server) serveDebugHomePage(w http.ResponseWriter) {
	html := `<!DOCTYPE html>
<html>
<head>
<title>DKST RetroProxy - Debug Viewer</title>
<style>
body { font-family: 'Segoe UI', sans-serif; background: #1a1d23; color: #fff; margin: 0; padding: 40px; }
.container { max-width: 800px; margin: 0 auto; }
h1 { color: #3b82f6; font-weight: 300; margin-bottom: 10px; }
.form { background: #2d323a; padding: 30px; border-radius: 12px; margin-top: 30px; box-shadow: 0 4px 6px rgba(0,0,0,0.3); }
input[type="text"] { width: 100%; padding: 15px; font-size: 16px; border: 1px solid #444; border-radius: 8px; background: #1a1d23; color: #fff; box-sizing: border-box; transition: border-color 0.2s; }
input[type="text"]:focus { border-color: #3b82f6; outline: none; }
button { padding: 12px 30px; font-size: 16px; background: #3b82f6; color: #fff; border: none; border-radius: 8px; cursor: pointer; margin-top: 20px; font-weight: 600; transition: background 0.2s; }
button:hover { background: #2563eb; }
.settings { margin-top: 25px; display: grid; grid-template-columns: repeat(2, 1fr); gap: 15px; }
.settings select { width: 100%; padding: 10px; background: #1a1d23; color: #fff; border: 1px solid #444; border-radius: 8px; font-size: 14px; }
.settings label { font-size: 13px; color: #9ca3af; display: block; margin-bottom: 5px; font-weight: 500; }
.internal-links { margin-top: 40px; border-top: 1px solid #374151; padding-top: 30px; }
.internal-links h3 { font-size: 18px; color: #e5e7eb; margin-bottom: 15px; font-weight: 600; }
.internal-links ul { list-style: none; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 10px; }
.internal-links li { margin-bottom: 0; background: #2d323a; border-radius: 8px; padding: 12px 15px; border: 1px solid #374151; transition: border-color 0.2s; }
.internal-links li:hover { border-color: #4b5563; }
.internal-links a { color: #60a5fa; text-decoration: none; font-weight: 500; display: block; }
.internal-links a:hover { text-decoration: underline; }
.internal-links small { color: #9ca3af; display: block; font-size: 0.85em; margin-top: 2px; }
.toast { position: fixed; bottom: 20px; left: 50%; transform: translateX(-50%); background: #22c55e; color: #fff; padding: 10px 20px; border-radius: 20px; font-size: 14px; opacity: 0; transition: opacity 0.3s; pointer-events: none; }
</style>
<script>
function updateSetting(key, value) {
	fetch('/_drp/set?' + key + '=' + value)
		.then(() => {
			const toast = document.getElementById('toast');
			toast.style.opacity = '1';
			setTimeout(() => toast.style.opacity = '0', 2000);
		})
		.catch(console.error);
}
</script>
</head>
<body>
<div class="container">
<h1>🌐 DKST RetroProxy</h1>
<p style="color:#9ca3af;">Enter a URL to view how it will be converted by RetroProxy.</p>

<div class="form">
	<form action="/debug" method="get">
		<input type="text" name="url" placeholder="https://example.com" autofocus>
		
		<div class="settings">
			<div>
				<label>Mode</label>
				<select name="mode">
					` + GenerateOptionsHTML(AvailableHTMLModes, "modern") + `
				</select>
			</div>
			<div>
				<label>Encoding (Global)</label>
				<select onchange="updateSetting('enc', this.value)">
					` + GenerateOptionsHTML(AvailableEncodings, "") + `
				</select>
			</div>
			<div>
				<label>Image Format (Global)</label>
				<select onchange="updateSetting('img', this.value)">
					` + GenerateOptionsHTML(AvailableImageFormats, "") + `
				</select>
			</div>
		</div>
		
		<button type="submit">View Page</button>
	</form>
</div>

<div class="internal-links">
	<h3>📌 Internal Pages Index</h3>
	` + GenerateLinkListHTML(InternalPages) + `
</div>

<div id="toast" class="toast">Settings Saved</div>

<div style="margin-top:20px; color:#6b7280; font-size:12px; text-align:center;">
	DKST RetroProxy v2.0 - Debug Console
</div>

</div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// generateDebugToolbarForView creates toolbar for debug view mode
func (s *Server) generateDebugToolbarForView(currentURL string, stats string, mode string) string {
	modeParam := ""
	if mode != "" {
		modeParam = "&mode=" + mode
	}

	// Determine selected mode for UI (default to html if empty)
	selectedMode := mode
	if selectedMode == "" {
		selectedMode = "html"
	}
	modeOptions := GenerateOptionsHTML(AvailableHTMLModes, selectedMode)

	return `<div id="owp-debug-toolbar" style="position:fixed;top:0;left:0;right:0;z-index:999999;background:#1a1d23;color:#fff;padding:8px 15px;font-family:Arial,sans-serif;font-size:12px;display:flex;gap:10px;align-items:center;box-shadow:0 2px 10px rgba(0,0,0,0.5);">
<a href="/debug" style="font-weight:bold;color:#3b82f6;text-decoration:none;">🌐 DKST RetroProxy</a>
<span style="color:#aaa;font-size:11px;border-left:1px solid #444;padding-left:10px;">` + stats + `</span>
<select onchange="location.href='/debug?url='+encodeURIComponent('` + currentURL + `')+'&mode='+this.value" style="padding:4px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;">` + modeOptions + `</select>
<select onchange="location.href='/_drp/set?enc='+this.value+'&url='+encodeURIComponent('` + currentURL + `')+'` + modeParam + `'" style="padding:4px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;">` + s.getEncodingOptions() + `</select>
<select onchange="location.href='/_drp/set?img='+this.value+'&url='+encodeURIComponent('` + currentURL + `')+'` + modeParam + `'" style="padding:4px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;">` + s.getImageOptions() + `</select>
<input id="owp-url" type="text" value="` + currentURL + `" style="flex:1;padding:4px 8px;background:#2d323a;color:#fff;border:1px solid #444;border-radius:4px;">
<button onclick="location.href='/debug?url='+encodeURIComponent(document.getElementById('owp-url').value)+'&mode=` + mode + `'" style="padding:4px 12px;background:#3b82f6;color:#fff;border:none;border-radius:4px;cursor:pointer;">Go</button>
<button onclick="location.reload()" style="padding:4px 12px;background:#22c55e;color:#fff;border:none;border-radius:4px;cursor:pointer;">↻</button>
</div>
<div style="height:45px;"></div>`
}

// Note: Legacy browsers may not support CONNECT, so we provide a message
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	// For legacy browsers, CONNECT won't work well
	// We'll try to establish a tunnel anyway

	destHost := r.Host
	if !strings.Contains(destHost, ":") {
		destHost += ":443"
	}

	// Try to connect to the destination
	destConn, err := net.DialTimeout("tcp", destHost, 10*time.Second)
	if err != nil {
		http.Error(w, "Failed to connect to destination", http.StatusBadGateway)
		return
	}

	// Send 200 Connection Established
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		destConn.Close()
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		destConn.Close()
		return
	}

	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Tunnel data between client and server
	go func() {
		io.Copy(destConn, clientConn)
		destConn.Close()
	}()
	go func() {
		io.Copy(clientConn, destConn)
		clientConn.Close()
	}()
}

// serveHomePage serves a simple home page when accessing proxy directly
func (s *Server) serveHomePage(w http.ResponseWriter) {
	html := `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
<head>
<title>DKST RetroProxy</title>
</head>
<body>
<h1>DKST RetroProxy</h1>
<p>This proxy renders modern websites for legacy browsers.</p>
<hr>
<h2>Setup Instructions</h2>
<p>Configure your browser to use this proxy:</p>
<ul>
<li><b>Proxy Address:</b> ` + s.getLocalIP() + `</li>
<li><b>Port:</b> ` + fmt.Sprintf("%d", s.port) + `</li>
</ul>

<p>After setting up the proxy, just browse normally!</p>
<hr>
<h2>Test Links</h2>
<ul>
<li><a href="http://example.com">http://example.com</a></li>
<li><a href="http://info.cern.ch">http://info.cern.ch (First website)</a></li>
</ul>
<br><br>
<center><font size="1">%s</font></center>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(html, FooterText)))
}

// serveRetryPage shows a friendly error page that auto-refreshes
func (s *Server) serveRetryPage(w http.ResponseWriter, targetURL string, message string) {
	html := fmt.Sprintf(`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
<head>
<title>Please Wait - DKST RetroProxy</title>
<meta http-equiv="refresh" content="5;url=%s">
</head>
<body bgcolor="#ffffff" text="#000000">
<center>
<h1>Please Try Again</h1>
<hr>
<p><b>%s</b></p>
<p>The page will automatically reload in 5 seconds...</p>
<br>
<a href="%s"><button>Retry Now</button></a>
&nbsp;
<a href="http://server/"><button>Server Settings</button></a>
<br><br>
<font size="1">%s</font>
</center>
</body>
</html>`, targetURL, message, targetURL, FooterText)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(html))
}

// getLocalIP returns the local IP address
func (s *Server) getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "localhost"
}

// isImageRequest checks if the URL is an image request
func (s *Server) isImageRequest(targetURL string) bool {
	lowerURL := strings.ToLower(targetURL)
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".ico", ".svg"}
	for _, ext := range imageExts {
		if strings.Contains(lowerURL, ext) {
			return true
		}
	}
	return false
}

// isBlockedResource checks if the URL is a resource that should be blocked
func (s *Server) isBlockedResource(targetURL string) bool {
	lowerURL := strings.ToLower(targetURL)
	blockedExts := []string{".css", ".js", ".woff", ".woff2", ".ttf", ".eot", ".map"}
	for _, ext := range blockedExts {
		if strings.HasSuffix(lowerURL, ext) {
			return true
		}
	}
	// Block common tracking/analytics
	blockedDomains := []string{"google-analytics.com", "googletagmanager.com", "facebook.net", "doubleclick.net"}
	for _, domain := range blockedDomains {
		if strings.Contains(lowerURL, domain) {
			return true
		}
	}
	return false
}

// proxyImage proxies image requests with optional format conversion
func (s *Server) proxyImage(w http.ResponseWriter, targetURL string) {
	// Use image converter
	imageData, contentType, err := s.imageConverter.FetchAndConvertImage(targetURL)
	if err != nil {
		http.Error(w, "Failed to fetch image", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(imageData)))
	w.WriteHeader(http.StatusOK)
	w.Write(imageData)
}

// fetchDirect fetches a page directly without headless browser
func (s *Server) fetchDirect(targetURL string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// rewriteLinksForProxy modifies links to work with proxy
// For standard HTTP proxy, absolute URLs work automatically
func (s *Server) rewriteLinksForProxy(html string, baseURL *url.URL) string {
	// For standard proxy mode, we don't need to rewrite URLs
	// The browser will send all requests through the proxy automatically
	return html
}

// Close cleans up resources
func (s *Server) Close() error {
	if s.running {
		s.Stop()
	}
	return s.rendererPool.Close()
}

// renderImageMode captures screenshot, slices it, and returns HTML with image map
func (s *Server) renderImageMode(ctx context.Context, targetURL string, debugMode bool) (html string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log(fmt.Sprintf("Panic in renderImageMode: %v", rec))
			err = fmt.Errorf("panic in renderer: %v", rec)
		}
	}()

	// Capture full page screenshot directly (returns []byte now)
	imageData, links, inputs, _, err := s.rendererPool.CaptureScreenshotAndLinks(ctx, targetURL)
	if err != nil {
		s.log(fmt.Sprintf("Error capturing screenshot: %v", err))
		return "", err
	}

	// Slice the full image into tiles on the server side
	// This prevents sticky elements from repeating and ensuring 1:1 coordinates
	const ServerTileHeight = 600
	tiles, width, height, err := SliceImage(imageData, ServerTileHeight)
	if err != nil {
		s.log(fmt.Sprintf("Error slicing image: %v", err))
		return "", err
	}
	_ = height

	// Generate UUID
	uuidBytes := make([]byte, 16)
	crand.Read(uuidBytes)
	uuid := hex.EncodeToString(uuidBytes)

	// Save to cache
	s.muTiles.Lock()
	s.imageTiles[uuid] = tiles
	// TODO: Cleanup old tiles logic
	s.muTiles.Unlock()

	// Build HTML
	var sb strings.Builder
	// Basic HTML 3.2 structure
	sb.WriteString("<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\"><html><body bgcolor='#ffffff' text='#000000' link='#0000ee' vlink='#551a8b' alink='#ff0000' topmargin='0' leftmargin='0' rightmargin='0' bottommargin='0'>")

	// Render Image Tiles
	sb.WriteString(`<div align="center">`)

	// Add fixed refresh button
	sb.WriteString(`<div style="position:fixed;top:5px;left:5px;z-index:9999;">
		<button onclick="window.location.reload()" style="font-size:12px;cursor:pointer;background:#eee;border:1px solid #999;">Refresh View</button>
	</div>`)

	for i, tile := range tiles {
		imgSrc := fmt.Sprintf("/_drp/tile/%s/%d", uuid, i)
		mapName := fmt.Sprintf("map%d-%s", i, uuid[:8]) // Unique map name

		sb.WriteString(fmt.Sprintf(`<map name="%s">`, mapName))

		// Calculate tile bounds
		tileStart := float64(tile.Y)
		tileEnd := tileStart + float64(tile.H)

		// 1. Add Links
		for _, link := range links {
			if link.Y+link.H < tileStart || link.Y > tileEnd {
				continue
			}
			// Calculate relative coordinates
			localY1 := int(link.Y - tileStart)
			localY2 := int(link.Y + link.H - tileStart)
			x1, x2 := int(link.X), int(link.X+link.W)

			// Clip coords
			if localY1 < 0 {
				localY1 = 0
			}
			if localY2 > int(tile.H) {
				localY2 = int(tile.H)
			}
			// Only add if visible
			if localY2 > localY1 {
				href := link.Href
				// Ensure HTTP for compatibility if needed
				if strings.HasPrefix(href, "https://") {
					href = "http://" + strings.TrimPrefix(href, "https://")
				}

				// Debug Mode: Rewrite link to keep user in viewer
				if debugMode {
					href = fmt.Sprintf("/debug?url=%s&mode=image", url.QueryEscape(href))
				}

				sb.WriteString(fmt.Sprintf(`<area shape="rect" coords="%d,%d,%d,%d" href="%s" alt="Link">`, x1, localY1, x2, localY2, href))
			}
		}

		// 2. Add Inputs
		for _, inp := range inputs {
			if inp.Y+inp.H < tileStart || inp.Y > tileEnd {
				continue
			}
			localY1 := int(inp.Y - tileStart)
			localY2 := int(inp.Y + inp.H - tileStart)
			x1, x2 := int(inp.X), int(inp.X+inp.W)

			if localY1 < 0 {
				localY1 = 0
			}
			if localY2 > int(tile.H) {
				localY2 = int(tile.H)
			}

			if localY2 > localY1 {
				// Encode XPath safe for URL
				xp := base64.StdEncoding.EncodeToString([]byte(inp.XPath))

				// Input click action
				// In usage, inputs redirect to /_drp/input page.
				// This page POSTs to /_drp/action_input.
				// For Debug Mode, we want the result of the action to come back to Debug Viewer.
				// Currently /_drp/action_input redirects to the new URL.
				// If we are proxying, correct.
				// If we are debugging, we want to go back to /_drp/view.
				// This is tricky. Input page logic needs to know if it's debug mode.
				// We can pass a 'next' param or 'debug=1'.
				// Adding 'debug=1' to input URL.

				nextParam := ""
				if debugMode {
					nextParam = "&debug=1"
				}

				lnk := fmt.Sprintf("/_drp/input?url=%s&xpath=%s&name=%s%s", url.QueryEscape(targetURL), xp, url.QueryEscape(inp.Name), nextParam)
				sb.WriteString(fmt.Sprintf(`<area shape="rect" coords="%d,%d,%d,%d" href="%s" alt="Input: %s">`, x1, localY1, x2, localY2, lnk, inp.Name))
			}
		}

		sb.WriteString("</map>")

		// Render image with usemap
		// Use border=0 and display:block behavior simulation
		sb.WriteString(fmt.Sprintf(`<img src="%s" width="%d" height="%d" border="0" usemap="#%s"><br>`, imgSrc, width, tile.H, mapName))
	}

	sb.WriteString("</div>")
	sb.WriteString(fmt.Sprintf("\n<br><hr><center><font size=\"1\">%s</font></center>", FooterText))
	sb.WriteString("</body></html>")
	return sb.String(), nil
}

// handleImageTile serves a cached image tile
func (s *Server) handleImageTile(w http.ResponseWriter, r *http.Request) {
	// Path: /_drp/tile/{uuid}/{index}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.NotFound(w, r)
		return
	}
	uuid := parts[3]
	indexStr := parts[4]

	s.muTiles.RLock()
	tiles, ok := s.imageTiles[uuid]
	s.muTiles.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	var idx int
	fmt.Sscanf(indexStr, "%d", &idx)
	if idx < 0 || idx >= len(tiles) {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache aggressively
	w.Write(tiles[idx].Data)
}

// handleManagementPage serves the server configuration page
func (s *Server) handleManagementPage(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	currentEnc := s.encoder.GetForcedEncoding()
	if currentEnc == "" {
		currentEnc = "auto"
	}
	currentHTML := s.GetCurrentHTMLVersion()
	currentDebug := s.debugMode
	currentImg := s.GetCurrentImageFormat()
	s.mu.RUnlock()

	if r.Method == "POST" {
		r.ParseForm()
		action := r.FormValue("act")

		if action == "Shutdown" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body><center><h1>Shutting down...</h1></center></body></html>"))
			go func() {
				time.Sleep(1 * time.Second)
				if s.shutdownCallback != nil {
					s.shutdownCallback()
				}
			}()
			return
		}

		if action == "Restart" {
			// Show restarting message and trigger restart callback
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html>
<head>
<meta http-equiv="refresh" content="3;url=http://server">
</head>
<body>
<center>
<h1>Restarting...</h1>
<p>Please wait. Redirecting automatically in 3 seconds...</p>
<br>
<a href="http://server"><button>Go to Settings</button></a>
</center>
</body>
</html>`))
			go func() {
				time.Sleep(500 * time.Millisecond)
				// Clear caches
				s.muTiles.Lock()
				s.imageTiles = make(map[string][]Tile)
				s.muTiles.Unlock()
				// Trigger restart callback if set
				if s.restartCallback != nil {
					s.restartCallback()
				}
			}()
			return
		}

		// Update Settings
		if r.URL.Path == "/update" {
			enc := r.FormValue("encoding")
			htmlVer := r.FormValue("html")
			imgFmt := r.FormValue("imgfmt")
			debug := r.FormValue("debug")

			s.SetEncoding(enc)
			s.SetHTMLVersion(htmlVer) // Handles modern, 3.2, 4.01, text, image
			s.SetImageFormat(imgFmt)  // Handles original, gif, jpeg, bmp
			s.SetDebugMode(debug == "on")

			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	// Render Page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Helper for selected
	// Generate Options
	encOpts := GenerateOptionsHTML(AvailableEncodings, currentEnc)
	htmlOpts := GenerateOptionsHTML(AvailableHTMLModes, currentHTML)
	imgOpts := GenerateOptionsHTML(AvailableImageFormats, currentImg)

	// Helper for check
	chk := func(curr bool, val string) string {
		if (curr && val == "on") || (!curr && val == "off") {
			return "selected"
		}
		return ""
	}

	debugOpts := fmt.Sprintf(`<option value="on" %s>On</option><option value="off" %s>Off</option>`,
		chk(currentDebug, "on"), chk(currentDebug, "off"))

	html := fmt.Sprintf(`<html>
<head><title>%s</title></head>
<body bgcolor="#ffffff" text="#000000" link="#0000EE" vlink="#551A8B">
<center>
<h1>%s</h1>
<font size="2">Modern websites for legacy browsers</font><br><br>
<form method="POST" action="/update">
<table border="1" cellpadding="5" cellspacing="0">
<tr><td bgcolor="#efefef"><b>Encoding</b></td><td>
<select name="encoding">`+encOpts+`</select>
</td></tr>
<tr><td bgcolor="#efefef"><b>HTML</b></td><td>
<select name="html">`+htmlOpts+`</select>
</td></tr>
<tr><td bgcolor="#efefef"><b>Image</b></td><td>
<select name="imgfmt">`+imgOpts+`</select>
</td></tr>
<tr><td bgcolor="#efefef"><b>Debug</b></td><td>
<select name="debug">`+debugOpts+`</select>
</td></tr>
</table>
<br>
<input type="submit" value="Save Settings">
</form>
<br><hr width="50%"><br>
<form method="POST" action="/action">
<input type="submit" name="act" value="Restart"> &nbsp;
<input type="submit" name="act" value="Shutdown">
</form>
<br><br>
<font size="1">%s</font>
</center>
</body>
</html>`, AppName, AppName, CopyrightText)

	w.Write([]byte(html))
}

// handleInputUI displays the text input form
func (s *Server) handleInputUI(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	xpathEnc := r.URL.Query().Get("xpath")
	name := r.URL.Query().Get("name")
	debugMode := r.URL.Query().Get("debug")

	debugHidden := ""
	if debugMode == "1" {
		debugHidden = `<input type="hidden" name="debug" value="1">`
	}

	html := fmt.Sprintf(`<html>
<head><title>Input Text</title></head>
<body bgcolor="#eeeeee">
<center>
<h3>Input Text</h3>
<form method="POST" action="/_drp/action_input">
<input type="hidden" name="url" value="%s">
<input type="hidden" name="xpath" value="%s">
%s
Field: <b>%s</b><br><br>
<input type="text" name="text" size="40"><br><br>
<input type="submit" name="act" value="Input Only">
<input type="submit" name="act" value="Input & Enter">
<input type="submit" name="act" value="Cancel">
</form>
</center>
</body>
</html>`, targetURL, xpathEnc, debugHidden, name)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// handleInputAction processes the input submission
func (s *Server) handleInputAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	targetURL := r.FormValue("url")
	xpathEnc := r.FormValue("xpath")
	text := r.FormValue("text")
	action := r.FormValue("act")
	debugMode := r.FormValue("debug") == "1"

	if action == "Cancel" {
		if debugMode {
			http.Redirect(w, r, fmt.Sprintf("/debug?url=%s&mode=image", url.QueryEscape(targetURL)), http.StatusFound)
		} else {
			http.Redirect(w, r, targetURL, http.StatusFound)
		}
		return
	}

	xpathBytes, _ := base64.StdEncoding.DecodeString(xpathEnc)
	xpath := string(xpathBytes)
	doEnter := (action == "Input & Enter")

	// Perform interaction via renderer pool
	imageData, links, inputs, newURL, err := s.rendererPool.SubmitInput(r.Context(), targetURL, xpath, text, doEnter)
	if err != nil {
		if errors.Is(err, ErrRendererBusy) || errors.Is(err, context.Canceled) {
			s.serveRetryPage(w, targetURL, "The server prevents duplicate heavy tasks. Please try again.")
			return
		}
		http.Error(w, fmt.Sprintf("Interaction failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Determine final URL for redirects
	finalURL := targetURL
	if newURL != "" && newURL != targetURL {
		finalURL = newURL
	}

	// If in debug mode, always redirect back to debug viewer
	if debugMode {
		http.Redirect(w, r, fmt.Sprintf("/debug?url=%s&mode=image", url.QueryEscape(finalURL)), http.StatusFound)
		return
	}

	// If URL changed significantly and Enter was pressed, redirect to new URL
	if newURL != "" && newURL != targetURL && doEnter {
		// Legacy Browser Compat: Force HTTP scheme
		if strings.HasPrefix(strings.ToLower(newURL), "https://") {
			newURL = "http://" + strings.TrimPrefix(newURL, "https://")
		}
		http.Redirect(w, r, newURL, http.StatusFound)
		return
	}

	// Slice image server-side
	const ServerTileHeight = 600
	tiles, width, height, err := SliceImage(imageData, ServerTileHeight)
	if err != nil {
		http.Error(w, fmt.Sprintf("Slicing failed: %v", err), http.StatusInternalServerError)
		return
	}
	_ = height

	// Render the result
	// Generate UUID
	uuidBytes := make([]byte, 16)
	crand.Read(uuidBytes)
	uuid := hex.EncodeToString(uuidBytes)

	s.muTiles.Lock()
	s.imageTiles[uuid] = tiles
	s.muTiles.Unlock()

	// Build HTML response manually for efficiency
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE HTML PUBLIC \"-//W3C//DTD HTML 3.2 Final//EN\"><html><body bgcolor='#ffffff' text='#000000' link='#0000ee' vlink='#551a8b' alink='#ff0000' topmargin='0' leftmargin='0' rightmargin='0' bottommargin='0'>")

	sb.WriteString(`<div align="center">`)
	for i, tile := range tiles {

		mapName := fmt.Sprintf("map%d", i)
		sb.WriteString(fmt.Sprintf("<img src='/_drp/tile/%s/%d' width='%d' border='0' usemap='#%s'><br>", uuid, i, width, mapName))
		sb.WriteString(fmt.Sprintf("<map name='%s'>", mapName))
		tileStart := float64(tile.Y)
		tileEnd := tileStart + float64(tile.H)

		for _, link := range links {
			if link.Y+link.H < tileStart || link.Y > tileEnd {
				continue
			}
			localY1 := int(link.Y - tileStart)
			localY2 := int(link.Y + link.H - tileStart)
			x1, x2 := int(link.X), int(link.X+link.W)
			if localY1 < 0 {
				localY1 = 0
			}
			if localY2 > int(tile.H) {
				localY2 = int(tile.H)
			}
			href := link.Href
			if strings.HasPrefix(href, "https://") {
				href = "http://" + strings.TrimPrefix(href, "https://")
			}
			sb.WriteString(fmt.Sprintf("<area shape='rect' coords='%d,%d,%d,%d' href='%s' alt='Link'>", x1, localY1, x2, localY2, href))
		}
		for _, inp := range inputs {
			if inp.Y+inp.H < tileStart || inp.Y > tileEnd {
				continue
			}
			localY1 := int(inp.Y - tileStart)
			localY2 := int(inp.Y + inp.H - tileStart)
			x1, x2 := int(inp.X), int(inp.X+inp.W)
			if localY1 < 0 {
				localY1 = 0
			}
			if localY2 > int(tile.H) {
				localY2 = int(tile.H)
			}
			xp := base64.StdEncoding.EncodeToString([]byte(inp.XPath))
			lnk := fmt.Sprintf("/_drp/input?url=%s&xpath=%s&name=%s", url.QueryEscape(targetURL), xp, url.QueryEscape(inp.Name))
			sb.WriteString(fmt.Sprintf("<area shape='rect' coords='%d,%d,%d,%d' href='%s' alt='Input: %s'>", x1, localY1, x2, localY2, lnk, inp.Name))
		}
		sb.WriteString("</map>")
	}
	sb.WriteString("</div>")
	sb.WriteString(fmt.Sprintf("\n<br><hr><center><font size=\"1\">%s</font></center>", FooterText))
	sb.WriteString("</body></html>")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(sb.String()))
}
