package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// configureDesktop installs the native application menu and system tray menu.
// Both menus are backed by separate Wails menu items so each platform can own
// and update its native menu safely.
func (a *App) configureDesktop(window application.Window, macIcon, linuxIcon, windowsIcon []byte) {
	a.mainWindow = window

	applicationMenu := a.application.NewMenu()
	serverMenu := applicationMenu.AddSubmenu("Server")
	a.populateServerMenu(serverMenu)
	a.application.Menu.SetApplicationMenu(applicationMenu)

	a.configurePlatformTray(macIcon, linuxIcon, windowsIcon)

	a.updateServerMenus()
}

func (a *App) populateServerMenu(menu *application.Menu) {
	statusItem := menu.Add("Server Status: Stopped").SetEnabled(false)
	toggleItem := menu.Add("Start Server").OnClick(func(_ *application.Context) {
		a.toggleServerFromMenu()
	})
	menu.Add("Show Main Window").OnClick(func(_ *application.Context) {
		a.ShowMainWindow()
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(_ *application.Context) {
		a.application.Quit()
	})

	a.statusItems = append(a.statusItems, statusItem)
	a.toggleItems = append(a.toggleItems, toggleItem)
}

func (a *App) toggleServerFromMenu() {
	if a.server.IsRunning() {
		a.StopProxy()
		return
	}
	a.StartProxy(a.server.GetPort())
}

func (a *App) updateServerMenus() {
	running := a.server.IsRunning()
	port := a.server.GetPort()

	statusLabel := "Server Status: Stopped"
	toggleLabel := "Start Server"
	if running {
		statusLabel = fmt.Sprintf("Server Status: Running (Port %d)", port)
		toggleLabel = "Stop Server"
	}

	a.menuMu.Lock()
	defer a.menuMu.Unlock()
	for _, item := range a.statusItems {
		item.SetLabel(statusLabel)
	}
	for _, item := range a.toggleItems {
		item.SetLabel(toggleLabel)
	}
	a.updatePlatformTray(running, port)
}

// ShowMainWindow restores the main application window from the taskbar, Dock,
// or tray and brings it to the foreground.
func (a *App) ShowMainWindow() {
	if a.mainWindow == nil {
		return
	}
	a.mainWindow.Show()
	a.mainWindow.Restore()
	a.mainWindow.Focus()
}
