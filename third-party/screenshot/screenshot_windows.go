package screenshot

import (
	"errors"
	"fmt"
	"github.com/ghp3000/screenshot/internal/util"
	"github.com/ghp3000/screenshot/swizzle"
	"github.com/lxn/win"
	"image"
	"reflect"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	libUser32, _               = syscall.LoadLibrary("user32.dll")
	funcGetDesktopWindow, _    = syscall.GetProcAddress(syscall.Handle(libUser32), "GetDesktopWindow")
	funcEnumDisplayMonitors, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplayMonitors")
	funcGetMonitorInfo, _      = syscall.GetProcAddress(syscall.Handle(libUser32), "GetMonitorInfoW")
	funcEnumDisplaySettings, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplaySettingsW")
)

func Capture(x, y, width, height int) (*image.RGBA, error) {
	rect := image.Rect(0, 0, width, height)
	img, err := util.CreateImage(rect)
	if err != nil {
		return nil, err
	}
	hwnd := getDesktopWindow()
	hdc := win.GetDC(hwnd)
	if hdc == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer win.ReleaseDC(hwnd, hdc)
	memory_device := win.CreateCompatibleDC(hdc)
	if memory_device == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer win.DeleteDC(memory_device)
	bitmap := win.CreateCompatibleBitmap(hdc, int32(width), int32(height))
	if bitmap == 0 {
		return nil, errors.New("CreateCompatibleBitmap failed")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))

	var header win.BITMAPINFOHEADER
	header.BiSize = uint32(unsafe.Sizeof(header))
	header.BiPlanes = 1
	header.BiBitCount = 32
	header.BiWidth = int32(width)
	header.BiHeight = int32(-height)
	header.BiCompression = win.BI_RGB
	header.BiSizeImage = 0

	// GetDIBits balks at using Go memory on some systems. The MSDN example uses
	// GlobalAlloc, so we'll do that too. See:
	// https://docs.microsoft.com/en-gb/windows/desktop/gdi/capturing-an-image
	bitmapDataSize := uintptr(((int64(width)*int64(header.BiBitCount) + 31) / 32) * 4 * int64(height))
	hmem := win.GlobalAlloc(win.GMEM_MOVEABLE, bitmapDataSize)
	defer win.GlobalFree(hmem)
	memptr := win.GlobalLock(hmem)
	defer win.GlobalUnlock(hmem)

	old := win.SelectObject(memory_device, win.HGDIOBJ(bitmap))
	if old == 0 {
		return nil, errors.New("SelectObject failed")
	}
	defer win.SelectObject(memory_device, old)

	if !win.BitBlt(memory_device, 0, 0, int32(width), int32(height), hdc, int32(x), int32(y), win.SRCCOPY) {
		return nil, errors.New("BitBlt failed")
	}

	if win.GetDIBits(hdc, bitmap, 0, uint32(height), (*uint8)(memptr), (*win.BITMAPINFO)(unsafe.Pointer(&header)), win.DIB_RGB_COLORS) == 0 {
		return nil, errors.New("GetDIBits failed")
	}

	/*	i := 0
		src := uintptr(memptr)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				v0 := *(*uint8)(unsafe.Pointer(src))
				v1 := *(*uint8)(unsafe.Pointer(src + 1))
				v2 := *(*uint8)(unsafe.Pointer(src + 2))

				// BGRA => RGBA, and set A to 255
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = v2, v1, v0, 255

				i += 4
				src += 4
			}
		}*/

	var slice []byte
	hdrp := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	hdrp.Data = uintptr(memptr)
	hdrp.Len = width * height * 4
	hdrp.Cap = width * height * 4
	swizzle.BGRA(slice)
	copy(img.Pix, slice)
	return img, nil
}

func NumActiveDisplays() int {
	var count int = 0
	enumDisplayMonitors(win.HDC(0), nil, syscall.NewCallback(countupMonitorCallback), uintptr(unsafe.Pointer(&count)))
	return count
}

func GetDisplayBounds(displayIndex int) image.Rectangle {
	var ctx getMonitorBoundsContext
	ctx.Index = displayIndex
	ctx.Count = 0
	enumDisplayMonitors(win.HDC(0), nil, syscall.NewCallback(getMonitorBoundsCallback), uintptr(unsafe.Pointer(&ctx)))
	return image.Rect(
		int(ctx.Rect.Left), int(ctx.Rect.Top),
		int(ctx.Rect.Right), int(ctx.Rect.Bottom))
}

func getDesktopWindow() win.HWND {
	ret, _, _ := syscall.Syscall(funcGetDesktopWindow, 0, 0, 0, 0)
	return win.HWND(ret)
}

func enumDisplayMonitors(hdc win.HDC, lprcClip *win.RECT, lpfnEnum uintptr, dwData uintptr) bool {
	ret, _, _ := syscall.Syscall6(funcEnumDisplayMonitors, 4,
		uintptr(hdc),
		uintptr(unsafe.Pointer(lprcClip)),
		lpfnEnum,
		dwData,
		0,
		0)
	return int(ret) != 0
}

func countupMonitorCallback(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
	var count *int
	count = (*int)(unsafe.Pointer(dwData))
	*count = *count + 1
	return uintptr(1)
}

type getMonitorBoundsContext struct {
	Index int
	Rect  win.RECT
	Count int
}

func getMonitorBoundsCallback(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
	var ctx *getMonitorBoundsContext
	ctx = (*getMonitorBoundsContext)(unsafe.Pointer(dwData))
	if ctx.Count != ctx.Index {
		ctx.Count = ctx.Count + 1
		return uintptr(1)
	}

	if realSize := getMonitorRealSize(hMonitor); realSize != nil {
		ctx.Rect = *realSize
	} else {
		ctx.Rect = *lprcMonitor
	}

	return uintptr(0)
}

type _MONITORINFOEX struct {
	win.MONITORINFO
	DeviceName [win.CCHDEVICENAME]uint16
}

const _ENUM_CURRENT_SETTINGS = 0xFFFFFFFF

type _DEVMODE struct {
	_            [68]byte
	DmSize       uint16
	_            [6]byte
	DmPosition   win.POINT
	_            [86]byte
	DmPelsWidth  uint32
	DmPelsHeight uint32
	_            [40]byte
}

// getMonitorRealSize makes a call to GetMonitorInfo
// to obtain the device name for the monitor handle
// provided to the method.
//
// With the device name, EnumDisplaySettings is called to
// obtain the current configuration for the monitor, this
// information includes the real resolution of the monitor
// rather than the scaled version based on DPI.
//
// If either handle calls fail, it will return a nil
// allowing the calling method to use the bounds information
// returned by EnumDisplayMonitors which may be affected
// by DPI.
func getMonitorRealSize(hMonitor win.HMONITOR) *win.RECT {
	info := _MONITORINFOEX{}
	info.CbSize = uint32(unsafe.Sizeof(info))

	ret, _, _ := syscall.Syscall(funcGetMonitorInfo, 2, uintptr(hMonitor), uintptr(unsafe.Pointer(&info)), 0)
	if ret == 0 {
		return nil
	}

	devMode := _DEVMODE{}
	devMode.DmSize = uint16(unsafe.Sizeof(devMode))

	if ret, _, _ := syscall.Syscall(funcEnumDisplaySettings, 3, uintptr(unsafe.Pointer(&info.DeviceName[0])), _ENUM_CURRENT_SETTINGS, uintptr(unsafe.Pointer(&devMode))); ret == 0 {
		return nil
	}

	return &win.RECT{
		Left:   devMode.DmPosition.X,
		Right:  devMode.DmPosition.X + int32(devMode.DmPelsWidth),
		Top:    devMode.DmPosition.Y,
		Bottom: devMode.DmPosition.Y + int32(devMode.DmPelsHeight),
	}
}

type GDIScreenshot struct {
	rect          image.Rectangle
	hwnd          win.HWND
	hdc           win.HDC
	memory_device win.HDC
	bitmap        win.HBITMAP
	header        win.BITMAPINFOHEADER
	hmem          win.HGLOBAL
	ptr           unsafe.Pointer
	display       int //截取哪个屏幕
	cursor        int32
}

func NewGDIScreenshot() *GDIScreenshot {
	return &GDIScreenshot{}
}

func (s *GDIScreenshot) Init(display int) (err error) {
	if display < 0 {
		return fmt.Errorf("display %d is invalid", display)
	}
	s.display = display
	s.rect = GetDisplayBounds(display)
	width := s.rect.Dx()
	height := s.rect.Dy()
	if width < 1 || height < 1 {
		return fmt.Errorf("width=%d,height=%d is invalid", width, height)
	}

	s.hwnd = getDesktopWindow()
	s.hdc = win.GetDC(s.hwnd)
	if s.hdc == 0 {
		return errors.New("GetDC failed")
	}
	s.memory_device = win.CreateCompatibleDC(s.hdc)
	if s.memory_device == 0 {
		return errors.New("CreateCompatibleDC failed")
	}

	s.header.BiSize = uint32(unsafe.Sizeof(s.header))
	s.header.BiPlanes = 1
	s.header.BiBitCount = 32
	s.header.BiWidth = int32(width)
	s.header.BiHeight = int32(-height)
	s.header.BiCompression = win.BI_RGB
	s.header.BiSizeImage = 0
	bitmapDataSize := uintptr(((int64(width)*int64(s.header.BiBitCount) + 31) / 32) * 4 * int64(height))
	s.hmem = win.GlobalAlloc(win.GMEM_MOVEABLE, bitmapDataSize)
	s.ptr = unsafe.Pointer(uintptr(0))
	s.bitmap = win.CreateDIBSection(s.memory_device, &s.header, win.DIB_RGB_COLORS, &s.ptr, 0, 0)
	if s.bitmap == 0 {
		return errors.New("CreateCompatibleBitmap failed")
	} else if s.bitmap == 2 {
		return errors.New("One or more of the input parameters is invalid while calling CreateDIBSection.\n")
	}
	return nil
}

// Capture x,y 起始的点坐标，width宽度，height高度
func (s *GDIScreenshot) Capture() (*image.RGBA, error) {
	rect := GetDisplayBounds(s.display)
	// 如果发现屏幕范围发生了变化就重新初始化
	if rect != s.rect {
		s.Release()
		if err := s.Init(s.display); err != nil {
			return nil, err
		}
	}

	x, y, width, height := s.rect.Min.X, s.rect.Min.Y, s.rect.Dx(), s.rect.Dy()

	old := win.SelectObject(s.memory_device, win.HGDIOBJ(s.bitmap))
	if old == 0 {
		return nil, errors.New("SelectObject failed")
	}
	defer win.SelectObject(s.memory_device, old)

	if !win.BitBlt(s.memory_device, 0, 0, int32(width), int32(height), s.hdc, int32(x), int32(y), win.SRCCOPY|win.CAPTUREBLT) {
		return nil, errors.New("BitBlt failed")
	}
	if atomic.LoadInt32(&s.cursor) == 1 {
		CursorInfo := new(CURSORINFO)
		_ = GetCursorInfo(CursorInfo)
		cx, cy, ok := ScreenToClient(s.hwnd, int(CursorInfo.PtScreenPos.X), int(CursorInfo.PtScreenPos.Y))
		if ok {
			if int(CursorInfo.HCursor) == 65541 {
				cx -= 8
				cy -= 9
			}
			DrawIcon(s.memory_device, cx, cy, win.HANDLE(CursorInfo.HCursor))
		}
	}
	var slice []byte
	hdrp := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	hdrp.Data = uintptr(s.ptr)
	hdrp.Len = width * height * 4
	hdrp.Cap = width * height * 4

	/*	imageBytes := make([]byte, len(slice))
		start := time.Now()
		for i := 0; i < len(imageBytes); i += 4 {
			imageBytes[i], imageBytes[i+2], imageBytes[i+1], imageBytes[i+3] = slice[i+2], slice[i], slice[i+1], slice[i+3]
		}
		fmt.Println("BGRA to RGBA", time.Since(start))
		img := &image.RGBA{Pix: imageBytes, Stride: 4 * x, Rect: image.Rect(0, 0, x, y)}
		//swizzle*/
	swizzle.BGRA(slice)
	img := image.NewRGBA(image.Rect(x, y, x+width, y+height))
	copy(img.Pix, slice)
	return img, nil
}

func (s *GDIScreenshot) CaptureBGRA() (*image.RGBA, error) {
	x, y := s.rect.Dx(), s.rect.Dy()
	old := win.SelectObject(s.memory_device, win.HGDIOBJ(s.bitmap))
	if old == 0 {
		return nil, errors.New("SelectObject failed")
	}
	defer win.SelectObject(s.memory_device, old)
	if !win.BitBlt(s.memory_device, 0, 0, int32(x), int32(y), s.hdc, int32(s.rect.Min.X), int32(s.rect.Min.Y), win.SRCCOPY|win.CAPTUREBLT) {
		return nil, errors.New("BitBlt failed")
	}
	var slice []byte
	hdrp := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	hdrp.Data = uintptr(s.ptr)
	hdrp.Len = x * y * 4
	hdrp.Cap = x * y * 4
	//swizzle.BGRA(slice)
	img := image.NewRGBA(image.Rect(0, 0, x, y))
	copy(img.Pix, slice)
	return img, nil
}

func (s *GDIScreenshot) Release() {
	win.ReleaseDC(s.hwnd, s.hdc)
	win.DeleteDC(s.memory_device)
	win.DeleteObject(win.HGDIOBJ(s.bitmap))
	win.GlobalFree(s.hmem)
}
func (s *GDIScreenshot) GetDisplayId() int {
	return s.display
}
func (s *GDIScreenshot) GetCaptureName() string {
	return "GDI"
}

func (s *GDIScreenshot) DrawCursor(cursor int32) {
	atomic.StoreInt32(&s.cursor, cursor)
}
