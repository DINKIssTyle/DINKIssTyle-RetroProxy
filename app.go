// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package main

import (
	"context"
	"fmt"
	"oldwebproxy/proxy"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AppConfig holds initial configuration from command line
type AppConfig struct {
	StartServer bool
	Port        int
	Encoding    string
	Mode        string
	ImageFormat string
	StopServer  bool
	QuitApp     bool
	SetFlags    map[string]bool // Tracks which flags were actually provided
}

// App struct
type App struct {
	application *application.App
	mainWindow  application.Window
	systemTray  *application.SystemTray
	server      *proxy.Server
	logs        []string
	logMu       sync.RWMutex
	menuMu      sync.Mutex
	statusItems []*application.MenuItem
	toggleItems []*application.MenuItem
	config      AppConfig
}

// NewApp creates a new App application struct
func NewApp(config AppConfig) *App {
	return &App{
		server: proxy.NewServer(),
		logs:   make([]string, 0, 100),
		config: config,
	}
}

// ServiceStartup is called by Wails when the application starts.
func (a *App) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	a.server.SetLogger(a.addLog)
	a.server.SetShutdownCallback(func() {
		a.application.Quit()
	})
	a.server.SetRestartCallback(func() {
		port := a.server.GetPort()
		a.addLog("Restarting server...")
		a.server.ForceStop()
		a.server.Start(port)
		a.addLog("Server restarted")
		a.updateServerMenus()
	})

	// Apply initial settings from config (only if explicitly set or non-default)
	if a.config.SetFlags["e"] {
		a.server.SetEncoding(a.config.Encoding)
		a.addLog(fmt.Sprintf("Initial encoding set to: %s", a.config.Encoding))
	}

	if a.config.SetFlags["i"] {
		a.server.SetImageFormat(a.config.ImageFormat)
		a.addLog(fmt.Sprintf("Initial image format set to: %s", a.config.ImageFormat))
	}

	if a.config.SetFlags["m"] {
		internalMode := ""
		switch a.config.Mode {
		case "nossl":
			internalMode = "modern"
		case "html32":
			internalMode = "3.2"
		case "html32new":
			internalMode = "3.2new"
		case "html401":
			internalMode = "4.01"
		case "textonly":
			internalMode = "text"
		case "imagemap":
			internalMode = "image"
		default:
			internalMode = a.config.Mode
		}
		a.server.SetHTMLVersion(internalMode)
		a.addLog(fmt.Sprintf("Initial mode set to: %s (%s)", a.config.Mode, internalMode))
	}

	a.addLog("Application started")

	// Quit if requested
	if a.config.QuitApp {
		a.addLog("Quit requested via command line")
		a.application.Quit()
		return nil
	}

	// Start proxy if requested (and not stopping)
	// For backward compatibility or ease of use, we check StartServer even if not in SetFlags,
	// but usually user will pass -start to make it true.
	if a.config.StartServer && !a.config.StopServer {
		port := a.config.Port
		// If port 8080 is default but user didn't set it, it's still 8080.
		err := a.server.Start(port)
		if err != nil {
			a.addLog(fmt.Sprintf("Failed to auto-start server: %v", err))
		} else {
			a.addLog(fmt.Sprintf("Proxy server auto-started on port %d", port))
		}
	}
	a.updateServerMenus()

	return nil
}

// ServiceShutdown is called by Wails when the application is closing.
func (a *App) ServiceShutdown() error {
	a.addLog("Application shutting down...")
	a.destroyPlatformTray()
	if a.server != nil {
		a.server.Close()
	}
	return nil
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
	a.updateServerMenus()
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
	a.updateServerMenus()
	return msg
}

// GetLaunchAtStartup reports whether the application is registered to launch
// when the current user signs in.
func (a *App) GetLaunchAtStartup() (bool, error) {
	if a.application == nil || a.application.Autostart == nil {
		return false, fmt.Errorf("autostart is not available")
	}
	return a.application.Autostart.IsEnabled()
}

// SetLaunchAtStartup updates the operating system's per-user startup entry.
func (a *App) SetLaunchAtStartup(enabled bool) error {
	if a.application == nil || a.application.Autostart == nil {
		return fmt.Errorf("autostart is not available")
	}

	var err error
	if enabled {
		err = a.application.Autostart.Enable()
	} else {
		err = a.application.Autostart.Disable()
	}
	if err != nil {
		a.addLog(fmt.Sprintf("Failed to update launch at startup: %v", err))
		return err
	}

	if enabled {
		a.addLog("Launch at startup enabled")
	} else {
		a.addLog("Launch at startup disabled")
	}
	return nil
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
