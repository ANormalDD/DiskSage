package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	if app == nil {
		log.Fatal("failed to bootstrap app")
	}

	err := wails.Run(&options.App{
		Title:            "DiskSage",
		Width:            1280,
		Height:           840,
		MinWidth:         1024,
		MinHeight:        680,
		DisableResize:    false,
		Frameless:        false,
		StartHidden:      false,
		BackgroundColour: &options.RGBA{R: 15, G: 26, B: 32, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
