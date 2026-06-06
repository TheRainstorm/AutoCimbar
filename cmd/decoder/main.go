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
	output := flag.String("o", ".", "output file or directory; when omitted or a directory, uses the sender file name")
	q := flag.Int("Q", 120, "grid size in cells")
	rq := flag.Int("RQ", 0, "reference grid size for 8x8 tiles; when set, actual Q is scaled by 8/tile_width")
	b := flag.Int("B", 1, "screen scale factor")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	eccPercent := flag.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; encoder must use the same value")
	backend := flag.String("backend", app.BackendSymbols, "frame backend: symbols or qr")
	colorBits := flag.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; encoder must use the same value")
	shapeBits := flag.Int("shape-bits", 4, "shape bits per cell: 4 uses 16 symbols, 5 uses 32, 6 uses 64; encoder must use the same value")
	tile := flag.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT; encoder must use the same value")
	cell := flag.String("cell", "", "compact cell spec like 8t4s2c: tile width, color bits, shape bits")
	cellShort := flag.String("c", "", "short alias for -cell")
	packetsPerFrame := flag.Int("packets", 1, "independent packets per screen frame; encoder must use the same value")
	packetsShort := flag.Int("p", 0, "short alias for -packets")
	screen := flag.Bool("screen", true, "capture frames from screen instead of reading PNG files")
	pngMode := flag.Bool("png", false, "read PNG frames instead of screen mode")
	region := flag.String("R", "0", "screen capture region SCREEN, X:Y or SCREEN:X:Y; c centers an axis")
	regionShort := flag.String("r", "", "short alias for -R")
	fps := flag.Int("fps", 120, "screen capture rate")
	fpsShort := flag.Int("f", 0, "short alias for -fps")
	decodeWorkers := flag.Int("decode-workers", 0, "parallel screen decode workers, 0 chooses automatically")
	timeout := flag.Duration("timeout", 5*time.Minute, "screen decode timeout")
	listDisplays := flag.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing symbol PNG files named 00.png..; empty uses built-in symbols")
	symbolDirShort := flag.String("s", "", "short alias for -symbols")
	flag.Parse()

	if err := app.ApplyINIConfig(flag.CommandLine, "decoder", shortAliases()); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	if *cellShort != "" {
		*cell = *cellShort
	}
	cellSpec, err := app.ParseCellSpec(*cell, app.CellSpec{Tile: *tile, ShapeBits: *shapeBits, ColorBits: *colorBits})
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid cell spec: %v\n", err)
		os.Exit(1)
	}
	*tile = cellSpec.Tile
	*shapeBits = cellSpec.ShapeBits
	*colorBits = cellSpec.ColorBits
	if *packetsShort > 0 {
		*packetsPerFrame = *packetsShort
	}
	if *regionShort != "" {
		*region = *regionShort
	}
	if *fpsShort > 0 {
		*fps = *fpsShort
	}
	if *symbolDirShort != "" {
		*symbolDir = *symbolDirShort
	}

	if *listDisplays {
		for i, bounds := range app.DisplayBounds() {
			fmt.Printf("%d: %v width=%d height=%d\n", i, bounds, bounds.Dx(), bounds.Dy())
		}
		return
	}

	if *screen && !*pngMode {
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
		writeResult, err := app.DecodeScreenToPath(app.ScreenDecodeConfig{
			OutputPath:      *output,
			Backend:         *backend,
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
			DecodeWorkers:   *decodeWorkers,
			Timeout:         *timeout,
			Progress:        os.Stderr,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "screen decoder failed: %v\n", err)
			os.Exit(1)
		}
		md5, err := app.FileMD5Hex(writeResult.OutputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "md5 failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("decoded screen -> %s, md5=%s\n", writeResult.OutputPath, md5)
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
	writeResult, err := app.DecodePNGFramesToPathWithBackend(*input, *output, gridSize, *b, *symbolDir, *blockSize, *eccPercent, *colorBits, spec, *backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decoder failed: %v\n", err)
		os.Exit(1)
	}

	md5, err := app.FileMD5Hex(writeResult.OutputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "md5 failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("decoded %s -> %s, md5=%s\n", *input, writeResult.OutputPath, md5)
}

func shortAliases() map[string][]string {
	return map[string][]string{
		"cell":    {"c"},
		"packets": {"p"},
		"R":       {"r"},
		"fps":     {"f"},
		"symbols": {"s"},
	}
}
