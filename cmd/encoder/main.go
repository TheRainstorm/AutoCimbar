package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	fs := flag.CommandLine
	installUsage(fs, "encoder")
	input := fs.String("i", "", "input file")
	outputDir := fs.String("o", "frames", "output PNG frame directory")
	q := fs.Int("Q", 120, "grid size in cells")
	rq := fs.Int("RQ", 0, "reference grid size for 8x8 tiles; when set, actual Q is scaled by 8/tile_width")
	b := fs.Int("B", 1, "screen scale factor")
	redundancy := fs.Int("redundancy", 10, "extra fountain frames as a percentage")
	eccPercent := fs.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; decoder must use the same value")
	backend := fs.String("backend", app.BackendSymbols, "frame backend: symbols or qr")
	colorBits := fs.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; decoder must use the same value")
	shapeBits := fs.Int("shape-bits", 4, "shape bits per cell: 4 uses 16 symbols, 5 uses 32, 6 uses 64; decoder must use the same value")
	tile := fs.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT; decoder must use the same value")
	cell := fs.String("cell", "", "compact cell spec like 8t4s2c: tile width, shape bits, color bits")
	cellShort := fs.String("c", "", "short alias for -cell")
	packetsPerFrame := fs.Int("packets", 1, "independent packets per screen frame; decoder must use the same value")
	packetsShort := fs.Int("p", 0, "short alias for -packets")
	noZstd := fs.Bool("no-zstd", false, "disable default zstd source compression")
	screen := fs.Bool("screen", true, "show frames in a borderless screen window instead of writing PNG files")
	pngMode := fs.Bool("png", false, "write PNG frames instead of screen mode")
	region := fs.String("R", "0", "screen window region SCREEN, X:Y or SCREEN:X:Y; c centers an axis")
	regionShort := fs.String("r", "", "short alias for -R")
	fps := fs.Int("fps", 120, "screen frame rate")
	fpsShort := fs.Int("f", 0, "short alias for -fps")
	addr := fs.String("addr", "127.0.0.1:8080", "screen encoder HTTP listen address")
	open := fs.Bool("open", true, "open the screen encoder page in the default browser")
	listDisplays := fs.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := fs.String("symbols", app.DefaultSymbolDir, "optional directory containing symbol PNG files named 00.png..; empty uses built-in symbols")
	symbolDirShort := fs.String("s", "", "short alias for -symbols")
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
		printRuntimeConfig(os.Stderr, runtimeConfig{
			Command: "encoder", Mode: "screen", Input: *input, Output: "", Backend: *backend, Q: *q, RQ: *rq, ResolvedQ: gridSize,
			Scale: *b, Tile: *tile, ShapeBits: *shapeBits, ColorBits: *colorBits, Cell: *cell, ECC: *eccPercent, Packets: *packetsPerFrame,
			Region: *region, FPS: *fps, Zstd: !*noZstd, Redundancy: *redundancy,
		})
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
	printRuntimeConfig(os.Stderr, runtimeConfig{
		Command: "encoder", Mode: "png", Input: *input, Output: *outputDir, Backend: *backend, Q: *q, RQ: *rq, ResolvedQ: gridSize,
		Scale: *b, Tile: *tile, ShapeBits: *shapeBits, ColorBits: *colorBits, Cell: *cell, ECC: *eccPercent, Packets: *packetsPerFrame,
		Region: *region, FPS: *fps, Zstd: !*noZstd, Redundancy: *redundancy,
	})
	result, err := app.EncodeFileToPNGFramesWithBackend(*input, *outputDir, gridSize, *b, *symbolDir, *redundancy, 0, *eccPercent, !*noZstd, *colorBits, spec, *backend)
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

type runtimeConfig struct {
	Command        string
	Mode           string
	Input          string
	Output         string
	Backend        string
	Q              int
	RQ             int
	ResolvedQ      int
	Scale          int
	Tile           string
	ShapeBits      int
	ColorBits      int
	Cell           string
	ECC            int
	Packets        int
	Region         string
	FPS            int
	Zstd           bool
	Redundancy     int
	CaptureBackend string
	DebugCapture   string
}

func printRuntimeConfig(out *os.File, cfg runtimeConfig) {
	cell := cfg.Cell
	if cell == "" {
		cell = app.CellSpecName(cfg.Tile, cfg.ShapeBits, cfg.ColorBits)
	}
	fmt.Fprintf(out, "%s frame-format: backend=%s cell=%s ecc=%d packets=%d zstd=%t\n",
		cfg.Command, cfg.Backend, cell, cfg.ECC, cfg.Packets, cfg.Zstd)
	fmt.Fprintf(out, "%s runtime: mode=%s input=%q output=%q RQ=%d Q=%d resolved_Q=%d B=%d fps=%d region=%s redundancy=%d\n",
		cfg.Command, cfg.Mode, cfg.Input, cfg.Output, cfg.RQ, cfg.Q, cfg.ResolvedQ, cfg.Scale, cfg.FPS, cfg.Region, cfg.Redundancy)
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

func installUsage(fs *flag.FlagSet, command string) {
	fs.Usage = func() {
		exe := filepath.Base(os.Args[0])
		fmt.Fprintf(fs.Output(), "Usage: %s -i FILE [options]\n\n", exe)
		fmt.Fprintln(fs.Output(), "Frame format options (encoder and decoder must match):")
		printOption(fs, "-backend", "symbols|qr", "Frame backend. symbols is the high-throughput AutoCimBar path; qr is for QR-code comparison.")
		printOption(fs, "-c, -cell", "8t4s2c", "Compact cell format: tile size, shape bits, color bits. Example: 4t4s8c.")
		printOption(fs, "-ecc", "3", "Per-packet Reed-Solomon ECC percentage.")
		printOption(fs, "-p, -packets", "1", "Independent packets packed into each screen frame.")
		printOption(fs, "-no-zstd", "", "Disable default zstd source compression.")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Runtime options:")
		printOption(fs, "-i", "FILE", "Input file.")
		printOption(fs, "-RQ", "N", "Reference grid size using 8x8 tiles; actual Q scales when tile size changes.")
		printOption(fs, "-Q", "N", "Raw grid/cell count. RQ takes precedence when set.")
		printOption(fs, "-B", "N", "Screen scale factor.")
		printOption(fs, "-r, -R", "SCREEN[:X:Y]", "Screen placement. SCREEN, X:Y, or SCREEN:X:Y; X/Y accept -0 and c.")
		printOption(fs, "-f, -fps", "N", "Screen refresh frame rate.")
		printOption(fs, "-list-displays", "", "List display indexes and bounds.")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "PNG and advanced options:")
		printOption(fs, "-png", "", "Write PNG frames instead of using screen mode.")
		printOption(fs, "-o", "DIR", "PNG output frame directory.")
		printOption(fs, "-redundancy", "10", "PNG mode extra fountain frames, percentage.")
		printOption(fs, "-symbols, -s", "DIR", "Override built-in symbol tiles with a directory of PNG symbols.")
		_ = command
	}
}

func printOption(fs *flag.FlagSet, name string, value string, help string) {
	if value != "" {
		fmt.Fprintf(fs.Output(), "  %-18s %-12s %s\n", name, value, help)
		return
	}
	fmt.Fprintf(fs.Output(), "  %-31s %s\n", name, help)
}
