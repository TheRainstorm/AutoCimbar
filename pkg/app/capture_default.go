//go:build !windows

package app

import (
	"fmt"
	"image"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/kbinani/screenshot"
)

type screenCapturer struct {
	rect image.Rectangle
}

func newScreenCapturer(rect image.Rectangle) (*screenCapturer, error) {
	return &screenCapturer{rect: rect}, nil
}

func (c *screenCapturer) Capture() (*image.RGBA, error) {
	return screenshot.CaptureRect(c.rect)
}

func (c *screenCapturer) DecodeInto(dec *codec.Decoder, dst []byte) ([]byte, error) {
	img, err := c.Capture()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrScreenCapture, err)
	}
	return dec.DecodeInto(img, dst)
}

func (c *screenCapturer) Close() error {
	return nil
}
