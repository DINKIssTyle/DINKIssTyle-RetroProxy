// RendererPool manages a pool of Renderer instances for concurrent page rendering
// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

package main

import (
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

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

	// Create an instance of the app structure
	app := NewApp(config)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "DKST RetroProxy",
		Width:  550,
		Height: 580,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
