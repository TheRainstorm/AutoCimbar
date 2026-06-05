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
	q := flag.Int("Q", 120, "grid size in cells")
	rq := flag.Int("RQ", 0, "reference grid size for 8x8 tiles; when set, actual Q is scaled by 8/tile_width")
	b := flag.Int("B", 1, "screen scale factor")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	eccPercent := flag.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; encoder must use the same value")
	colorBits := flag.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; encoder must use the same value")
	shapeBits := flag.Int("shape-bits", 4, "shape bits per cell: 4 uses 16 symbols, 5 uses 32, 6 uses 64; encoder must use the same value")
	tile := flag.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT; encoder must use the same value")
	packetsPerFrame := flag.Int("packets", 1, "independent packets per screen frame; encoder must use the same value")
	screen := flag.Bool("screen", false, "capture frames from screen instead of reading PNG files")
	region := flag.String("R", "", "screen capture region SCREEN:X:Y, negative values anchor from right/bottom")
	fps := flag.Int("fps", 60, "screen capture rate")
	timeout := flag.Duration("timeout", 5*time.Minute, "screen decode timeout")
	listDisplays := flag.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing symbol PNG files named 00.png..; empty uses built-in 8x8/4bit symbols")
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
		spec, err := app.ParseTileSpec(*tile, *shapeBits)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid tile spec: %v\n", err)
			os.Exit(1)
		}
		gridSize, err := app.ResolveGridSize(*q, *rq, spec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid grid size: %v\n", err)
			os.Exit(1)
		}
		if err := app.DecodeScreenToFile(app.ScreenDecodeConfig{
			OutputPath:      *output,
			GridSize:        gridSize,
			Scale:           *b,
			SymbolDir:       *symbolDir,
			BlockSize:       *blockSize,
			ECCPercent:      *eccPercent,
			ColorBits:       *colorBits,
			ShapeBits:       *shapeBits,
			Tile:            *tile,
			PacketsPerFrame: *packetsPerFrame,
			Region:          *region,
			FPS:             *fps,
			Timeout:         *timeout,
			Progress:        os.Stderr,
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

	spec, err := app.ParseTileSpec(*tile, *shapeBits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid tile spec: %v\n", err)
		os.Exit(1)
	}
	gridSize, err := app.ResolveGridSize(*q, *rq, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid grid size: %v\n", err)
		os.Exit(1)
	}
	if err := app.DecodePNGFramesToFileWithSpec(*input, *output, gridSize, *b, *symbolDir, *blockSize, *eccPercent, *colorBits, spec); err != nil {
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
