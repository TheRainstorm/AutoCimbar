//go:build windows

package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/autocambar/autocambar/cmd/gui/internal/backend"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	appSvc := backend.NewAppService()
	configSvc := backend.NewConfigService()
	encoderSvc := backend.NewEncoderService()
	decoderSvc := backend.NewDecoderService()

	wailsApp := application.New(application.Options{
		Name:        "AutoCimBar",
		Description: "High-speed QR screen channel transfer tool",
		LogLevel:    slog.LevelInfo,
		Services: []application.Service{
			application.NewService(appSvc),
			application.NewService(configSvc),
			application.NewService(encoderSvc),
			application.NewService(decoderSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	mainWindow := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "AutoCimBar",
		Width:     1280,
		Height:    820,
		MinWidth:  1040,
		MinHeight: 700,
		URL:       "/",
	})
	appSvc.Attach(wailsApp, mainWindow)
	encoderSvc.Attach(wailsApp)
	decoderSvc.Attach(wailsApp)
	backend.ConfigureSystemTray(wailsApp, mainWindow)
	mainWindow.Show()

	if err := wailsApp.Run(); err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
