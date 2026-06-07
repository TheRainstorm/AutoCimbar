//go:build !windows

package app

import (
	"image"

	"github.com/kbinani/screenshot"
)

func activeDisplayCount() int {
	return screenshot.NumActiveDisplays()
}

func displayBounds(index int) image.Rectangle {
	return screenshot.GetDisplayBounds(index)
}
