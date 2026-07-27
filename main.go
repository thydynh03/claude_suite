package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"agent_center/backend/logger"

	"github.com/joho/godotenv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Startup diagnostics go to the log file as well as stdout: a release build
	// is compiled with -H windowsgui, which has no console at all, so anything
	// printed here is discarded exactly when a user needs it most.
	logger.Info("Agent Center starting")
	fmt.Println("=== Agent Center STARTING ===")
	_ = godotenv.Load() // Load .env file if it exists
	app := NewApp()

	subFS, errFS := fs.Sub(assets, "frontend/dist")
	if errFS != nil {
		logger.Error(fmt.Sprintf("embedded frontend missing: %v", errFS))
		fmt.Println("Failed to load embedded frontend files:", errFS)
		os.Exit(1)
	}

	err := wails.Run(&options.App{
		Title:     "Agent Center",
		Width:     1280,
		Height:    800,
		MinWidth:  1024,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: subFS,
		},
		BackgroundColour: &options.RGBA{R: 247, G: 249, B: 251, A: 1},
		OnStartup:        app.startup,
		// The frontend registers its event listeners as it mounts, and Wails
		// does not queue events for a listener that is not there yet — so every
		// warning startup emitted (a data file that could not be decrypted,
		// tasks recovered from a killed session) was fired into the void.
		OnDomReady: app.domReady,
		Windows: &windows.Options{
			// One instance per machine. On a fresh install Defender scans the
			// new exe and several seconds pass with no window, so the user
			// double-clicks again — and two full instances then shared one
			// SQLite file, each resetting the other's running tasks back to
			// backlog and dispatching a second agent into the same workspace.
			// A second launch raises the window that already exists.
			WebviewIsTransparent: false,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "agent-center-thydynh03",
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		logger.Error(fmt.Sprintf("wails.Run failed: %v", err))
		fmt.Println("Wails Run Error:", err.Error())
		os.Exit(1)
	}
	fmt.Println("=== Agent Center EXITING ===")
}
