package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autocambar/autocambar/pkg/app"
	"github.com/autocambar/autocambar/pkg/tilegen"
)

func main() {
	tile := flag.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT")
	shapeBits := flag.Int("shape-bits", 4, "shape bits; output symbol count is 1<<shape-bits")
	output := flag.String("o", "", "output directory; default generated-tiles/TILE_SHAPEBITSbit")
	seed := flag.Uint64("seed", 1, "deterministic random seed")
	attempts := flag.Int("attempts", 0, "candidate attempts per symbol, 0 uses default")
	targetDistance := flag.Int("target-distance", 0, "stop early once a candidate reaches this min hamming distance, 0 uses default")
	flag.Parse()

	spec, err := app.ParseTileSpec(*tile, *shapeBits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid tile spec: %v\n", err)
		os.Exit(2)
	}
	out := *output
	if out == "" {
		out = filepath.Join("generated-tiles", fmt.Sprintf("%dx%d_%dbit", spec.Width, spec.Height, spec.ShapeBits))
	}

	result, err := tilegen.Generate(tilegen.Options{
		Spec:           spec,
		Seed:           *seed,
		Attempts:       *attempts,
		TargetDistance: *targetDistance,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "tilegen failed: %v\n", err)
		os.Exit(1)
	}
	if err := tilegen.Save(result, out); err != nil {
		fmt.Fprintf(os.Stderr, "save failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("generated %d symbols into %s\n", spec.SymbolCount(), out)
	fmt.Printf("tile=%dx%d shape_bits=%d min_distance=%d avg_distance=%.2f seed=%d attempts=%d\n",
		spec.Width, spec.Height, spec.ShapeBits, result.MinDistance, result.AvgDistance, result.Seed, result.Attempts)
}
