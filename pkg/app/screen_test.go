package app

import (
	"testing"

	"github.com/autocambar/autocambar/pkg/codec"
	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/fountain"
)

func TestResolveAxis(t *testing.T) {
	tests := []struct {
		name  string
		token string
		min   int
		max   int
		size  int
		want  int
	}{
		{name: "from left", token: "12", min: 100, max: 500, size: 80, want: 112},
		{name: "right edge", token: "-0", min: 100, max: 500, size: 80, want: 420},
		{name: "right offset", token: "-15", min: 100, max: 500, size: 80, want: 405},
		{name: "plain zero", token: "0", min: -200, max: 300, size: 50, want: -200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAxis(tt.token, tt.min, tt.max, tt.size)
			if err != nil {
				t.Fatalf("resolveAxis failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveAxis = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseRegionSpec(t *testing.T) {
	tests := []struct {
		name                string
		region              string
		allowImplicitScreen bool
		wantScreen          int
		wantX               string
		wantY               string
		wantErr             bool
	}{
		{name: "implicit screen", region: "-0:-0", allowImplicitScreen: true, wantScreen: 0, wantX: "-0", wantY: "-0"},
		{name: "explicit screen", region: "2:10:-5", allowImplicitScreen: true, wantScreen: 2, wantX: "10", wantY: "-5"},
		{name: "decoder requires screen", region: "10:-5", allowImplicitScreen: false, wantErr: true},
		{name: "decoder explicit screen", region: "1:10:-5", allowImplicitScreen: false, wantScreen: 1, wantX: "10", wantY: "-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen, x, y, err := parseRegionSpec(tt.region, tt.allowImplicitScreen)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRegionSpec failed: %v", err)
			}
			if screen != tt.wantScreen || x != tt.wantX || y != tt.wantY {
				t.Fatalf("got %d:%s:%s, want %d:%s:%s", screen, x, y, tt.wantScreen, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestScreenFrameSourceReturnsDecodableFrame(t *testing.T) {
	symRec, err := LoadLibcimbarSymbols(testSymbolDir(t))
	if err != nil {
		t.Fatalf("LoadLibcimbarSymbols failed: %v", err)
	}

	gridSize := 20
	scale := 1
	blockSize := PayloadCapacityBytes(gridSize)
	fountainEnc, err := fountain.NewEncoder([]byte("screen frame test"), blockSize)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	source := &screenFrameSource{
		codecEnc:    codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize),
		fountainEnc: fountainEnc,
		fileSize:    len([]byte("screen frame test")),
		width:       gridSize * CellSize(scale),
		height:      gridSize * CellSize(scale),
	}

	img, err := source.NextImage()
	if err != nil {
		t.Fatalf("NextImage failed: %v", err)
	}
	packet, err := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize).Decode(img)
	if err != nil {
		t.Fatalf("decode codec frame: %v", err)
	}
	frame, err := ParsePacket(packet, blockSize)
	if err != nil {
		t.Fatalf("ParsePacket failed: %v", err)
	}
	if frame.FileSize != len([]byte("screen frame test")) {
		t.Fatalf("FileSize = %d", frame.FileSize)
	}
	if frame.FrameID != 0 {
		t.Fatalf("FrameID = %d, want 0", frame.FrameID)
	}
	if len(frame.Payload) != blockSize {
		t.Fatalf("payload len = %d, want %d", len(frame.Payload), blockSize)
	}
}

func TestScreenFrameSourceReturnsMultiplePackets(t *testing.T) {
	symRec, err := LoadLibcimbarSymbols(testSymbolDir(t))
	if err != nil {
		t.Fatalf("LoadLibcimbarSymbols failed: %v", err)
	}

	gridSize := 40
	scale := 1
	packetsPerFrame := 2
	blockSize, err := PayloadCapacityBytesWithECCAndColorBitsAndPackets(gridSize, 3, codec.ColorBits, packetsPerFrame)
	if err != nil {
		t.Fatalf("PayloadCapacityBytesWithECCAndColorBitsAndPackets failed: %v", err)
	}
	packetCodec, err := NewFramePacketCodecWithColorBitsAndPackets(gridSize, 3, blockSize, codec.ColorBits, packetsPerFrame)
	if err != nil {
		t.Fatalf("NewFramePacketCodecWithColorBitsAndPackets failed: %v", err)
	}
	fountainEnc, err := fountain.NewEncoder(deterministicBytes(blockSize*3), blockSize)
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	source := &screenFrameSource{
		codecEnc:        codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize),
		fountainEnc:     fountainEnc,
		fileSize:        fountainEnc.FileSize(),
		width:           gridSize * CellSize(scale),
		height:          gridSize * CellSize(scale),
		packetsPerFrame: packetsPerFrame,
		packetCodec:     packetCodec,
	}

	img, err := source.NextImage()
	if err != nil {
		t.Fatalf("NextImage failed: %v", err)
	}
	encodedFrame, err := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize).Decode(img)
	if err != nil {
		t.Fatalf("decode codec frame: %v", err)
	}
	var packetBuf []byte
	for i := 0; i < packetsPerFrame; i++ {
		start := i * packetCodec.EncodedSize()
		end := start + packetCodec.EncodedSize()
		packet, err := packetCodec.DecodeInto(encodedFrame[start:end], packetBuf)
		if err != nil {
			t.Fatalf("DecodeInto packet %d failed: %v", i, err)
		}
		packetBuf = packet
		frame, err := ParsePacket(packet, blockSize)
		if err != nil {
			t.Fatalf("ParsePacket %d failed: %v", i, err)
		}
		if frame.FrameID != uint32(i) {
			t.Fatalf("packet %d frame id = %d, want %d", i, frame.FrameID, i)
		}
		if frame.FileSize != fountainEnc.FileSize() {
			t.Fatalf("packet %d file size = %d, want %d", i, frame.FileSize, fountainEnc.FileSize())
		}
	}
}

func BenchmarkScreenFrameSourceNextBGRASystematic(b *testing.B) {
	symRec, err := LoadLibcimbarSymbols(DefaultSymbolDir)
	if err != nil {
		b.Fatalf("LoadLibcimbarSymbols failed: %v", err)
	}

	gridSize := 80
	scale := 1
	blockSize := PayloadCapacityBytes(gridSize)
	fountainEnc, err := fountain.NewEncoder(deterministicBytes(64*1024*1024), blockSize)
	if err != nil {
		b.Fatalf("NewEncoder failed: %v", err)
	}
	source := &screenFrameSource{
		codecEnc:    codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize),
		fountainEnc: fountainEnc,
		fileSize:    fountainEnc.FileSize(),
		width:       gridSize * CellSize(scale),
		height:      gridSize * CellSize(scale),
	}

	var dst []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		dst, err = source.NextBGRA(dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkScreenFrameSourceNextBGRARedundant(b *testing.B) {
	symRec, err := LoadLibcimbarSymbols(DefaultSymbolDir)
	if err != nil {
		b.Fatalf("LoadLibcimbarSymbols failed: %v", err)
	}

	gridSize := 80
	scale := 1
	blockSize := PayloadCapacityBytes(gridSize)
	fountainEnc, err := fountain.NewEncoder(deterministicBytes(4*1024*1024), blockSize)
	if err != nil {
		b.Fatalf("NewEncoder failed: %v", err)
	}
	source := &screenFrameSource{
		codecEnc:    codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize),
		fountainEnc: fountainEnc,
		fileSize:    fountainEnc.FileSize(),
		width:       gridSize * CellSize(scale),
		height:      gridSize * CellSize(scale),
		frameID:     uint32(fountainEnc.BlockCount()),
	}

	var dst []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		dst, err = source.NextBGRA(dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}
