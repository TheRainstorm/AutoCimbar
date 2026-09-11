package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/autocambar/autocambar/pkg/symbol"
)

type staticScaleCapture struct{ frame *capturedScreenFrame }

func (s *staticScaleCapture) CaptureFrame([]byte) (*capturedScreenFrame, error) { return s.frame, nil }

func scaleTestFixture(t testing.TB, tile, shape, colors, grid, b, packets int) (*autoScaleCapturer, *image.RGBA, []byte) {
	t.Helper()
	spec, err := symbol.NewSpec(tile, tile, shape)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ScreenDecodeConfig{Tile: fmt.Sprintf("%dx%d", tile, tile), ShapeBits: shape, ColorBits: colors, GridSize: grid, Scale: b, ECCPercent: 3, PacketsPerFrame: packets}
	capacity := GridCapacityBytesWithSpec(grid, spec, colors)
	block, err := PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(capacity, 3, packets)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newAutoScaleCapturer(nil, cfg, spec, grid*tile*b, capacity, block, nil)
	if err != nil {
		t.Fatal(err)
	}
	cr, err := colorRecognizerForBits(colors)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := codec.NewEncoderWithColorBits(a.symbols, cr, tile*b, grid, colors)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(42))
	var data []byte
	for i := 0; i < packets; i++ {
		blockData := make([]byte, block)
		rng.Read(blockData)
		packet, err := a.packetCodec.EncodeInto(BuildPacket(block*20, uint32(i), blockData), nil)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, packet...)
	}
	img, err := enc.Encode(data)
	if err != nil {
		t.Fatal(err)
	}
	return a, img, data
}

// Emulate the remote client's bilinear scaling independently of the decoder's
// tile-centre sampler, including a textured desktop outside the transfer frame.
func remoteScaledDesktop(src *image.RGBA, side, width, height int, bgra bool) *capturedScreenFrame {
	desktop := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			desktop.SetRGBA(x, y, color.RGBA{uint8(60 + x%41), uint8(80 + y%31), 100, 255})
		}
	}
	left, top := (width-side)/2, (height-side)/2
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			sx := (float64(x)+0.5)*float64(src.Rect.Dx())/float64(side) - 0.5
			sy := (float64(y)+0.5)*float64(src.Rect.Dy())/float64(side) - 0.5
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			var channels [3]float64
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					p := src.RGBAAt(max(0, min(x0+dx, src.Rect.Dx()-1)), max(0, min(y0+dy, src.Rect.Dy()-1)))
					wx, wy := 1-fx, 1-fy
					if dx == 1 {
						wx = fx
					}
					if dy == 1 {
						wy = fy
					}
					for c, v := range []uint8{p.R, p.G, p.B} {
						channels[c] += float64(v) * wx * wy
					}
				}
			}
			desktop.SetRGBA(left+x, top+y, color.RGBA{uint8(channels[0] + 0.5), uint8(channels[1] + 0.5), uint8(channels[2] + 0.5), 255})
		}
	}
	frame := &capturedScreenFrame{Pix: desktop.Pix, Img: desktop, Width: width, Height: height, Stride: desktop.Stride, BGRA: bgra}
	if bgra {
		for i := 0; i < len(frame.Pix); i += 4 {
			frame.Pix[i], frame.Pix[i+2] = frame.Pix[i+2], frame.Pix[i]
		}
		frame.Img = nil
	}
	return frame
}

func TestAutoScaleCenteredTransfer(t *testing.T) {
	for _, tc := range []struct {
		name                                                       string
		tile, shape, colors, grid, b, packets, side, width, height int
		bgra                                                       bool
	}{
		{"1080p_to_4k", 8, 4, 2, 20, 1, 1, 320, 3840, 2160, true},
		{"RQ120_4k", 8, 4, 2, 120, 1, 1, 1920, 3840, 2160, true},
		{"125_percent", 8, 4, 2, 20, 1, 1, 200, 800, 600, false},
		{"150_percent", 8, 4, 2, 20, 1, 3, 240, 801, 601, true},
		{"downscale_B2", 8, 4, 2, 20, 2, 1, 240, 800, 600, true},
		{"six_pixel_tile", 6, 4, 2, 20, 1, 1, 180, 800, 600, true},
		{"four_pixel_tile", 4, 3, 2, 40, 1, 1, 320, 800, 600, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, src, want := scaleTestFixture(t, tc.tile, tc.shape, tc.colors, tc.grid, tc.b, tc.packets)
			a.source = &staticScaleCapture{remoteScaledDesktop(src, tc.side, tc.width, tc.height, tc.bgra)}
			got, err := a.CaptureFrame(nil)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("did not lock centered scaled frame")
			}
			decoded, err := decodeCapturedFrame(a.decoder, got, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < tc.packets; i++ {
				n := a.packetCodec.EncodedSize()
				actual, err := a.packetCodec.DecodeInto(decoded[i*n:(i+1)*n], nil)
				if err != nil {
					t.Fatal(err)
				}
				expected, err := a.packetCodec.DecodeInto(want[i*n:(i+1)*n], nil)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(actual, expected) {
					t.Fatal("packet changed after scaled capture")
				}
			}
		})
	}
}

func TestAutoScaleSourceRecovery(t *testing.T) {
	a, _, _ := scaleTestFixture(t, 8, 4, 2, 20, 1, 1)
	input := make([]byte, a.blockSize*3)
	rand.New(rand.NewSource(7)).Read(input)
	source := BuildSourceDataWithCompressionAndName(input, "scaled.bin", true)
	enc, err := fountain.NewEncoder(source, a.blockSize)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := fountain.NewDecoder(len(source), a.blockSize, blockCountForFile(len(source), a.blockSize))
	if err != nil {
		t.Fatal(err)
	}
	colors, _ := colorRecognizerForBits(2)
	imageEncoder := codec.NewEncoder(a.symbols, colors, 8, a.grid)
	capture := &staticScaleCapture{}
	a.source = capture
	for id := uint32(0); !dec.Complete() && id < 20; id++ {
		block := enc.EncodeInto(id, make([]byte, a.blockSize))
		packet, err := a.packetCodec.EncodeInto(BuildPacket(len(source), id, block.Data), nil)
		if err != nil {
			t.Fatal(err)
		}
		img, err := imageEncoder.Encode(packet)
		if err != nil {
			t.Fatal(err)
		}
		capture.frame = remoteScaledDesktop(img, 240, 800, 600, true)
		frame, err := a.CaptureFrame(nil)
		if err != nil || frame == nil {
			t.Fatal("scaled frame unavailable", err)
		}
		payload, err := decodeCapturedFrame(a.decoder, frame, nil)
		if err != nil {
			t.Fatal(err)
		}
		packet, err = a.packetCodec.DecodeInto(payload[:a.packetCodec.EncodedSize()], nil)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParsePacket(packet, a.blockSize)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = dec.AddFrame(parsed.FrameID, parsed.Payload); err != nil {
			t.Fatal(err)
		}
	}
	recovered, err := dec.Decode()
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseSourceData(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Payload, input) || result.FileName != "scaled.bin" || result.MD5 != BytesMD5Hex(input) {
		t.Fatal("source metadata or data mismatch")
	}
}

func BenchmarkAutoScaleLocked(b *testing.B) {
	a, src, _ := scaleTestFixture(b, 8, 4, 2, 120, 1, 1)
	a.source = &staticScaleCapture{remoteScaledDesktop(src, 1920, 3840, 2160, true)}
	frame, err := a.CaptureFrame(nil)
	if err != nil || frame == nil {
		b.Fatal("no lock", err)
	}
	dst := frame.Pix
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame, err = a.CaptureFrame(dst)
		if err != nil || frame == nil {
			b.Fatal("lost lock", err)
		}
		dst = frame.Pix
	}
}

func TestAutoScaleRejectAndReacquire(t *testing.T) {
	a, src, _ := scaleTestFixture(t, 8, 4, 2, 20, 1, 1)
	blank := image.NewRGBA(image.Rect(0, 0, 800, 600))
	draw.Draw(blank, blank.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	source := &staticScaleCapture{&capturedScreenFrame{Img: blank, Width: 800, Height: 600, Stride: blank.Stride}}
	a.source = source
	if f, err := a.CaptureFrame(nil); err != nil || f != nil {
		t.Fatal("locked blank desktop", err)
	}
	source.frame = remoteScaledDesktop(src, 320, 800, 600, true)
	a.nextSearch = time.Time{}
	if f, _ := a.CaptureFrame(nil); f == nil {
		t.Fatal("failed initial lock")
	}
	source.frame = remoteScaledDesktop(src, 240, 800, 600, true)
	for i := 0; i < 3; i++ {
		a.nextCheck = time.Time{}
		a.CaptureFrame(nil)
	}
	if !a.region.Empty() {
		t.Fatal("did not release old scale")
	}
	a.nextSearch = time.Time{}
	if f, _ := a.CaptureFrame(nil); f == nil {
		t.Fatal("failed to reacquire new scale")
	}
	// A plausible symbol texture with a mismatched grid must never lock.
	a.grid = 21
	a.region = image.Rectangle{}
	a.nextSearch = time.Time{}
	if f, _ := a.CaptureFrame(nil); f != nil {
		t.Fatal("locked wrong grid")
	}
}

func TestAutoScaleRequiresPacketCRC(t *testing.T) {
	a, _, _ := scaleTestFixture(t, 8, 4, 2, 20, 1, 1)
	payload := make([]byte, a.blockSize)
	rand.New(rand.NewSource(9)).Read(payload)
	packet := BuildPacket(12345, 0, payload)
	packet[len(packet)-1] ^= 1
	encoded, err := a.packetCodec.EncodeInto(packet, nil)
	if err != nil {
		t.Fatal(err)
	}
	colors, _ := colorRecognizerForBits(2)
	encoder := codec.NewEncoder(a.symbols, colors, 8, a.grid)
	img, err := encoder.Encode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	a.source = &staticScaleCapture{remoteScaledDesktop(img, 320, 800, 600, true)}
	if frame, err := a.CaptureFrame(nil); err != nil || frame != nil {
		t.Fatal("locked symbol frame with bad CRC", err)
	}
}
