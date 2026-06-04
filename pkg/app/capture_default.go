//go:build !windows

package app

import (
	"image"

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

func (c *screenCapturer) Close() error {
	return nil
}
