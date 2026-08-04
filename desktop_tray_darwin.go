//go:build darwin

package main

/*
#include <stdlib.h>
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Cocoa
void DRPInitStatusItem(const unsigned char *iconBytes, int iconLength);
void DRPUpdateStatusItem(const char *statusTitle, const char *toggleTitle);
void DRPRemoveStatusItem(void);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

var darwinTrayApp *App

func (a *App) configurePlatformTray(macIcon, _, _ []byte) {
	darwinTrayApp = a
	if len(macIcon) == 0 {
		C.DRPInitStatusItem(nil, 0)
		return
	}
	C.DRPInitStatusItem(
		(*C.uchar)(unsafe.Pointer(&macIcon[0])),
		C.int(len(macIcon)),
	)
}

func (a *App) updatePlatformTray(running bool, port int) {
	statusLabel := "Server Status: Stopped"
	toggleLabel := "Start Server"
	if running {
		statusLabel = fmt.Sprintf("Server Status: Running (Port %d)", port)
		toggleLabel = "Stop Server"
	}

	status := C.CString(statusLabel)
	toggle := C.CString(toggleLabel)
	defer C.free(unsafe.Pointer(status))
	defer C.free(unsafe.Pointer(toggle))
	C.DRPUpdateStatusItem(status, toggle)
}

func (a *App) destroyPlatformTray() {
	C.DRPRemoveStatusItem()
	darwinTrayApp = nil
}

//export DRPTrayToggleServer
func DRPTrayToggleServer() {
	if darwinTrayApp != nil {
		go darwinTrayApp.toggleServerFromMenu()
	}
}

//export DRPTrayShowMainWindow
func DRPTrayShowMainWindow() {
	if darwinTrayApp != nil {
		darwinTrayApp.ShowMainWindow()
	}
}

//export DRPTrayQuit
func DRPTrayQuit() {
	if darwinTrayApp != nil {
		darwinTrayApp.application.Quit()
	}
}
