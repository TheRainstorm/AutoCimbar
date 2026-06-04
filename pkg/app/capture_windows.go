//go:build windows

package app

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/lxn/win"
)

type screenCapturer struct {
	rect         image.Rectangle
	width        int
	height       int
	hwnd         win.HWND
	hdc          win.HDC
	memoryDevice win.HDC
	bitmap       win.HBITMAP
	oldObject    win.HGDIOBJ
	header       win.BITMAPINFOHEADER
	memptr       unsafe.Pointer
	img          *image.RGBA
}

func newScreenCapturer(rect image.Rectangle) (*screenCapturer, error) {
	width := rect.Dx()
	height := rect.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("capture rect must have positive size")
	}

	c := &screenCapturer{
		rect:   rect,
		width:  width,
		height: height,
		hwnd:   win.GetDesktopWindow(),
		img:    image.NewRGBA(image.Rect(0, 0, width, height)),
	}

	c.hdc = win.GetDC(c.hwnd)
	if c.hdc == 0 {
		c.Close()
		return nil, errors.New("GetDC failed")
	}

	c.memoryDevice = win.CreateCompatibleDC(c.hdc)
	if c.memoryDevice == 0 {
		c.Close()
		return nil, errors.New("CreateCompatibleDC failed")
	}

	c.header.BiSize = uint32(unsafe.Sizeof(c.header))
	c.header.BiPlanes = 1
	c.header.BiBitCount = 32
	c.header.BiWidth = int32(width)
	c.header.BiHeight = int32(-height)
	c.header.BiCompression = win.BI_RGB

	c.bitmap = win.CreateDIBSection(c.hdc, &c.header, win.DIB_RGB_COLORS, &c.memptr, 0, 0)
	if c.bitmap == 0 {
		c.Close()
		return nil, errors.New("CreateDIBSection failed")
	}
	if c.memptr == nil {
		c.Close()
		return nil, errors.New("CreateDIBSection returned nil bits")
	}

	c.oldObject = win.SelectObject(c.memoryDevice, win.HGDIOBJ(c.bitmap))
	if c.oldObject == 0 {
		c.Close()
		return nil, errors.New("SelectObject failed")
	}

	return c, nil
}

func (c *screenCapturer) Capture() (*image.RGBA, error) {
	if !win.BitBlt(c.memoryDevice, 0, 0, int32(c.width), int32(c.height), c.hdc, int32(c.rect.Min.X), int32(c.rect.Min.Y), win.SRCCOPY) {
		return nil, errors.New("BitBlt failed")
	}

	copyBGRAToRGBA(c.img.Pix, c.memptr, c.width, c.height)
	return c.img, nil
}

func (c *screenCapturer) DecodeInto(dec *codec.Decoder, dst []byte) ([]byte, error) {
	if !win.BitBlt(c.memoryDevice, 0, 0, int32(c.width), int32(c.height), c.hdc, int32(c.rect.Min.X), int32(c.rect.Min.Y), win.SRCCOPY) {
		return nil, fmt.Errorf("%w: BitBlt failed", ErrScreenCapture)
	}

	stride := c.width * 4
	pix := unsafe.Slice((*byte)(c.memptr), stride*c.height)
	return dec.DecodeBGRAInto(pix, c.width, c.height, stride, dst)
}

func (c *screenCapturer) CaptureFrame(dst []byte) (*capturedScreenFrame, error) {
	if !win.BitBlt(c.memoryDevice, 0, 0, int32(c.width), int32(c.height), c.hdc, int32(c.rect.Min.X), int32(c.rect.Min.Y), win.SRCCOPY) {
		return nil, fmt.Errorf("%w: BitBlt failed", ErrScreenCapture)
	}

	stride := c.width * 4
	need := stride * c.height
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	pix := unsafe.Slice((*byte)(c.memptr), need)
	copy(dst, pix)
	return &capturedScreenFrame{
		Pix:    dst,
		Width:  c.width,
		Height: c.height,
		Stride: stride,
		BGRA:   true,
	}, nil
}

func (c *screenCapturer) Close() error {
	if c.memoryDevice != 0 && c.oldObject != 0 {
		win.SelectObject(c.memoryDevice, c.oldObject)
		c.oldObject = 0
	}
	if c.bitmap != 0 {
		win.DeleteObject(win.HGDIOBJ(c.bitmap))
		c.bitmap = 0
	}
	if c.memoryDevice != 0 {
		win.DeleteDC(c.memoryDevice)
		c.memoryDevice = 0
	}
	if c.hdc != 0 {
		win.ReleaseDC(c.hwnd, c.hdc)
		c.hdc = 0
	}
	return nil
}

func copyBGRAToRGBA(dst []byte, src unsafe.Pointer, width int, height int) {
	offset := uintptr(src)
	i := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			b := *(*uint8)(unsafe.Pointer(offset))
			g := *(*uint8)(unsafe.Pointer(offset + 1))
			r := *(*uint8)(unsafe.Pointer(offset + 2))
			dst[i+0] = r
			dst[i+1] = g
			dst[i+2] = b
			dst[i+3] = 255
			i += 4
			offset += 4
		}
	}
}
