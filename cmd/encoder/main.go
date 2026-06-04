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
	redundancy := flag.Int("redundancy", 10, "extra fountain frames as a percentage")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	screen := flag.Bool("screen", false, "show frames in a borderless screen window instead of writing PNG files")
	region := flag.String("R", "0:0", "screen window region X:Y, negative values anchor from right/bottom")
	fps := flag.Int("fps", 30, "screen frame rate")
	addr := flag.String("addr", "127.0.0.1:8080", "screen encoder HTTP listen address")
	open := flag.Bool("open", true, "open the screen encoder page in the default browser")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing 16 libcimbar bitmap symbols; empty uses built-in symbols")
	flag.Parse()

	if *input == "" {
		fmt.Fprintln(os.Stderr, "missing -i input file")
		os.Exit(2)
	}

	if *screen {
		fmt.Printf("screen encoder serving http://%s/\n", *addr)
		result, err := app.EncodeFileToScreen(app.ScreenEncodeConfig{
			InputPath: *input,
			GridSize:  *q,
			Scale:     *b,
			SymbolDir: *symbolDir,
			BlockSize: *blockSize,
			Region:    *region,
			FPS:       *fps,
			Addr:      *addr,
			Open:      *open,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "screen encoder failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("screen encoded %d bytes, %d source block(s), block=%d bytes\n", result.FileSize, result.BlockCount, result.BlockSize)
		return
	}

	result, err := app.EncodeFileToPNGFrames(*input, *outputDir, *q, *b, *symbolDir, *redundancy, *blockSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("encoded %d bytes into %d frame(s), %d source block(s)\n", result.FileSize, len(result.FramePaths), result.BlockCount)
	fmt.Printf("Q=%d B=%d cell=%dpx image=%dx%dpx payload=%d bytes/frame block=%d bytes\n",
		result.GridSize, result.Scale, result.CellSize, result.ImageSize, result.ImageSize, result.PayloadCapacity, result.BlockSize)
	fmt.Printf("output: %s\n", *outputDir)
}
