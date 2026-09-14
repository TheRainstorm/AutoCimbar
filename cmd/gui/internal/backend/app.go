//go:build windows

package backend

import (
	"errors"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/events"
	"os"
	"path/filepath"
	"sync"

	coreapp "github.com/autocambar/autocambar/pkg/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type AppService struct {
	app        *application.App
	window     *application.WebviewWindow
	logMu      sync.Mutex
	logWindows map[string]*application.WebviewWindow
}

func NewAppService() *AppService {
	return &AppService{}
}

func (s *AppService) Attach(app *application.App, window *application.WebviewWindow) {
	s.app = app
	s.window = window
}

func (s *AppService) GetAppInfo() AppInfo {
	return AppInfo{Name: "AutoCimBar", Version: "1.0.0"}
}

func (s *AppService) SelectFileToSend() (SelectedFile, error) {
	if s.app == nil {
		return SelectedFile{}, errors.New("application is not ready")
	}
	path, err := s.app.Dialog.OpenFile().
		SetTitle("Select file to send").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return SelectedFile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return SelectedFile{}, err
	}
	return SelectedFile{Path: path, Name: filepath.Base(path), Size: info.Size()}, nil
}

func (s *AppService) SelectOutputDirectory() (string, error) {
	if s.app == nil {
		return "", errors.New("application is not ready")
	}
	return s.app.Dialog.OpenFile().
		SetTitle("Select output directory").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

func (s *AppService) ListScreens() []ScreenInfo {
	bounds := coreapp.DisplayBounds()
	screens := make([]ScreenInfo, 0, len(bounds))
	for i, rect := range bounds {
		screens = append(screens, ScreenInfo{
			Index:  i,
			Label:  fmt.Sprintf("Screen %d (%dx%d)", i, rect.Dx(), rect.Dy()),
			X:      rect.Min.X,
			Y:      rect.Min.Y,
			Width:  rect.Dx(),
			Height: rect.Dy(),
		})
	}
	return screens
}

func (s *AppService) ShowMainWindow() {
	showMainWindow(s.window)
}

func (s *AppService) HideMainWindow() {
	hideMainWindow(s.window)
}

func (s *AppService) Quit() {
	if s.app != nil {
		s.app.Quit()
	}
}

func (s *AppService) GetAutoStart() (bool, error) {
	if s.app == nil {
		return false, errors.New("application is not ready")
	}
	status, err := s.app.Autostart.Status()
	if err != nil {
		return false, err
	}
	return status.Enabled, nil
}

func (s *AppService) SetAutoStart(enabled bool) error {
	if s.app == nil {
		return errors.New("application is not ready")
	}
	if enabled {
		return s.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "autocimbar"})
	}
	return s.app.Autostart.Disable()
}

func showMainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Restore()
	window.Show()
	window.Focus()
}

func hideMainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Hide()
}

func ConfigureSystemTray(app *application.App, window *application.WebviewWindow, icon []byte) {
	tray := app.SystemTray.New()
	if len(icon) > 0 {
		tray.SetIcon(icon)
	}
	tray.SetTooltip("AutoCimBar")
	tray.AttachWindow(window)
	tray.OnClick(func() {
		if window != nil && window.IsVisible() {
			hideMainWindow(window)
			return
		}
		showMainWindow(window)
	})

	menu := app.NewMenu()
	menu.Add("显示").OnClick(func(ctx *application.Context) {
		showMainWindow(window)
	})
	menu.Add("隐藏").OnClick(func(ctx *application.Context) {
		hideMainWindow(window)
	})
	autoStartEnabled := false
	if status, err := app.Autostart.Status(); err == nil {
		autoStartEnabled = status.Enabled
	}
	menu.AddCheckbox("开机自启动", autoStartEnabled).OnClick(func(ctx *application.Context) {
		if ctx.IsChecked() {
			_ = app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "autocimbar"})
			return
		}
		_ = app.Autostart.Disable()
	})
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	tray.SetMenu(menu)
}

func (s *AppService) GetLogs(kind string, after uint64) (LogBatch, error) {
	if kind != "sender" && kind != "receiver" {
		return LogBatch{}, errors.New("invalid log kind")
	}
	return transferLogs.snapshot(kind, after), nil
}

func (s *AppService) OpenLogWindow(kind string) error {
	if kind != "sender" && kind != "receiver" {
		return errors.New("invalid log kind")
	}
	if s.app == nil {
		return errors.New("application is not ready")
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if s.logWindows == nil {
		s.logWindows = make(map[string]*application.WebviewWindow)
	}
	window := s.logWindows[kind]
	if window == nil {
		window = s.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: kind + "-logs", Title: "AutoCimBar — " + kind + " logs",
			Width: 1000, Height: 600, MinWidth: 480, MinHeight: 240, URL: "/?logs=" + kind,
		})
		window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			event.Cancel()
			window.Hide()
		})
		s.logWindows[kind] = window
	}
	showMainWindow(window)
	return nil
}
