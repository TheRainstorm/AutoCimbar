package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/autocambar/autocambar/pkg/symbol"
)

const DefaultSymbolDir = "third-party/libcimbar/bitmap/4"

var symbolFileNames = [symbol.NumSymbols]string{
	"00.png", "01.png", "02.png", "03.png",
	"04.png", "05.png", "06.png", "07.png",
	"08.png", "09.png", "0a.png", "0b.png",
	"0c.png", "0d.png", "0e.png", "0f.png",
}

func LoadLibcimbarSymbols(dir string) (*symbol.Recognizer, error) {
	rec := symbol.NewRecognizer()

	for id, name := range symbolFileNames {
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
