package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/autocambar/autocambar/pkg/symbol"
	"github.com/autocambar/autocambar/pkg/tilegen"
)

func TestPNGFrameFountainRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.bin")
	outputPath := filepath.Join(dir, "output.bin")
	frameDir := filepath.Join(dir, "frames")

	input := deterministicBytes(8192)
	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	result, err := EncodeFileToPNGFrames(inputPath, frameDir, 50, 1, testSymbolDir(t), 10, 0, 0)
	if err != nil {
		t.Fatalf("EncodeFileToPNGFrames failed: %v", err)
	}
	if result.BlockCount != 5 {
		t.Fatalf("block count = %d, want 5", result.BlockCount)
	}

	if err := DecodePNGFramesToFile(frameDir, outputPath, 50, 1, testSymbolDir(t), 0, 0); err != nil {
		t.Fatalf("DecodePNGFramesToFile failed: %v", err)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("decoded output differs from input")
	}
}

func TestPNGFrameFountainRecoversDroppedFrames(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.bin")
	outputPath := filepath.Join(dir, "output.bin")
	frameDir := filepath.Join(dir, "frames")

	input := deterministicBytes(8192)
	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	result, err := EncodeFileToPNGFrames(inputPath, frameDir, 50, 1, testSymbolDir(t), 100, 0, 0)
	if err != nil {
		t.Fatalf("EncodeFileToPNGFrames failed: %v", err)
	}
	if len(result.FramePaths) < result.BlockCount+2 {
		t.Fatalf("not enough redundant frames: got %d frames for %d blocks", len(result.FramePaths), result.BlockCount)
	}

	for _, name := range []string{"frame_000002.png", "frame_000004.png"} {
		if err := os.Remove(filepath.Join(frameDir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}

	if err := DecodePNGFramesToFile(frameDir, outputPath, 50, 1, testSymbolDir(t), 0, 0); err != nil {
		t.Fatalf("DecodePNGFramesToFile failed: %v", err)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("decoded output differs from input after dropped frames")
	}
}

func TestPNGFrameFountainRoundTripWithECC(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.bin")
	outputPath := filepath.Join(dir, "output.bin")
	frameDir := filepath.Join(dir, "frames")

	input := deterministicBytes(8192)
	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	result, err := EncodeFileToPNGFrames(inputPath, frameDir, 50, 1, testSymbolDir(t), 10, 0, 20)
	if err != nil {
		t.Fatalf("EncodeFileToPNGFrames failed: %v", err)
	}
	if result.ECCBytes <= 0 {
		t.Fatalf("ECCBytes = %d, want > 0", result.ECCBytes)
	}
	if result.PacketBytes > result.FrameCapacity {
		t.Fatalf("packet bytes = %d exceeds frame capacity %d", result.PacketBytes, result.FrameCapacity)
	}

	if err := DecodePNGFramesToFile(frameDir, outputPath, 50, 1, testSymbolDir(t), 0, 20); err != nil {
		t.Fatalf("DecodePNGFramesToFile failed: %v", err)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("decoded output differs from input")
	}
}

func TestPNGFrameQRRoundsTrip(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.bin")
	outputPath := filepath.Join(dir, "output.bin")
	frameDir := filepath.Join(dir, "frames")

	input := deterministicBytes(1024)
	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	result, err := EncodeFileToPNGFramesWithBackend(inputPath, frameDir, 33, 4, "", 10, 0, 0, true, 0, symbol.DefaultSpec(), BackendQR)
	if err != nil {
		t.Fatalf("EncodeFileToPNGFramesWithBackend QR failed: %v", err)
	}
	if result.Backend != BackendQR {
		t.Fatalf("Backend = %q, want %q", result.Backend, BackendQR)
	}
	if result.FrameCapacity <= FrameHeaderSize {
		t.Fatalf("FrameCapacity = %d, want > %d", result.FrameCapacity, FrameHeaderSize)
	}

	if err := DecodePNGFramesToFileWithBackend(frameDir, outputPath, 33, 4, "", 0, 0, 0, symbol.DefaultSpec(), BackendQR); err != nil {
		t.Fatalf("DecodePNGFramesToFileWithBackend QR failed: %v", err)
	}

	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("decoded QR output differs from input")
	}
}

func TestPNGFrameFountainRoundTripVariableColorBits(t *testing.T) {
	for _, colorBits := range []int{0, 1, 3, 4, 5, 8} {
		t.Run(fmt.Sprintf("colorBits%d", colorBits), func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input.bin")
			outputPath := filepath.Join(dir, "output.bin")
			frameDir := filepath.Join(dir, "frames")

			input := deterministicBytes(8192)
			if err := os.WriteFile(inputPath, input, 0644); err != nil {
				t.Fatalf("write input: %v", err)
			}

			result, err := EncodeFileToPNGFramesWithOptions(inputPath, frameDir, 50, 1, testSymbolDir(t), 10, 0, 3, true, colorBits)
			if err != nil {
				t.Fatalf("EncodeFileToPNGFramesWithOptions failed: %v", err)
			}
			if result.ColorBits != colorBits {
				t.Fatalf("ColorBits = %d, want %d", result.ColorBits, colorBits)
			}
			if result.FrameCapacity != GridCapacityBytesWithColorBits(50, colorBits) {
				t.Fatalf("FrameCapacity = %d, want %d", result.FrameCapacity, GridCapacityBytesWithColorBits(50, colorBits))
			}

			if err := DecodePNGFramesToFileWithColorBits(frameDir, outputPath, 50, 1, testSymbolDir(t), 0, 3, colorBits); err != nil {
				t.Fatalf("DecodePNGFramesToFileWithColorBits failed: %v", err)
			}

			output, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if !bytes.Equal(output, input) {
				t.Fatal("decoded output differs from input")
			}
		})
	}
}

func TestPNGFrameFountainRoundTripVariableShapeSpecs(t *testing.T) {
	tests := []struct {
		name      string
		spec      symbol.Spec
		gridSize  int
		colorBits int
	}{
		{"8x8_5bit", mustTestSpec(t, 8, 8, 5), 44, 2},
		{"8x8_6bit", mustTestSpec(t, 8, 8, 6), 40, 2},
		{"6x6_4bit", mustTestSpec(t, 6, 6, 4), 50, 2},
		{"4x4_3bit", mustTestSpec(t, 4, 4, 3), 56, 2},
		{"4x4_2bit", mustTestSpec(t, 4, 4, 2), 60, 2},
		{"4x4_4bit", mustTestSpec(t, 4, 4, 4), 52, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input.bin")
			outputPath := filepath.Join(dir, "output.bin")
			frameDir := filepath.Join(dir, "frames")
			symbolDir := generateTestSymbols(t, dir, tt.spec)

			input := deterministicBytes(2048)
			if err := os.WriteFile(inputPath, input, 0644); err != nil {
				t.Fatalf("write input: %v", err)
			}

			result, err := EncodeFileToPNGFramesWithSpec(inputPath, frameDir, tt.gridSize, 1, symbolDir, 10, 0, 3, true, tt.colorBits, tt.spec)
			if err != nil {
				t.Fatalf("EncodeFileToPNGFramesWithSpec failed: %v", err)
			}
			if result.ShapeBits != tt.spec.ShapeBits || result.TileWidth != tt.spec.Width || result.TileHeight != tt.spec.Height {
				t.Fatalf("result spec = %dx%d/%d, want %dx%d/%d", result.TileWidth, result.TileHeight, result.ShapeBits, tt.spec.Width, tt.spec.Height, tt.spec.ShapeBits)
			}

			if err := DecodePNGFramesToFileWithSpec(frameDir, outputPath, tt.gridSize, 1, symbolDir, 0, 3, tt.colorBits, tt.spec); err != nil {
				t.Fatalf("DecodePNGFramesToFileWithSpec failed: %v", err)
			}

			output, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if !bytes.Equal(output, input) {
				t.Fatal("decoded output differs from input")
			}
		})
	}
}

func deterministicBytes(size int) []byte {
	data := make([]byte, size)
	var x uint32 = 0x12345678
	for i := range data {
		x = x*1664525 + 1013904223
		data[i] = byte(x >> 24)
	}
	return data
}

func testSymbolDir(t *testing.T) string {
	t.Helper()
	return DefaultSymbolDir
}

func mustTestSpec(t *testing.T, width int, height int, shapeBits int) symbol.Spec {
	t.Helper()
	spec, err := symbol.NewSpec(width, height, shapeBits)
	if err != nil {
		t.Fatalf("NewSpec: %v", err)
	}
	return spec
}

func generateTestSymbols(t *testing.T, baseDir string, spec symbol.Spec) string {
	t.Helper()
	result, err := tilegen.Generate(tilegen.Options{
		Spec:     spec,
		Seed:     uint64(spec.Width*100 + spec.Height*10 + spec.ShapeBits),
		Attempts: 3000,
	})
	if err != nil {
		t.Fatalf("tilegen.Generate: %v", err)
	}
	dir := filepath.Join(baseDir, fmt.Sprintf("symbols_%dx%d_%d", spec.Width, spec.Height, spec.ShapeBits))
	if err := tilegen.Save(result, dir); err != nil {
		t.Fatalf("tilegen.Save: %v", err)
	}
	return dir
}
