package app

import (
	"fmt"

	"github.com/autocambar/autocambar/pkg/codec"
	colorpkg "github.com/autocambar/autocambar/pkg/color"
)

func normalizeColorBits(colorBits int) int {
	if colorBits == 0 {
		return codec.ColorBits
	}
	return colorBits
}

func colorRecognizerForBits(colorBits int) (*colorpkg.Recognizer, error) {
	switch normalizeColorBits(colorBits) {
	case 2:
		return colorpkg.NewRecognizer4Color(), nil
	case 4:
		return colorpkg.NewRecognizer16Color(), nil
	default:
		return nil, fmt.Errorf("color bits must be 2 or 4, got %d", colorBits)
	}
}
