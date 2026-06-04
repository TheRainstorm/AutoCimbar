package app

import (
	"image/png"
	"net/http/httptest"
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

func TestScreenFrameHandlerReturnsDecodableFrame(t *testing.T) {
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
	server := &screenFrameServer{
		codecEnc:    codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize),
		fountainEnc: fountainEnc,
		fileSize:    len([]byte("screen frame test")),
		width:       gridSize * CellSize(scale),
		height:      gridSize * CellSize(scale),
	}

	resp := httptest.NewRecorder()
	server.frameHandler(resp, httptest.NewRequest("GET", "/frame.png", nil))
	if resp.Code != 200 {
		t.Fatalf("frameHandler status = %d, body %q", resp.Code, resp.Body.String())
	}

	img, err := png.Decode(resp.Body)
	if err != nil {
		t.Fatalf("decode response png: %v", err)
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
