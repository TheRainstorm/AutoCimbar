package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	input := flag.String("i", "frames", "input PNG frame file or directory")
	output := flag.String("o", "decoded.out", "output file")
	q := flag.Int("Q", 50, "grid size in cells")
	b := flag.Int("B", 1, "screen scale factor")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	eccPercent := flag.Int("ecc", 0, "per-frame Reed-Solomon ECC percentage; encoder must use the same value")
	screen := flag.Bool("screen", false, "capture frames from screen instead of reading PNG files")
	region := flag.String("R", "", "screen capture region SCREEN:X:Y, negative values anchor from right/bottom")
	fps := flag.Int("fps", 30, "screen capture rate")
	timeout := flag.Duration("timeout", 5*time.Minute, "screen decode timeout")
	listDisplays := flag.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing 16 libcimbar bitmap symbols; empty uses built-in symbols")
	flag.Parse()

	if *listDisplays {
		for i, bounds := range app.DisplayBounds() {
			fmt.Printf("%d: %v width=%d height=%d\n", i, bounds, bounds.Dx(), bounds.Dy())
		}
		return
	}

	if *screen {
		if *region == "" {
			fmt.Fprintln(os.Stderr, "missing -R screen region")
			os.Exit(2)
		}
		if err := app.DecodeScreenToFile(app.ScreenDecodeConfig{
			OutputPath: *output,
			GridSize:   *q,
			Scale:      *b,
			SymbolDir:  *symbolDir,
			BlockSize:  *blockSize,
			ECCPercent: *eccPercent,
			Region:     *region,
			FPS:        *fps,
			Timeout:    *timeout,
			Progress:   os.Stderr,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "screen decoder failed: %v\n", err)
			os.Exit(1)
		}
		md5, err := app.FileMD5Hex(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "md5 failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("decoded screen -> %s, md5=%s\n", *output, md5)
		return
	}

	if err := app.DecodePNGFramesToFile(*input, *output, *q, *b, *symbolDir, *blockSize, *eccPercent); err != nil {
		fmt.Fprintf(os.Stderr, "decoder failed: %v\n", err)
		os.Exit(1)
	}

	md5, err := app.FileMD5Hex(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "md5 failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("decoded %s -> %s, md5=%s\n", *input, *output, md5)
}
