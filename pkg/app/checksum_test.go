package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBytesMD5Hex(t *testing.T) {
	got := BytesMD5Hex([]byte("hello"))
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Fatalf("BytesMD5Hex = %s, want %s", got, want)
	}
}

func TestFileMD5Hex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.bin")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	got, err := FileMD5Hex(path)
	if err != nil {
		t.Fatalf("FileMD5Hex failed: %v", err)
	}
	want := "5d41402abc4b2a76b9719d911017c592"
	if got != want {
		t.Fatalf("FileMD5Hex = %s, want %s", got, want)
	}
}
