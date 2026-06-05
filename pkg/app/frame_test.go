package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePacketValidatesMagic(t *testing.T) {
	packet := BuildPacket(1234, 7, []byte{1, 2, 3, 4})
	packet[0] = 0

	if _, err := ParsePacket(packet, 4); !errors.Is(err, ErrInvalidFrameMagic) {
		t.Fatalf("ParsePacket error = %v, want ErrInvalidFrameMagic", err)
	}
}

func TestBuildParsePacketRoundTrip(t *testing.T) {
	payload := []byte{9, 8, 7, 6}
	frame, err := ParsePacket(BuildPacket(1234, 7, payload), len(payload))
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}
	if frame.FileSize != 1234 {
		t.Fatalf("FileSize = %d, want 1234", frame.FileSize)
	}
	if frame.FrameID != 7 {
		t.Fatalf("FrameID = %d, want 7", frame.FrameID)
	}
	for i, got := range frame.Payload {
		if got != payload[i] {
			t.Fatalf("Payload[%d] = %d, want %d", i, got, payload[i])
		}
	}
}

func TestParsePacketRejectsCRCMismatch(t *testing.T) {
	packet := BuildPacket(1234, 7, []byte{1, 2, 3, 4})
	packet[len(packet)-1] ^= 0x01
	if _, err := ParsePacket(packet, 4); !errors.Is(err, ErrInvalidFrameCRC) {
		t.Fatalf("ParsePacket error = %v, want ErrInvalidFrameCRC", err)
	}
}

func TestBuildPacketIntoRoundTrip(t *testing.T) {
	payload := []byte{1, 3, 5, 7}
	buf := make([]byte, 0, FrameHeaderSize+len(payload))
	packet := BuildPacketInto(buf, 42, 9, payload)

	frame, err := ParsePacket(packet, len(payload))
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}
	if frame.FileSize != 42 {
		t.Fatalf("FileSize = %d, want 42", frame.FileSize)
	}
	if frame.FrameID != 9 {
		t.Fatalf("FrameID = %d, want 9", frame.FrameID)
	}
	for i, got := range frame.Payload {
		if got != payload[i] {
			t.Fatalf("Payload[%d] = %d, want %d", i, got, payload[i])
		}
	}
}

func TestBuildParseSourceDataRoundTrip(t *testing.T) {
	input := []byte("source payload")
	source, err := ParseSourceData(BuildSourceData(input))
	if err != nil {
		t.Fatalf("ParseSourceData failed: %v", err)
	}
	if source.FileSize != len(input) {
		t.Fatalf("FileSize = %d, want %d", source.FileSize, len(input))
	}
	if source.MD5 != BytesMD5Hex(input) {
		t.Fatalf("MD5 = %s, want %s", source.MD5, BytesMD5Hex(input))
	}
	if !bytes.Equal(source.Payload, input) {
		t.Fatal("payload differs from input")
	}
}

func TestBuildParseSourceDataRoundTripWithoutCompression(t *testing.T) {
	input := []byte("plain source payload")
	source, err := ParseSourceData(BuildSourceDataWithCompression(input, false))
	if err != nil {
		t.Fatalf("ParseSourceData failed: %v", err)
	}
	if source.FileSize != len(input) {
		t.Fatalf("FileSize = %d, want %d", source.FileSize, len(input))
	}
	if source.MD5 != BytesMD5Hex(input) {
		t.Fatalf("MD5 = %s, want %s", source.MD5, BytesMD5Hex(input))
	}
	if source.Compression != SourceCompressionNone {
		t.Fatalf("Compression = %d, want %d", source.Compression, SourceCompressionNone)
	}
	if !bytes.Equal(source.Payload, input) {
		t.Fatal("payload differs from input")
	}
}

func TestParseSourceDataRejectsMD5Mismatch(t *testing.T) {
	sourceData := BuildSourceData([]byte("source payload"))
	sourceData[len(sourceData)-1] ^= 0x01
	if _, err := ParseSourceData(sourceData); err == nil {
		t.Fatal("expected md5 mismatch error")
	}
}

func TestWriteSourceDataToFileRoundTrip(t *testing.T) {
	input := bytes.Repeat([]byte("compressible payload "), 128)
	outputPath := filepath.Join(t.TempDir(), "output.bin")
	if err := os.WriteFile(outputPath, []byte("old output"), 0644); err != nil {
		t.Fatalf("write old output: %v", err)
	}

	source, err := WriteSourceDataToFile(BuildSourceData(input), outputPath)
	if err != nil {
		t.Fatalf("WriteSourceDataToFile failed: %v", err)
	}
	if source.MD5 != BytesMD5Hex(input) {
		t.Fatalf("MD5 = %s, want %s", source.MD5, BytesMD5Hex(input))
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("output differs from input")
	}
}

func TestWriteSourceDataToFileRoundTripWithoutCompression(t *testing.T) {
	input := bytes.Repeat([]byte("plain payload "), 128)
	outputPath := filepath.Join(t.TempDir(), "output.bin")

	source, err := WriteSourceDataToFile(BuildSourceDataWithCompression(input, false), outputPath)
	if err != nil {
		t.Fatalf("WriteSourceDataToFile failed: %v", err)
	}
	if source.Compression != SourceCompressionNone {
		t.Fatalf("Compression = %d, want %d", source.Compression, SourceCompressionNone)
	}
	if source.MD5 != BytesMD5Hex(input) {
		t.Fatalf("MD5 = %s, want %s", source.MD5, BytesMD5Hex(input))
	}
	output, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(output, input) {
		t.Fatal("output differs from input")
	}
}
