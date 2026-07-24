package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	fmt.Println("=== CLAUDE SUITE STARTING ===")
	app := NewApp()
	fmt.Println("=== APP CREATED SUCCESSFULLY ===")

	subFS, errFS := fs.Sub(assets, "frontend/dist")
	if errFS != nil {
		fmt.Println("Failed to load embedded frontend files:", errFS)
		os.Exit(1)
	}

	err := wails.Run(&options.App{
		Title:     "Claude Suite — Agent Control Center",
		Width:     1280,
		Height:    800,
		MinWidth:  1024,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: subFS,
		},
		BackgroundColour: &options.RGBA{R: 247, G: 249, B: 251, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		fmt.Println("Wails Run Error:", err.Error())
		os.Exit(1)
	}
	fmt.Println("=== CLAUDE SUITE EXITING ===")
}
