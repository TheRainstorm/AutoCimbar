package app

import (
	"errors"
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
