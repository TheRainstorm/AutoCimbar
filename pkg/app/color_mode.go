package app

import (
	colorpkg "github.com/autocambar/autocambar/pkg/color"
)

func normalizeColorBits(colorBits int) int {
	return colorBits
}

func colorRecognizerForBits(colorBits int) (*colorpkg.Recognizer, error) {
	return colorpkg.NewRecognizerForBits(normalizeColorBits(colorBits))
}
