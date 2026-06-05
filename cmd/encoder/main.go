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
	b := flag.Int("B", 1, "screen scale factor")
	redundancy := flag.Int("redundancy", 10, "extra fountain frames as a percentage")
	blockSize := flag.Int("block-size", 0, "fountain block size in bytes, 0 uses max frame payload")
	eccPercent := flag.Int("ecc", 3, "per-frame Reed-Solomon ECC percentage; decoder must use the same value")
	colorBits := flag.Int("color-bits", 2, "color bits per cell: 0..8 uses 1..256 colors; decoder must use the same value")
	packetsPerFrame := flag.Int("packets", 1, "independent packets per screen frame; decoder must use the same value")
	noZstd := flag.Bool("no-zstd", false, "disable default zstd source compression")
	screen := flag.Bool("screen", false, "show frames in a borderless screen window instead of writing PNG files")
	region := flag.String("R", "0:0", "screen window region X:Y or SCREEN:X:Y, negative values anchor from right/bottom")
	fps := flag.Int("fps", 60, "screen frame rate")
	addr := flag.String("addr", "127.0.0.1:8080", "screen encoder HTTP listen address")
	open := flag.Bool("open", true, "open the screen encoder page in the default browser")
	listDisplays := flag.Bool("list-displays", false, "list detected display indexes and bounds")
	symbolDir := flag.String("symbols", app.DefaultSymbolDir, "optional directory containing 16 libcimbar bitmap symbols; empty uses built-in symbols")
	flag.Parse()

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

	if *screen {
		if runtime.GOOS == "windows" {
			fmt.Println("screen encoder opening native Windows window")
		} else {
			fmt.Printf("screen encoder serving http://%s/\n", *addr)
		}
		result, err := app.EncodeFileToScreen(app.ScreenEncodeConfig{
			InputPath:       *input,
			GridSize:        *q,
			Scale:           *b,
			SymbolDir:       *symbolDir,
			BlockSize:       *blockSize,
			ECCPercent:      *eccPercent,
			ColorBits:       *colorBits,
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
		fmt.Printf("screen encoded %d bytes, source_payload=%d bytes, compression=%s, transfer=%d bytes, %d source block(s), block=%d bytes, md5=%s\n",
			result.FileSize, result.CompressedSize, app.SourceCompressionName(result.Compression), result.TransferSize, result.BlockCount, result.BlockSize, result.MD5)
		return
	}

	result, err := app.EncodeFileToPNGFramesWithOptions(*input, *outputDir, *q, *b, *symbolDir, *redundancy, *blockSize, *eccPercent, !*noZstd, *colorBits)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encoder failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("encoded %d bytes into %d frame(s), source_payload=%d bytes, compression=%s, transfer=%d bytes, %d source block(s), md5=%s\n",
		result.FileSize, len(result.FramePaths), result.CompressedSize, app.SourceCompressionName(result.Compression), result.TransferSize, result.BlockCount, result.MD5)
	fmt.Printf("Q=%d B=%d color_bits=%d cell=%dpx image=%dx%dpx payload=%d bytes/frame block=%d bytes ecc=%d%% parity=%d bytes packet=%d bytes\n",
		result.GridSize, result.Scale, result.ColorBits, result.CellSize, result.ImageSize, result.ImageSize, result.PayloadCapacity, result.BlockSize, result.ECCPercent, result.ECCBytes, result.PacketBytes)
	fmt.Printf("output: %s\n", *outputDir)
}
