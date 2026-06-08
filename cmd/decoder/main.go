package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autocambar/autocambar/pkg/app"
)

func main() {
	fs := flag.CommandLine
	installUsage(fs)
	input := fs.String("i", "frames", "input PNG frame file or directory")
	output := fs.String("o", ".", "output file or directory; when omitted or a directory, uses the sender file name")
	q := fs.Int("Q", 120, "grid size in cells")
	rq := fs.Int("RQ", 0, "reference grid size for 8x8 tiles; when set, actual Q is scaled by 8/tile_width")
	b := fs.Int("B", 1, "screen scale factor")
	eccPercent := fs.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; encoder must use the same value")
	backend := fs.String("backend", app.BackendSymbols, "frame backend: symbols or qr")
	colorBits := fs.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; encoder must use the same value")
	shapeBits := fs.Int("shape-bits", 4, "shape bits per cell: 4 uses 16 symbols, 5 uses 32, 6 uses 64; encoder must use the same value")
	tile := fs.String("tile", "8x8", "logical shape tile size WIDTHxHEIGHT; encoder must use the same value")
	cell := fs.String("cell", "", "compact cell spec like 8t4s2c: tile width, shape bits, color bits")
	cellShort := fs.String("c", "", "short alias for -cell")
	packetsPerFrame := fs.Int("packets", 1, "independent packets per screen frame; encoder must use the same value")
	packetsShort := fs.Int("p", 0, "short alias for -packets")
	screen := fs.Bool("screen", true, "capture frames from screen instead of reading PNG files")
	pngMode := fs.Bool("png", false, "read PNG frames instead of screen mode")
	region := fs.String("R", "0", "screen capture region SCREEN, X:Y or SCREEN:X:Y; c centers an axis")
	regionShort := fs.String("r", "", "short alias for -R")
	fps := fs.Int("fps", 120, "screen capture rate")
	fpsShort := fs.Int("f", 0, "short alias for -fps")
	decodeWorkers := fs.Int("decode-workers", 0, "parallel screen decode workers, 0 chooses automatically")
	captureBackend := fs.String("capture-backend", app.CaptureBackendAuto, "screen capture backend: auto, dxgi, or gdi")
	debugCapture := fs.String("debug-capture", "", "directory for first 60 captured frames; files are named <cell>_NNN.png")
	verbose := fs.Bool("v", false, "print verbose decoder performance diagnostics")
	listDisplays := fs.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := fs.String("symbols", app.DefaultSymbolDir, "optional directory containing symbol PNG files named 00.png..; empty uses built-in symbols")
	symbolDirShort := fs.String("s", "", "short alias for -symbols")
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
		printRuntimeConfig(os.Stderr, runtimeConfig{
			Mode: "screen", Input: *input, Output: *output, Backend: *backend, Q: *q, RQ: *rq, ResolvedQ: gridSize, Scale: *b,
			Tile: *tile, ShapeBits: *shapeBits, ColorBits: *colorBits, Cell: *cell, ECC: *eccPercent, Packets: *packetsPerFrame,
			Region: *region, FPS: *fps, CaptureBackend: *captureBackend, DebugCapture: *debugCapture, DecodeWorkers: *decodeWorkers,
			Verbose: *verbose,
		})
		writeResult, err := app.DecodeScreenToPath(app.ScreenDecodeConfig{
			OutputPath:       *output,
			Backend:          *backend,
			GridSize:         gridSize,
			Scale:            *b,
			SymbolDir:        *symbolDir,
			ECCPercent:       *eccPercent,
			ColorBits:        *colorBits,
			ShapeBits:        *shapeBits,
			Tile:             *tile,
			PacketsPerFrame:  *packetsPerFrame,
			Region:           *region,
			FPS:              *fps,
			DecodeWorkers:    *decodeWorkers,
			CaptureBackend:   *captureBackend,
			DebugCapturePath: *debugCapture,
			Verbose:          *verbose,
			Progress:         os.Stderr,
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
	printRuntimeConfig(os.Stderr, runtimeConfig{
		Mode: "png", Input: *input, Output: *output, Backend: *backend, Q: *q, RQ: *rq, ResolvedQ: gridSize, Scale: *b,
		Tile: *tile, ShapeBits: *shapeBits, ColorBits: *colorBits, Cell: *cell, ECC: *eccPercent, Packets: *packetsPerFrame,
		Region: *region, FPS: *fps, CaptureBackend: *captureBackend, DebugCapture: *debugCapture, DecodeWorkers: *decodeWorkers,
		Verbose: *verbose,
	})
	writeResult, err := app.DecodePNGFramesToPathWithBackend(*input, *output, gridSize, *b, *symbolDir, 0, *eccPercent, *colorBits, spec, *backend)
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

type runtimeConfig struct {
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
	CaptureBackend string
	DebugCapture   string
	DecodeWorkers  int
	Verbose        bool
}

func printRuntimeConfig(out *os.File, cfg runtimeConfig) {
	cell := cfg.Cell
	if cell == "" {
		cell = app.CellSpecName(cfg.Tile, cfg.ShapeBits, cfg.ColorBits)
	}
	fmt.Fprintf(out, "decoder frame-format: backend=%s cell=%s ecc=%d packets=%d zstd=auto\n",
		cfg.Backend, cell, cfg.ECC, cfg.Packets)
	fmt.Fprintf(out, "decoder runtime: mode=%s input=%q output=%q RQ=%d Q=%d resolved_Q=%d B=%d fps=%d region=%s capture_backend=%s debug_capture=%q workers=%d\n",
		cfg.Mode, cfg.Input, cfg.Output, cfg.RQ, cfg.Q, cfg.ResolvedQ, cfg.Scale, cfg.FPS, cfg.Region, cfg.CaptureBackend, cfg.DebugCapture, cfg.DecodeWorkers)
	if cfg.Verbose {
		fmt.Fprintln(out, "decoder verbose: enabled")
	}
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

func installUsage(fs *flag.FlagSet) {
	fs.Usage = func() {
		exe := filepath.Base(os.Args[0])
		fmt.Fprintf(fs.Output(), "Usage: %s [options]\n\n", exe)
		fmt.Fprintln(fs.Output(), "Frame format options (encoder and decoder must match):")
		printOption(fs, "-backend", "symbols|qr", "Frame backend. symbols is the high-throughput AutoCimBar path; qr is for QR-code comparison.")
		printOption(fs, "-c, -cell", "8t4s2c", "Compact cell format: tile size, shape bits, color bits. Example: 4t4s8c.")
		printOption(fs, "-ecc", "3", "Per-packet Reed-Solomon ECC percentage.")
		printOption(fs, "-p, -packets", "1", "Independent packets packed into each screen frame.")
		printOption(fs, "zstd", "auto", "Source compression is detected from sender metadata.")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Runtime options:")
		printOption(fs, "-o", "PATH", "Output file or directory; directories use the sender file name.")
		printOption(fs, "-RQ", "N", "Reference grid size using 8x8 tiles; actual Q scales when tile size changes.")
		printOption(fs, "-Q", "N", "Raw grid/cell count. RQ takes precedence when set.")
		printOption(fs, "-B", "N", "Screen scale factor.")
		printOption(fs, "-r, -R", "SCREEN[:X:Y]", "Screen capture region. SCREEN, X:Y, or SCREEN:X:Y; X/Y accept -0 and c.")
		printOption(fs, "-f, -fps", "N", "Screen capture frame rate.")
		printOption(fs, "-list-displays", "", "List display indexes and bounds.")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "Capture backend options:")
		printOption(fs, "-capture-backend", "auto|dxgi|gdi", "Screen capture backend. DXGI is fastest, but HDR/color-managed displays can break high color-bit modes; use SDR or GDI when colors do not decode.")
		printOption(fs, "-debug-capture", "DIR", "Save the first 60 captured frames as DIR/<cell>_NNN.png; creates DIR when missing.")
		printOption(fs, "-decode-workers", "N", "Parallel screen decode workers, 0 chooses automatically.")
		printOption(fs, "-v", "", "Print verbose decoder diagnostics: capture/decode/packet milliseconds, queue drops, and worker count.")
		fmt.Fprintln(fs.Output())
		fmt.Fprintln(fs.Output(), "PNG and advanced options:")
		printOption(fs, "-png", "", "Read PNG frames instead of screen capture mode.")
		printOption(fs, "-i", "PATH", "PNG frame file or directory.")
		printOption(fs, "-symbols, -s", "DIR", "Override built-in symbol tiles with a directory of PNG symbols.")
	}
}

func printOption(fs *flag.FlagSet, name string, value string, help string) {
	if value != "" {
		fmt.Fprintf(fs.Output(), "  %-18s %-12s %s\n", name, value, help)
		return
	}
	fmt.Fprintf(fs.Output(), "  %-31s %s\n", name, help)
}
