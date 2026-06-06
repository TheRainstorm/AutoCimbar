//go:build windows

package app

import (
	"fmt"
	"image"
	"runtime"
	"sync"
	"time"
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
	procPostQuitMessage               = user32.NewProc("PostQuitMessage")
	procRegisterClassExW              = user32.NewProc("RegisterClassExW")
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
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
	back   []byte
	err    error
	mu     sync.Mutex
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
	if cfg.Stop != nil {
		go func() {
			<-cfg.Stop
			procDestroyWindow.Call(hwnd)
		}()
	}

	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	stopFrames := make(chan struct{})
	go win.runFrameLoop(cfg.FPS, stopFrames)
	defer close(stopFrames)

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
		if err := win.getErr(); err != nil {
			return err
		}
	}
	return win.getErr()
}

func nativeWndProc(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	win := activeNativeWindow
	switch msg {
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

func (w *nativeWindow) runFrameLoop(fps int, stop <-chan struct{}) {
	if fps <= 0 {
		fps = 30
	}
	interval := time.Second / time.Duration(fps)
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := w.updateFrame(); err != nil {
				w.setErr(err)
				procDestroyWindow.Call(uintptr(w.hwnd))
				return
			}
			procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
		}
	}
}

func (w *nativeWindow) updateFrame() error {
	pixels, err := w.source.NextBGRA(w.back)
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.back = w.pixels
	w.pixels = pixels
	w.mu.Unlock()

	return nil
}

func (w *nativeWindow) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	w.mu.Lock()
	defer w.mu.Unlock()
	if hdc != 0 {
		if len(w.pixels) == 0 {
			if len(w.back) != w.width*w.height*4 {
				w.back = make([]byte, w.width*w.height*4)
				for i := 3; i < len(w.back); i += 4 {
					w.back[i] = 0xff
				}
			}
			w.pixels = w.back
			w.back = nil
		}
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
		w.source.notePresented()
	}
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func (w *nativeWindow) setErr(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.err = err
}

func (w *nativeWindow) getErr() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
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
