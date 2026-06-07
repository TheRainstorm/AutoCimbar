//go:build windows

package backend

import (
	"io/fs"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func RunGUI(lite bool, assets fs.FS, icon []byte) {
	appSvc := NewAppService()
	configSvc := NewConfigServiceWithMode(lite)
	encoderSvc := NewEncoderServiceWithMode(lite)
	decoderSvc := NewDecoderServiceWithMode(lite)

	name := "AutoCimBar"
	title := "AutoCimBar"
	if lite {
		name = "AutoCimBar Lite"
		title = "AutoCimBar Lite"
	}

	wailsApp := application.New(application.Options{
		Name:        name,
		Description: "High-speed QR screen channel transfer tool",
		Icon:        icon,
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
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	mainWindow := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     title,
		Width:     1080,
		Height:    720,
		MinWidth:  900,
		MinHeight: 620,
		URL:       "/",
	})
	appSvc.Attach(wailsApp, mainWindow)
	encoderSvc.Attach(wailsApp)
	decoderSvc.Attach(wailsApp)
	ConfigureSystemTray(wailsApp, mainWindow, icon)
	mainWindow.Show()

	if err := wailsApp.Run(); err != nil {
		println(err.Error())
		os.Exit(1)
	}
}
