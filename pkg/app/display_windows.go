//go:build windows

package app

import (
	"image"

	dxshot "github.com/ghp3000/screenshot"
)

func activeDisplayCount() int {
	return dxshot.NumActiveDisplays()
}

func displayBounds(index int) image.Rectangle {
	return dxshot.GetDisplayBounds(index)
}
