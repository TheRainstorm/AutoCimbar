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
	fileSize := flag.Int("size", -1, "original file size in bytes")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "directory containing 16 libcimbar bitmap symbols")
	flag.Parse()

	if *fileSize < 0 {
		fmt.Fprintln(os.Stderr, "missing -size original file size")
		os.Exit(2)
	}

	if err := app.DecodePNGFramesToFile(*input, *output, *q, *b, *symbolDir, *fileSize, *blockSize); err != nil {
		fmt.Fprintf(os.Stderr, "decoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("decoded %s -> %s\n", *input, *output)
}
