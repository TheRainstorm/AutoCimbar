package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	input := flag.String("i", "", "input file")
	outputDir := flag.String("o", "frames", "output PNG frame directory")
	q := flag.Int("Q", 120, "grid size in cells")
	rq := flag.Int("RQ", 0, "reference grid size for 8x8 tiles; when set, actual Q is scaled by 8/tile_width")
	b := flag.Int("B", 1, "screen scale factor")
	redundancy := flag.Int("redundancy", 10, "extra fountain frames as a percentage")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	eccPercent := flag.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; decoder must use the same value")
	backend := flag.String("backend", app.BackendSymbols, "frame backend: symbols or qr")
	colorBits := flag.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; decoder must use the same value")
	shapeBits := flag.Int("shape-bits", 4, "shape bits per cell: 4 uses 16 symbols, 5 uses 32, 6 uses 64; decoder must use the same value")
	tile := flag.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT; decoder must use the same value")
	cell := flag.String("cell", "", "compact cell spec like 8t4s2c: tile width, color bits, shape bits")
	cellShort := flag.String("c", "", "short alias for -cell")
	packetsPerFrame := flag.Int("packets", 1, "independent packets per screen frame; decoder must use the same value")
	packetsShort := flag.Int("p", 0, "short alias for -packets")
	noZstd := flag.Bool("no-zstd", false, "disable default zstd source compression")
	screen := flag.Bool("screen", true, "show frames in a borderless screen window instead of writing PNG files")
	pngMode := flag.Bool("png", false, "write PNG frames instead of screen mode")
	region := flag.String("R", "0", "screen window region SCREEN, X:Y or SCREEN:X:Y; c centers an axis")
	regionShort := flag.String("r", "", "short alias for -R")
	fps := flag.Int("fps", 120, "screen frame rate")
	fpsShort := flag.Int("f", 0, "short alias for -fps")
	addr := flag.String("addr", "127.0.0.1:8080", "screen encoder HTTP listen address")
	open := flag.Bool("open", true, "open the screen encoder page in the default browser")
	listDisplays := flag.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing symbol PNG files named 00.png..; empty uses built-in symbols")
	symbolDirShort := flag.String("s", "", "short alias for -symbols")
	flag.Parse()

	if err := app.ApplyINIConfig(flag.CommandLine, "encoder", shortAliases()); err != nil {
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

	if *input == "" {
		fmt.Fprintln(os.Stderr, "missing -i input file")
		os.Exit(2)
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
		if runtime.GOOS == "windows" {
			fmt.Println("screen encoder opening native Windows window")
		} else {
			fmt.Printf("screen encoder serving http://%s/\n", *addr)
		}
		result, err := app.EncodeFileToScreen(app.ScreenEncodeConfig{
			InputPath:       *input,
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
			NoZstd:          *noZstd,
			Region:          *region,
			FPS:             *fps,
			Addr:            *addr,
			Open:            *open,
			Progress:        os.Stderr,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "screen encoder failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("screen encoded %s (%d bytes), source_payload=%d bytes, compression=%s, transfer=%d bytes, %d source block(s), block=%d bytes, md5=%s, backend=%s\n",
			result.FileName, result.FileSize, result.CompressedSize, app.SourceCompressionName(result.Compression), result.TransferSize, result.BlockCount, result.BlockSize, result.MD5, result.Backend)
		fmt.Printf("tile=%dx%d shape_bits=%d color_bits=%d cell_bits=%d\n",
			result.TileWidth, result.TileHeight, result.ShapeBits, result.ColorBits, result.ShapeBits+result.ColorBits)
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
	result, err := app.EncodeFileToPNGFramesWithBackend(*input, *outputDir, gridSize, *b, *symbolDir, *redundancy, *blockSize, *eccPercent, !*noZstd, *colorBits, spec, *backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("encoded %s (%d bytes) into %d frame(s), source_payload=%d bytes, compression=%s, transfer=%d bytes, %d source block(s), md5=%s, backend=%s\n",
		result.FileName, result.FileSize, len(result.FramePaths), result.CompressedSize, app.SourceCompressionName(result.Compression), result.TransferSize, result.BlockCount, result.MD5, result.Backend)
	fmt.Printf("Q=%d B=%d tile=%dx%d shape_bits=%d color_bits=%d cell_bits=%d cell=%dpx image=%dx%dpx payload=%d bytes/frame block=%d bytes ecc=%d%% parity=%d bytes packet=%d bytes\n",
		result.GridSize, result.Scale, result.TileWidth, result.TileHeight, result.ShapeBits, result.ColorBits, result.ShapeBits+result.ColorBits, result.CellSize, result.ImageSize, result.ImageSize, result.PayloadCapacity, result.BlockSize, result.ECCPercent, result.ECCBytes, result.PacketBytes)
	fmt.Printf("output: %s\n", *outputDir)
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
