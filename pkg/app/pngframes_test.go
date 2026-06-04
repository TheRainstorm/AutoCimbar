package app

import (
	"bytes"
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

	result, err := EncodeFileToPNGFrames(inputPath, frameDir, 50, 1, testSymbolDir(t), 10, 0)
	if err != nil {
		t.Fatalf("EncodeFileToPNGFrames failed: %v", err)
	}
	if result.BlockCount != 5 {
		t.Fatalf("block count = %d, want 5", result.BlockCount)
	}

	if err := DecodePNGFramesToFile(frameDir, outputPath, 50, 1, testSymbolDir(t), 0); err != nil {
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

	result, err := EncodeFileToPNGFrames(inputPath, frameDir, 50, 1, testSymbolDir(t), 100, 0)
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

	if err := DecodePNGFramesToFile(frameDir, outputPath, 50, 1, testSymbolDir(t), 0); err != nil {
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
	path := filepath.Join("..", "..", DefaultSymbolDir)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("symbol dir not found: %v", err)
	}
	return path
}
