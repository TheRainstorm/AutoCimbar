package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/autocambar/autocambar/pkg/symbol"
)

func TestBuiltinLibcimbarSymbolsLoaded(t *testing.T) {
	rec, err := LoadBuiltinLibcimbarSymbols()
	if err != nil {
		t.Fatalf("LoadBuiltinLibcimbarSymbols failed: %v", err)
	}
	if !rec.IsLoaded() {
		t.Fatal("built-in recognizer is not fully loaded")
	}
}

func TestBuiltinLibcimbarSymbolsMatchPNGFiles(t *testing.T) {
	dir := filepath.Join("..", "..", LibcimbarSymbolDir)
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("libcimbar symbol dir not available: %v", err)
	}

	builtin, err := LoadBuiltinLibcimbarSymbols()
	if err != nil {
		t.Fatalf("LoadBuiltinLibcimbarSymbols failed: %v", err)
	}
	fromPNG, err := LoadLibcimbarSymbols(dir)
	if err != nil {
		t.Fatalf("LoadLibcimbarSymbols failed: %v", err)
	}

	for id := symbol.SymbolID(0); id < symbol.NumSymbols; id++ {
		builtinHash, err := builtin.GetHash(id)
		if err != nil {
			t.Fatalf("builtin hash %d: %v", id, err)
		}
		pngHash, err := fromPNG.GetHash(id)
		if err != nil {
			t.Fatalf("png hash %d: %v", id, err)
		}
		if builtinHash != pngHash {
			t.Fatalf("symbol %d hash mismatch: built-in %016x png %016x", id, builtinHash, pngHash)
		}
	}
}
