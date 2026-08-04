//go:build !darwin

package main

import (
	"runtime"
)

func (a *App) configurePlatformTray(_ []byte, linuxIcon, windowsIcon []byte) {
	trayMenu := a.application.NewMenu()
	a.populateServerMenu(trayMenu)

	tray := a.application.SystemTray.New()
	tray.SetTooltip("DKST RetroProxy")
	if runtime.GOOS == "windows" {
		tray.SetIcon(windowsIcon)
	} else {
		tray.SetIcon(linuxIcon)
	}
	tray.SetMenu(trayMenu)
	tray.OnClick(tray.ShowMenu)
	tray.OnRightClick(func() {})
	a.systemTray = tray
}

func (a *App) updatePlatformTray(_ bool, _ int) {}

func (a *App) destroyPlatformTray() {
	if a.systemTray != nil {
		a.systemTray.Destroy()
	}
}
