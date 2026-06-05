package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/autocambar/autocambar/pkg/symbol"
)

type CellSpec struct {
	Tile      string
	ShapeBits int
	ColorBits int
}

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

func ParseCellSpec(text string, defaults CellSpec) (CellSpec, error) {
	if strings.TrimSpace(text) == "" {
		return defaults, nil
	}
	out := defaults
	re := regexp.MustCompile(`(\d+)([tcs])`)
	matches := re.FindAllStringSubmatch(strings.ToLower(strings.TrimSpace(text)), -1)
	if len(matches) == 0 {
		return CellSpec{}, fmt.Errorf("cell spec must contain parts like 4t4c4s, got %q", text)
	}
	seen := make(map[byte]bool, len(matches))
	consumed := ""
	for _, match := range matches {
		consumed += match[0]
		value, err := strconv.Atoi(match[1])
		if err != nil {
			return CellSpec{}, err
		}
		key := match[2][0]
		if seen[key] {
			return CellSpec{}, fmt.Errorf("duplicate cell spec part %q", match[2])
		}
		seen[key] = true
		switch key {
		case 't':
			out.Tile = fmt.Sprintf("%dx%d", value, value)
		case 'c':
			out.ColorBits = value
		case 's':
			out.ShapeBits = value
		}
	}
	if consumed != strings.ToLower(strings.TrimSpace(text)) {
		return CellSpec{}, fmt.Errorf("invalid cell spec %q", text)
	}
	return out, nil
}

func ResolveGridSize(gridSize int, referenceGridSize int, spec symbol.Spec) (int, error) {
	if referenceGridSize > 0 {
		if err := spec.Validate(); err != nil {
			return 0, err
		}
		gridSize = (referenceGridSize*symbol.DefaultTileWidth + spec.Width - 1) / spec.Width
	}
	if gridSize <= 0 {
		return 0, fmt.Errorf("Q must be > 0")
	}
	return gridSize, nil
}
