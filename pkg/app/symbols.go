package app

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/autocambar/autocambar/pkg/symbol"
)

const (
	// DefaultSymbolDir is empty so release binaries use the built-in libcimbar
	// symbols and do not need third-party PNG files beside the executable.
	DefaultSymbolDir   = ""
	LibcimbarSymbolDir = "third-party/libcimbar/bitmap/4"
)

var symbolFileNames = [symbol.NumSymbols]string{
	"00.png", "01.png", "02.png", "03.png",
	"04.png", "05.png", "06.png", "07.png",
	"08.png", "09.png", "0a.png", "0b.png",
	"0c.png", "0d.png", "0e.png", "0f.png",
}

var builtinLibcimbarSymbolHashes = [symbol.NumSymbols]uint64{
	0x000103070f1f3f7f,
	0x7f3f1f0f07030100,
	0x0080c0e0f0f8fcfe,
	0xfefcf8f0e0c08000,
	0xe7e7e70000e7e7e7,
	0x991818ffff181899,
	0xc381183c3c1881c3,
	0xe7e7c3c381810000,
	0x3f0f030000030f3f,
	0x00030fffff0f0300,
	0x00c0f0fffff0c000,
	0x181818183c3c7e7e,
	0x7e7e3c3c18181818,
	0xffff3c1881c3e7ff,
	0xf3e3c78f8fc7e3f3,
	0xe1e1c7c7e3e38787,
}

func LoadLibcimbarSymbols(dir string) (*symbol.Recognizer, error) {
	return LoadSymbols(dir, symbol.DefaultSpec())
}

func LoadSymbols(dir string, spec symbol.Spec) (*symbol.Recognizer, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if dir == "" {
		if spec == symbol.DefaultSpec() {
			return LoadBuiltinLibcimbarSymbols()
		}
		return nil, fmt.Errorf("built-in symbols only support tile %dx%d shape-bits=%d; provide -symbols for tile %dx%d shape-bits=%d",
			symbol.DefaultTileWidth, symbol.DefaultTileHeight, symbol.DefaultShapeBits, spec.Width, spec.Height, spec.ShapeBits)
	}

	rec := symbol.NewRecognizerWithSpec(spec)

	for id := 0; id < spec.SymbolCount(); id++ {
		name := fmt.Sprintf("%02x.png", id)
		path := filepath.Join(dir, name)
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open symbol %s: %w", path, err)
		}

		img, err := png.Decode(f)
		closeErr := f.Close()
		if err != nil {
			return nil, fmt.Errorf("decode symbol %s: %w", path, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close symbol %s: %w", path, closeErr)
		}

		if err := rec.LoadSymbol(symbol.SymbolID(id), img); err != nil {
			return nil, fmt.Errorf("load symbol %s: %w", path, err)
		}
	}

	if !rec.IsLoaded() {
		return nil, fmt.Errorf("not all symbols were loaded from %s", dir)
	}

	return rec, nil
}

func LoadBuiltinLibcimbarSymbols() (*symbol.Recognizer, error) {
	rec := symbol.NewRecognizer()
	for id, hash := range builtinLibcimbarSymbolHashes {
		if err := rec.LoadSymbol(symbol.SymbolID(id), symbolHashImage(hash)); err != nil {
			return nil, fmt.Errorf("load built-in symbol %d: %w", id, err)
		}
	}
	return rec, nil
}

func symbolHashImage(hash uint64) image.Image {
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			shift := uint(63 - (y*8 + x))
			if (hash>>shift)&1 == 1 {
				img.SetGray(x, y, color.Gray{Y: 255})
			} else {
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return img
}
