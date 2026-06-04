package fountain

import (
	"bytes"
	"testing"
)

func TestSystemBlocksRoundTrip(t *testing.T) {
	data := []byte("hello fountain code, this must round trip")
	enc, err := NewEncoder(data, 7)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	dec, err := NewDecoder(enc.FileSize(), enc.BlockSize(), enc.BlockCount())
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	for i := 0; i < enc.BlockCount(); i++ {
		block := enc.Encode(uint32(i))
		added, err := dec.AddFrame(block.FrameID, block.Data)
		if err != nil {
			t.Fatalf("Add failed: %v", err)
		}
		if !added {
			t.Fatalf("system block %d was not independent", i)
		}
	}

	out, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("decoded mismatch: got %q want %q", out, data)
	}
}

func TestRecoversMissingSystemBlocksWithRedundancy(t *testing.T) {
	data := bytes.Repeat([]byte("abcdef0123456789"), 16)
	enc, err := NewEncoder(data, 23)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	dec, err := NewDecoder(enc.FileSize(), enc.BlockSize(), enc.BlockCount())
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	for i := 0; i < enc.BlockCount()+64 && !dec.Complete(); i++ {
		if i == 2 || i == 5 {
			continue
		}
		block := enc.Encode(uint32(i))
		if _, err := dec.AddFrame(block.FrameID, block.Data); err != nil {
			t.Fatalf("Add frame %d failed: %v", i, err)
		}
	}

	if !dec.Complete() {
		t.Fatalf("decoder rank %d, want %d", dec.Rank(), enc.BlockCount())
	}

	out, err := dec.Decode()
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("decoded data mismatch")
	}
}

func TestNewDecoderRejectsHugeBlockCount(t *testing.T) {
	if _, err := NewDecoder(1, 1, MaxDecoderBlockCount+1); err == nil {
		t.Fatal("expected huge block count error")
	}
}
