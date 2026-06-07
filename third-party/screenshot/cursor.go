package screenshot

import (
	"github.com/lxn/win"
	"syscall"
	"unsafe"
)

// http://msdn.microsoft.com/en-us/library/windows/desktop/dd162805.aspx
type POINT struct {
	X, Y int32
}

type (
	DWORD   uint32
	HCURSOR win.HANDLE
)

// https://msdn.microsoft.com/en-us/library/windows/desktop/ms648381(v=vs.85).aspx
type CURSORINFO struct {
	CbSize      DWORD
	Flags       DWORD
	HCursor     HCURSOR
	PtScreenPos POINT
}

var (
	moduser32          = syscall.NewLazyDLL("user32.dll")
	procGetCursorInfo  = moduser32.NewProc("GetCursorInfo")
	procGetCursorPos   = moduser32.NewProc("GetCursorPos")
	procDrawIcon       = moduser32.NewProc("DrawIcon")
	procScreenToClient = moduser32.NewProc("ScreenToClient")
)

func GetCursorInfo(pCursorInfo *CURSORINFO) bool {
	pCursorInfo.CbSize = DWORD(unsafe.Sizeof(*pCursorInfo))
	ret, _, _ := procGetCursorInfo.Call(uintptr(unsafe.Pointer(pCursorInfo)))
	return ret != 0
}

func ScreenToClient(hwnd win.HWND, x, y int) (X, Y int, ok bool) {
	pt := POINT{X: int32(x), Y: int32(y)}
	ret, _, _ := procScreenToClient.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&pt)))

	return int(pt.X), int(pt.Y), ret != 0
}
func DrawIcon(hDC win.HDC, x, y int, hIcon win.HANDLE) bool {
	ret, _, _ := procDrawIcon.Call(
		uintptr(unsafe.Pointer(hDC)),
		uintptr(x),
		uintptr(y),
		uintptr(unsafe.Pointer(hIcon)))

	return ret != 0
}
