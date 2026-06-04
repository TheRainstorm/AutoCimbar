package app

import (
	"errors"
	"image"
)

var ErrScreenCapture = errors.New("screen capture failed")

type capturedScreenFrame struct {
	Pix    []byte
	Img    *image.RGBA
	Width  int
	Height int
	Stride int
	BGRA   bool
}
