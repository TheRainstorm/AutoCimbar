package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
