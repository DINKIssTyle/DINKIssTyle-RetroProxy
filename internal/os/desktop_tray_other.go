//go:build !darwin

package os

import (
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type TrayCallbacks struct {
	OnToggleServer   func()
	OnShowMainWindow func()
	OnQuit           func()
}

type TrayManager struct {
	systemTray *application.SystemTray
}

var globalTray *TrayManager

func ConfigurePlatformTray(app *application.App, populateMenu func(*application.Menu), _ TrayCallbacks, _, linuxIcon, windowsIcon []byte) {
	trayMenu := app.NewMenu()
	populateMenu(trayMenu)

	tray := app.SystemTray.New()
	tray.SetTooltip("DKST RetroProxy")
	if runtime.GOOS == "windows" {
		tray.SetIcon(windowsIcon)
	} else {
		tray.SetIcon(linuxIcon)
	}
	tray.SetMenu(trayMenu)
	tray.OnClick(tray.ShowMenu)
	tray.OnRightClick(func() {})
	globalTray = &TrayManager{systemTray: tray}
}

func UpdatePlatformTray(_ bool, _ int) {}

func DestroyPlatformTray() {
	if globalTray != nil && globalTray.systemTray != nil {
		globalTray.systemTray.Destroy()
		globalTray = nil
	}
}
