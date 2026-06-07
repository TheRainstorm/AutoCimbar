package app

import (
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
)

var ErrScreenCapture = errors.New("screen capture failed")

const (
	CaptureBackendAuto = "auto"
	CaptureBackendDXGI = "dxgi"
	CaptureBackendGDI  = "gdi"
)

type capturedScreenFrame struct {
	Pix    []byte
	Img    *image.RGBA
	Width  int
	Height int
	Stride int
	BGRA   bool
}

func normalizeCaptureBackend(backend string) (string, error) {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		return CaptureBackendAuto, nil
	}
	switch backend {
	case CaptureBackendAuto, CaptureBackendDXGI, CaptureBackendGDI:
		return backend, nil
	default:
		return "", errors.New("capture backend must be auto, dxgi, or gdi")
	}
}

func saveCapturedFramePNG(path string, frame *capturedScreenFrame) error {
	if path == "" || frame == nil {
		return nil
	}
	img := frame.Img
	if img == nil {
		img = image.NewRGBA(image.Rect(0, 0, frame.Width, frame.Height))
		if frame.BGRA {
			copyBGRAFrameToRGBA(img.Pix, frame.Pix, frame.Width, frame.Height, frame.Stride)
		} else {
			copyRGBAFrameToRGBA(img.Pix, frame.Pix, frame.Width, frame.Height, frame.Stride)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func copyBGRAFrameToRGBA(dst []byte, src []byte, width int, height int, stride int) {
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

func copyRGBAFrameToRGBA(dst []byte, src []byte, width int, height int, stride int) {
	i := 0
	for y := 0; y < height; y++ {
		row := src[y*stride:]
		copy(dst[i:i+width*4], row[:width*4])
		i += width * 4
	}
}
