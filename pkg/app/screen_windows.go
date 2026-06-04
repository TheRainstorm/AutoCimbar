//go:build windows

package app

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	csHRedraw = 0x0002
	csVRedraw = 0x0001

	wsPopup   = 0x80000000
	wsVisible = 0x10000000

	wsExTopMost    = 0x00000008
	wsExToolWindow = 0x00000080

	swShow = 5

	wmDestroy = 0x0002
	wmPaint   = 0x000f
	wmTimer   = 0x0113
	wmKeyDown = 0x0100

	vkEscape = 0x1b

	biRGB        = 0
	dibRGBColors = 0
	srccopy      = 0x00cc0020
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	winmm  = windows.NewLazySystemDLL("winmm.dll")

	procBeginPaint                    = user32.NewProc("BeginPaint")
	procCreateWindowExW               = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                = user32.NewProc("DefWindowProcW")
	procDestroyWindow                 = user32.NewProc("DestroyWindow")
	procDispatchMessageW              = user32.NewProc("DispatchMessageW")
	procEndPaint                      = user32.NewProc("EndPaint")
	procGetMessageW                   = user32.NewProc("GetMessageW")
	procInvalidateRect                = user32.NewProc("InvalidateRect")
	procKillTimer                     = user32.NewProc("KillTimer")
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	procSetTimer                      = user32.NewProc("SetTimer")
	procShowWindow                    = user32.NewProc("ShowWindow")
	procTranslateMessage              = user32.NewProc("TranslateMessage")
	procUpdateWindow                  = user32.NewProc("UpdateWindow")

	procStretchDIBits = gdi32.NewProc("StretchDIBits")

	procTimeBeginPeriod = winmm.NewProc("timeBeginPeriod")
	procTimeEndPeriod   = winmm.NewProc("timeEndPeriod")
)

func init() {
	// Set DPI awareness before any monitor bounds are queried. Otherwise Windows
	// can virtualize coordinates and native window placement lands on the wrong
	// physical pixels on mixed/HiDPI multi-monitor layouts.
	const dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)
	if ret, _, _ := procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2); ret == 0 {
		_, _, _ = procSetProcessDPIAware.Call()
	}
}

type nativeWindow struct {
	source *screenFrameSource
	hwnd   windows.Handle
	width  int
	height int
	pixels []byte
	err    error
}

var activeNativeWindow *nativeWindow

func runScreenEncoderBackend(cfg ScreenEncodeConfig, source *screenFrameSource, rect image.Rectangle) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	endTimerResolution := beginTimerResolution()
	defer endTimerResolution()

	win := &nativeWindow{
		source: source,
		width:  rect.Dx(),
		height: rect.Dy(),
	}
	if err := win.updateFrame(); err != nil {
		return err
	}
	activeNativeWindow = win
	defer func() {
		activeNativeWindow = nil
	}()

	var instance windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
		return err
	}

	className, _ := windows.UTF16PtrFromString("AutoCamBarEncoderWindow")
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		Style:     csHRedraw | csVRedraw,
		WndProc:   windows.NewCallback(nativeWndProc),
		Instance:  instance,
		ClassName: className,
	}
	atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("RegisterClassExW failed: %w", callErr)
	}

	title, _ := windows.UTF16PtrFromString("AutoCamBar Encoder")
	hwnd, _, callErr := procCreateWindowExW.Call(
		wsExTopMost|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup|wsVisible,
		uintptr(int32(rect.Min.X)),
		uintptr(int32(rect.Min.Y)),
		uintptr(int32(rect.Dx())),
		uintptr(int32(rect.Dy())),
		0,
		0,
		uintptr(instance),
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed: %w", callErr)
	}
	win.hwnd = windows.Handle(hwnd)

	interval := 1000 / cfg.FPS
	if interval <= 0 {
		interval = 1
	}
	timer, _, callErr := procSetTimer.Call(hwnd, 1, uintptr(uint32(interval)), 0)
	if timer == 0 {
		return fmt.Errorf("SetTimer failed: %w", callErr)
	}
	defer procKillTimer.Call(hwnd, 1)

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	var msg msg
	for {
		ret, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) == -1 {
			return fmt.Errorf("GetMessageW failed: %w", callErr)
		}
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		if win.err != nil {
			return win.err
		}
	}
	return win.err
}

func nativeWndProc(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	win := activeNativeWindow
	switch msg {
	case wmTimer:
		if win != nil {
			if err := win.updateFrame(); err != nil {
				win.err = err
				procDestroyWindow.Call(hwnd)
				return 0
			}
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmPaint:
		if win != nil {
			win.paint(hwnd)
		}
		return 0
	case wmKeyDown:
		if wparam == vkEscape {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

func (w *nativeWindow) updateFrame() error {
	pixels, err := w.source.NextBGRA(w.pixels)
	if err != nil {
		return err
	}
	w.pixels = pixels
	w.source.notePresented()
	return nil
}

func (w *nativeWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc != 0 && len(w.pixels) > 0 {
		bmi := bitmapInfo{
			Header: bitmapInfoHeader{
				Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
				Width:       int32(w.width),
				Height:      -int32(w.height),
				Planes:      1,
				BitCount:    32,
				Compression: biRGB,
			},
		}
		procStretchDIBits.Call(
			hdc,
			0, 0, uintptr(int32(w.width)), uintptr(int32(w.height)),
			0, 0, uintptr(int32(w.width)), uintptr(int32(w.height)),
			uintptr(unsafe.Pointer(&w.pixels[0])),
			uintptr(unsafe.Pointer(&bmi)),
			dibRGBColors,
			srccopy,
		)
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func beginTimerResolution() func() {
	ret, _, _ := procTimeBeginPeriod.Call(1)
	if ret != 0 {
		return func() {}
	}
	return func() {
		procTimeEndPeriod.Call(1)
	}
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type paintStruct struct {
	Hdc         windows.Handle
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}
