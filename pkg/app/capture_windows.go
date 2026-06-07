//go:build windows

package app

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"github.com/autocambar/autocambar/pkg/codec"
	dxshot "github.com/ghp3000/screenshot"
	"github.com/ghp3000/screenshot/d3d"
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
	dxgi         dxshot.ScreenShot
	display      image.Rectangle
	clipped      bool
	lastDXGI     []byte
}

func newScreenCapturer(rect image.Rectangle, backend string) (*screenCapturer, error) {
	width := rect.Dx()
	height := rect.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("capture rect must have positive size")
	}
	backend, err := normalizeCaptureBackend(backend)
	if err != nil {
		return nil, err
	}

	c := &screenCapturer{
		rect:   rect,
		width:  width,
		height: height,
	}
	if backend != CaptureBackendGDI {
		displayIndex, bounds, clipped, ok := displayForRect(rect)
		if !ok {
			if backend == CaptureBackendDXGI {
				return nil, fmt.Errorf("no display overlaps capture rect %v", rect)
			}
		} else {
			shot := dxshot.NewScreenShot(dxshot.ProviderDXGI)
			if err := shot.Init(displayIndex); err == nil {
				shot.DrawCursor(0)
				c.dxgi = shot
				c.display = bounds
				c.clipped = clipped
				c.img = image.NewRGBA(image.Rect(0, 0, width, height))
				return c, nil
			} else if backend == CaptureBackendDXGI {
				shot.Release()
				return nil, err
			}
			shot.Release()
			if backend == CaptureBackendDXGI {
				return nil, errors.New("DXGI capture init failed")
			}
		}
	}

	if err := c.initGDI(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *screenCapturer) initGDI() error {
	c.hwnd = win.GetDesktopWindow()
	c.img = image.NewRGBA(image.Rect(0, 0, c.width, c.height))
	c.hdc = win.GetDC(c.hwnd)
	if c.hdc == 0 {
		return errors.New("GetDC failed")
	}

	c.memoryDevice = win.CreateCompatibleDC(c.hdc)
	if c.memoryDevice == 0 {
		return errors.New("CreateCompatibleDC failed")
	}

	c.header.BiSize = uint32(unsafe.Sizeof(c.header))
	c.header.BiPlanes = 1
	c.header.BiBitCount = 32
	c.header.BiWidth = int32(c.width)
	c.header.BiHeight = int32(-c.height)
	c.header.BiCompression = win.BI_RGB

	c.bitmap = win.CreateDIBSection(c.hdc, &c.header, win.DIB_RGB_COLORS, &c.memptr, 0, 0)
	if c.bitmap == 0 {
		return errors.New("CreateDIBSection failed")
	}
	if c.memptr == nil {
		return errors.New("CreateDIBSection returned nil bits")
	}

	c.oldObject = win.SelectObject(c.memoryDevice, win.HGDIOBJ(c.bitmap))
	if c.oldObject == 0 {
		return errors.New("SelectObject failed")
	}

	return nil
}

func (c *screenCapturer) Capture() (*image.RGBA, error) {
	if c.dxgi != nil {
		frame, err := c.CaptureFrame(nil)
		if err != nil {
			return nil, err
		}
		copyBGRASliceToRGBA(c.img.Pix, frame.Pix, c.width, c.height, frame.Stride)
		return c.img, nil
	}
	if !win.BitBlt(c.memoryDevice, 0, 0, int32(c.width), int32(c.height), c.hdc, int32(c.rect.Min.X), int32(c.rect.Min.Y), win.SRCCOPY) {
		return nil, errors.New("BitBlt failed")
	}

	copyBGRAToRGBA(c.img.Pix, c.memptr, c.width, c.height)
	return c.img, nil
}

func (c *screenCapturer) DecodeInto(dec *codec.Decoder, dst []byte) ([]byte, error) {
	if c.dxgi != nil {
		frame, err := c.CaptureFrame(nil)
		if err != nil {
			return nil, err
		}
		return dec.DecodeBGRAInto(frame.Pix, frame.Width, frame.Height, frame.Stride, dst)
	}
	if !win.BitBlt(c.memoryDevice, 0, 0, int32(c.width), int32(c.height), c.hdc, int32(c.rect.Min.X), int32(c.rect.Min.Y), win.SRCCOPY) {
		return nil, fmt.Errorf("%w: BitBlt failed", ErrScreenCapture)
	}

	stride := c.width * 4
	pix := unsafe.Slice((*byte)(c.memptr), stride*c.height)
	return dec.DecodeBGRAInto(pix, c.width, c.height, stride, dst)
}

func (c *screenCapturer) CaptureFrame(dst []byte) (*capturedScreenFrame, error) {
	if c.dxgi != nil {
		return c.captureDXGIFrame(dst)
	}
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
	if c.dxgi != nil {
		c.dxgi.Release()
		c.dxgi = nil
	}
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

func (c *screenCapturer) Name() string {
	if c.dxgi != nil {
		if c.clipped {
			return "DXGI clipped"
		}
		return "DXGI"
	}
	return "GDI"
}

func (c *screenCapturer) captureDXGIFrame(dst []byte) (*capturedScreenFrame, error) {
	need := c.width * c.height * 4
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}

	img, err := c.dxgi.CaptureBGRA()
	if err != nil {
		if errors.Is(err, d3d.ErrNoImageYet) {
			if len(c.lastDXGI) == need {
				copy(dst, c.lastDXGI)
			} else {
				clear(dst)
			}
			return c.capturedBGRAFrame(dst), nil
		}
		return nil, fmt.Errorf("%w: DXGI capture failed: %v", ErrScreenCapture, err)
	}
	if err := c.copyDXGIRegion(dst, img); err != nil {
		return nil, err
	}
	if cap(c.lastDXGI) < need {
		c.lastDXGI = make([]byte, need)
	} else {
		c.lastDXGI = c.lastDXGI[:need]
	}
	copy(c.lastDXGI, dst)
	return c.capturedBGRAFrame(dst), nil
}

func (c *screenCapturer) capturedBGRAFrame(pix []byte) *capturedScreenFrame {
	return &capturedScreenFrame{
		Pix:    pix,
		Width:  c.width,
		Height: c.height,
		Stride: c.width * 4,
		BGRA:   true,
	}
}

func (c *screenCapturer) copyDXGIRegion(dst []byte, img *image.RGBA) error {
	overlap := c.rect.Intersect(c.display)
	if overlap.Empty() {
		return fmt.Errorf("%w: DXGI frame %dx%d does not overlap capture rect %v within display %v", ErrScreenCapture, img.Bounds().Dx(), img.Bounds().Dy(), c.rect, c.display)
	}
	if c.clipped {
		clear(dst)
	}
	srcX := overlap.Min.X - c.display.Min.X
	srcY := overlap.Min.Y - c.display.Min.Y
	dstX := overlap.Min.X - c.rect.Min.X
	dstY := overlap.Min.Y - c.rect.Min.Y
	displayW := c.display.Dx()
	displayH := c.display.Dy()
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()
	if displayW != imgW || displayH != imgH {
		return c.copyDXGIRotatedRegion(dst, img, overlap, srcX, srcY, dstX, dstY, displayW, displayH)
	}
	if srcX < 0 || srcY < 0 || srcX+overlap.Dx() > img.Bounds().Dx() || srcY+overlap.Dy() > img.Bounds().Dy() {
		return fmt.Errorf("%w: DXGI frame %dx%d does not contain overlap %v within display %v", ErrScreenCapture, img.Bounds().Dx(), img.Bounds().Dy(), overlap, c.display)
	}
	rowBytes := overlap.Dx() * 4
	dstStride := c.width * 4
	for y := 0; y < overlap.Dy(); y++ {
		srcStart := (srcY+y)*img.Stride + srcX*4
		dstStart := (dstY+y)*dstStride + dstX*4
		copy(dst[dstStart:dstStart+rowBytes], img.Pix[srcStart:srcStart+rowBytes])
	}
	return nil
}

func (c *screenCapturer) copyDXGIRotatedRegion(dst []byte, img *image.RGBA, overlap image.Rectangle, srcX int, srcY int, dstX int, dstY int, displayW int, displayH int) error {
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()
	if imgW != displayH || imgH != displayW {
		return fmt.Errorf("%w: DXGI frame %dx%d does not match rotated display %dx%d", ErrScreenCapture, imgW, imgH, displayW, displayH)
	}
	dstStride := c.width * 4
	for y := 0; y < overlap.Dy(); y++ {
		for x := 0; x < overlap.Dx(); x++ {
			// Desktop portrait coordinates map to the unrotated DXGI landscape surface.
			// This handles DXGI_MODE_ROTATION_ROTATE90, the common Windows portrait case.
			rotX := srcY + y
			rotY := displayW - 1 - (srcX + x)
			if rotX < 0 || rotY < 0 || rotX >= imgW || rotY >= imgH {
				return fmt.Errorf("%w: rotated DXGI source (%d,%d) outside frame %dx%d for overlap %v display %v", ErrScreenCapture, rotX, rotY, imgW, imgH, overlap, c.display)
			}
			srcStart := rotY*img.Stride + rotX*4
			dstStart := (dstY+y)*dstStride + (dstX+x)*4
			copy(dst[dstStart:dstStart+4], img.Pix[srcStart:srcStart+4])
		}
	}
	return nil
}

func displayForRect(rect image.Rectangle) (index int, bounds image.Rectangle, clipped bool, ok bool) {
	bestIndex := -1
	var bestBounds image.Rectangle
	bestArea := 0
	for i := 0; i < activeDisplayCount(); i++ {
		bounds := displayBounds(i)
		if rect.In(bounds) {
			return i, bounds, false, true
		}
		overlap := rect.Intersect(bounds)
		if overlap.Empty() {
			continue
		}
		area := overlap.Dx() * overlap.Dy()
		if area > bestArea {
			bestIndex = i
			bestBounds = bounds
			bestArea = area
		}
	}
	if bestIndex >= 0 {
		return bestIndex, bestBounds, true, true
	}
	return 0, image.Rectangle{}, false, false
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

func copyBGRASliceToRGBA(dst []byte, src []byte, width int, height int, stride int) {
	i := 0
	for y := 0; y < height; y++ {
		row := src[y*stride:]
		for x := 0; x < width; x++ {
			p := x * 4
			dst[i+0] = row[p+2]
			dst[i+1] = row[p+1]
			dst[i+2] = row[p+0]
			dst[i+3] = 255
			i += 4
		}
	}
}
