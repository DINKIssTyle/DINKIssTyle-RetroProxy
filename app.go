package main

import (
	"context"
	"fmt"
	"oldwebproxy/proxy"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx    context.Context
	server *proxy.Server
	logs   []string
	logMu  sync.RWMutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		server: proxy.NewServer(),
		logs:   make([]string, 0, 100),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.server.SetLogger(a.addLog)
	a.server.SetShutdownCallback(func() {
		runtime.Quit(a.ctx)
	})
	a.server.SetRestartCallback(func() {
		port := a.server.GetPort()
		a.addLog("Restarting server...")
		a.server.ForceStop()
		a.server.Start(port)
		a.addLog("Server restarted")
	})
	a.addLog("Application started")
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.addLog("Application shutting down...")
	if a.server != nil {
		a.server.Close()
	}
}

// addLog adds a log message
func (a *App) addLog(msg string) {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	// Keep only last 100 logs
	if len(a.logs) >= 100 {
		a.logs = a.logs[1:]
	}
	a.logs = append(a.logs, msg)
}

// GetLogs returns recent log messages
func (a *App) GetLogs() []string {
	a.logMu.RLock()
	defer a.logMu.RUnlock()

	result := make([]string, len(a.logs))
	copy(result, a.logs)
	return result
}

// ClearLogs clears all logs
func (a *App) ClearLogs() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	a.logs = make([]string, 0, 100)
}

// StartProxy starts the proxy server on the specified port
func (a *App) StartProxy(port int) string {
	if a.server.IsRunning() {
		return "Server is already running"
	}

	err := a.server.Start(port)
	if err != nil {
		msg := fmt.Sprintf("Failed to start server: %v", err)
		a.addLog(msg)
		return msg
	}

	msg := fmt.Sprintf("Proxy server started on port %d", port)
	a.addLog(msg)
	return msg
}

// StopProxy stops the proxy server
func (a *App) StopProxy() string {
	if !a.server.IsRunning() {
		return "Server is not running"
	}

	err := a.server.Stop()
	if err != nil {
		msg := fmt.Sprintf("Failed to stop server: %v", err)
		a.addLog(msg)
		return msg
	}

	msg := "Proxy server stopped"
	a.addLog(msg)
	return msg
}

// GetStatus returns the current server status
func (a *App) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"running": a.server.IsRunning(),
		"port":    a.server.GetPort(),
	}
}

// IsRunning returns whether the proxy server is running
func (a *App) IsRunning() bool {
	return a.server.IsRunning()
}

// GetEncodings returns the available encoding options for the dropdown
func (a *App) GetEncodings() []proxy.EncodingOption {
	return a.server.GetEncodings()
}

// SetEncoding sets the forced encoding (or "auto" for auto-detect)
func (a *App) SetEncoding(encoding string) string {
	a.server.SetEncoding(encoding)
	if encoding == "auto" {
		a.addLog("Encoding set to: Auto Detect")
		return "Encoding set to Auto Detect"
	}
	a.addLog("Encoding set to: " + encoding)
	return "Encoding set to " + encoding
}

// GetCurrentEncoding returns the current encoding setting
func (a *App) GetCurrentEncoding() string {
	return a.server.GetCurrentEncoding()
}

// GetImageFormats returns available image format options
func (a *App) GetImageFormats() []proxy.ImageFormatOption {
	return a.server.GetImageFormats()
}

// SetImageFormat sets the image format
func (a *App) SetImageFormat(format string) string {
	a.server.SetImageFormat(format)
	if format == "original" {
		a.addLog("Image format set to: Original")
		return "Image format set to Original"
	}
	a.addLog("Image format set to: " + format)
	return "Image format set to " + format
}

// GetCurrentImageFormat returns the current image format setting
func (a *App) GetCurrentImageFormat() string {
	return a.server.GetCurrentImageFormat()
}

// SetDebugMode enables or disables debug mode
func (a *App) SetDebugMode(enabled bool) string {
	a.server.SetDebugMode(enabled)
	if enabled {
		a.addLog("Debug mode enabled - toolbar will appear on proxied pages")
		return "Debug mode enabled"
	}
	a.addLog("Debug mode disabled")
	return "Debug mode disabled"
}

// IsDebugMode returns whether debug mode is enabled
func (a *App) IsDebugMode() bool {
	return a.server.IsDebugMode()
}

// GetHTMLVersions returns available HTML version options
func (a *App) GetHTMLVersions() []proxy.HTMLVersionOption {
	return a.server.GetHTMLVersions()
}

// SetHTMLVersion sets the HTML version simplifier
func (a *App) SetHTMLVersion(version string) string {
	a.server.SetHTMLVersion(version)
	a.addLog("HTML Version set to: " + version)
	return "HTML Version set to " + version
}

// GetCurrentHTMLVersion returns the current HTML version
func (a *App) GetCurrentHTMLVersion() string {
	return a.server.GetCurrentHTMLVersion()
}
