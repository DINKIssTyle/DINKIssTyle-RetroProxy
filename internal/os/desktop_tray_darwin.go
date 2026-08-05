//go:build darwin

package os

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

	"github.com/wailsapp/wails/v3/pkg/application"
)

type TrayCallbacks struct {
	OnToggleServer   func()
	OnShowMainWindow func()
	OnQuit           func()
}

var callbacks TrayCallbacks

func ConfigurePlatformTray(_ *application.App, _ func(*application.Menu), cb TrayCallbacks, macIcon, _, _ []byte) {
	callbacks = cb
	if len(macIcon) == 0 {
		C.DRPInitStatusItem(nil, 0)
		return
	}
	C.DRPInitStatusItem(
		(*C.uchar)(unsafe.Pointer(&macIcon[0])),
		C.int(len(macIcon)),
	)
}

func UpdatePlatformTray(running bool, port int) {
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

func DestroyPlatformTray() {
	C.DRPRemoveStatusItem()
	callbacks = TrayCallbacks{}
}

//export DRPTrayToggleServer
func DRPTrayToggleServer() {
	if callbacks.OnToggleServer != nil {
		go callbacks.OnToggleServer()
	}
}

//export DRPTrayShowMainWindow
func DRPTrayShowMainWindow() {
	if callbacks.OnShowMainWindow != nil {
		callbacks.OnShowMainWindow()
	}
}

//export DRPTrayQuit
func DRPTrayQuit() {
	if callbacks.OnQuit != nil {
		callbacks.OnQuit()
	}
}
