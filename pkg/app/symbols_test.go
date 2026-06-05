package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/autocambar/autocambar/pkg/symbol"
	"github.com/autocambar/autocambar/pkg/tilegen"
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

func TestLoadGeneratedSymbolsWithSpec(t *testing.T) {
	spec, err := symbol.NewSpec(8, 8, 5)
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	result, err := tilegen.Generate(tilegen.Options{Spec: spec, Seed: 42, Attempts: 2000})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "symbols")
	if err := tilegen.Save(result, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rec, err := LoadSymbols(dir, spec)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	if !rec.IsLoaded() {
		t.Fatal("generated recognizer is not fully loaded")
	}
	if rec.SymbolCount() != 32 {
		t.Fatalf("SymbolCount = %d, want 32", rec.SymbolCount())
	}
	minDist, _ := rec.VerifyHammingDistances()
	if minDist <= 0 {
		t.Fatalf("min hamming distance = %d", minDist)
	}
	if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%02x.png", rec.SymbolCount()-1))); err != nil {
		t.Fatalf("last symbol missing: %v", err)
	}
}
