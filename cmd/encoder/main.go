package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	input := flag.String("i", "", "input file")
	outputDir := flag.String("o", "frames", "output PNG frame directory")
	q := flag.Int("Q", 50, "grid size in cells")
	b := flag.Int("B", 1, "screen scale factor")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "directory containing 16 libcimbar bitmap symbols")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "missing -i input file")
		os.Exit(2)
	}

	result, err := app.EncodeFileToPNGFrames(*input, *outputDir, *q, *b, *symbolDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("encoded %d bytes into %d frame(s)\n", result.FileSize, len(result.FramePaths))
	fmt.Printf("Q=%d B=%d cell=%dpx image=%dx%dpx payload=%d bytes/frame\n",
		result.GridSize, result.Scale, result.CellSize, result.ImageSize, result.ImageSize, result.PayloadCapacity)
	fmt.Printf("output: %s\n", *outputDir)
}
