// RendererPool manages a pool of Renderer instances for concurrent page rendering
// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Replace these platform-specific placeholders with final tray artwork while
// keeping the file names and formats unchanged.
//
//go:embed build/darwin/trayicon.png
var trayIconDarwinPNG []byte

//go:embed build/linux/trayicon.png
var trayIconLinuxPNG []byte

//go:embed build/windows/trayicon.ico
var trayIconWindowsICO []byte

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Parse command line flags
	start := flag.Bool("start", false, "Start proxy server on startup")
	port := flag.Int("p", 8080, "Port for the proxy server")
	encoding := flag.String("e", "auto", "Forced encoding (e.g. utf-8, euc-kr)")
	mode := flag.String("m", "html32", "Proxy mode (nossl, html32, html32new, html401, textonly, imagemap)")
	image := flag.String("i", "original", "Image conversion format (original, jpeg, gif, png)")
	stop := flag.Bool("stop", false, "Do not start server even if -start is passed")
	quit := flag.Bool("quit", false, "Quit application after processing flags")
	flag.Parse()

	// Handle remote control commands if requested
	if *stop || *quit {
		action := "stop"
		if *quit {
			action = "quit"
		}

		fmt.Printf("Sending %s command to running instance on port %d...\n", action, *port)

		client := &http.Client{Timeout: 5 * time.Second}
		controlURL := fmt.Sprintf("http://localhost:%d/_drp/control?action=%s", *port, action)
		resp, err := client.Get(controlURL)

		if err != nil {
			fmt.Printf("Error: Could not connect to running instance on port %d: %v\n", *port, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			fmt.Printf("Success: %s command accepted\n", action)
			os.Exit(0)
		} else {
			fmt.Printf("Error: Server returned status %d\n", resp.StatusCode)
			os.Exit(1)
		}
	}

	config := AppConfig{
		StartServer: *start,
		Port:        *port,
		Encoding:    *encoding,
		Mode:        *mode,
		ImageFormat: *image,
		StopServer:  *stop,
		QuitApp:     *quit,
		SetFlags:    make(map[string]bool),
	}

	// Track which flags were explicitly provided by the user
	flag.Visit(func(f *flag.Flag) {
		config.SetFlags[f.Name] = true
	})

	appService := NewApp(config)

	app := application.New(application.Options{
		Name:        "DKST RetroProxy",
		Description: "Modern websites for legacy browsers",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	appService.application = app

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "DKST RetroProxy",
		Width:            600,
		Height:           650,
		MinWidth:         430,
		MinHeight:        520,
		Frameless:        true,
		BackgroundColour: application.NewRGBA(255, 255, 255, 255),
		URL:              "/",
		Mac: application.MacWindow{
			CornerType: application.MacWindowCornerTypeSquare,
		},
		Windows: application.WindowsWindow{
			NonClientRegionSupport:            true,
			DisableFramelessWindowDecorations: true,
		},
	})
	appService.configureDesktop(mainWindow, trayIconDarwinPNG, trayIconLinuxPNG, trayIconWindowsICO)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
