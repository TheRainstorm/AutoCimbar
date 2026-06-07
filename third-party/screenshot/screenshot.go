// Package screenshot captures screen-shot image as image.RGBA.
// Mac, Windows, Linux, FreeBSD, OpenBSD, NetBSD, and Solaris are supported.
package screenshot

import (
	"image"
)

// CaptureDisplay captures whole region of displayIndex'th display.
func CaptureDisplay(displayIndex int) (*image.RGBA, error) {
	rect := GetDisplayBounds(displayIndex)
	return CaptureRect(rect)
}

// CaptureRect captures specified region of desktop.
func CaptureRect(rect image.Rectangle) (*image.RGBA, error) {
	return Capture(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy())
}

type CaptureProvider int32

const (
	ProviderAuto CaptureProvider = 0
	ProviderGDI  CaptureProvider = 1
	ProviderDXGI CaptureProvider = 2
)

type ScreenShot interface {
	// Init 初始化一个截屏实例，display是第几个屏幕，默认屏幕是0
	Init(display int) error
	// GetDisplayId 获取当前实例所截图的屏幕编号
	GetDisplayId() int
	// GetCaptureName 当前截图方法，GDI、DXGI
	GetCaptureName() string
	// Capture 截一张屏幕全图并用swizzle尝试使用simd方式转化为RGBA
	Capture() (*image.RGBA, error)
	// CaptureBGRA 截一张屏幕全图，不转换直接输出BGRA颜色格式
	CaptureBGRA() (*image.RGBA, error)
	// Release 释放资源，注意实例使用完毕一定要主动释放资源否则将内存溢出
	Release()
	// DrawCursor 是否绘制鼠标指针,0不绘制，1绘制，可动态设置
	DrawCursor(cursor int32)
}

func NewScreenShot(Provider CaptureProvider) ScreenShot {
	switch Provider {
	case ProviderAuto:
		shot := NewDXGIScreenshot()
		if shot == nil {
			return NewGDIScreenshot()
		}
		return shot
	case ProviderGDI:
		return NewGDIScreenshot()
	case ProviderDXGI:
		return NewDXGIScreenshot()
	}
	return NewGDIScreenshot()
}
