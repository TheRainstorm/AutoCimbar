package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/autocambar/autocambar/pkg/symbol"
)

func ParseTileSpec(tile string, shapeBits int) (symbol.Spec, error) {
	if tile == "" {
		tile = fmt.Sprintf("%dx%d", symbol.DefaultTileWidth, symbol.DefaultTileHeight)
	}
	parts := strings.Split(strings.ToLower(strings.TrimSpace(tile)), "x")
	if len(parts) != 2 {
		return symbol.Spec{}, fmt.Errorf("tile must be WIDTHxHEIGHT, got %q", tile)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return symbol.Spec{}, fmt.Errorf("invalid tile width %q: %w", parts[0], err)
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return symbol.Spec{}, fmt.Errorf("invalid tile height %q: %w", parts[1], err)
	}
	return symbol.NewSpec(width, height, shapeBits)
}
