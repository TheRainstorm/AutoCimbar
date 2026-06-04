package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	input := flag.String("i", "frames", "input PNG frame file or directory")
	output := flag.String("o", "decoded.out", "output file")
	q := flag.Int("Q", 50, "grid size in cells")
	b := flag.Int("B", 1, "screen scale factor")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "directory containing 16 libcimbar bitmap symbols")
	flag.Parse()

	if err := app.DecodePNGFramesToFile(*input, *output, *q, *b, *symbolDir); err != nil {
		fmt.Fprintf(os.Stderr, "decoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("decoded %s -> %s\n", *input, *output)
}
