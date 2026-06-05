package app

import (
	"fmt"
	"image"
	"image/color"

	"github.com/liyue201/goqr"
	qrcode "github.com/skip2/go-qrcode"
)

const qrRecoveryLevel = qrcode.Low
const qrQuietZoneModules = 4

type qrFrameCodec struct {
	version  int
	modules  int
	size     int
	capacity int
}

func newQRFrameCodec(q int, scale int) (*qrFrameCodec, error) {
	if q <= 0 {
		return nil, fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("B must be > 0")
	}
	version := qrVersionForQ(q)
	modules := qrModulesForVersion(version)
	size := (modules + qrQuietZoneModules*2) * scale
	capacity, err := qrPayloadCapacity(version)
	if err != nil {
		return nil, err
	}
	return &qrFrameCodec{
		version:  version,
		modules:  modules,
		size:     size,
		capacity: capacity,
	}, nil
}

func qrVersionForQ(q int) int {
	version := (q - 17 + 2) / 4
	if version < 1 {
		version = 1
	}
	if version > 40 {
		version = 40
	}
	return version
}

func qrModulesForVersion(version int) int {
	return 17 + version*4
}

func qrPayloadCapacity(version int) (int, error) {
	lo, hi := 1, 4096
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if qrCanEncode(version, mid) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo <= 0 {
		return 0, fmt.Errorf("QR version %d has no payload capacity", version)
	}
	return lo, nil
}

func qrCanEncode(version int, n int) bool {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(0x80 + i%0x40)
	}
	_, err := qrcode.NewWithForcedVersion(string(data), version, qrRecoveryLevel)
	return err == nil
}

func (q *qrFrameCodec) Encode(data []byte) (*image.RGBA, error) {
	img, err := q.encodeImage(data)
	if err != nil {
		return nil, err
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	out := image.NewRGBA(img.Bounds())
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, nil
}

func (q *qrFrameCodec) EncodeBGRA(data []byte, dst []byte) ([]byte, error) {
	img, err := q.encodeImage(data)
	if err != nil {
		return nil, err
	}
	need := q.size * q.size * 4
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	b := img.Bounds()
	for y := 0; y < q.size; y++ {
		for x := 0; x < q.size; x++ {
			r, g, b0, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			off := (y*q.size + x) * 4
			dst[off+0] = byte(b0 >> 8)
			dst[off+1] = byte(g >> 8)
			dst[off+2] = byte(r >> 8)
			dst[off+3] = byte(a >> 8)
		}
	}
	return dst, nil
}

func (q *qrFrameCodec) encodeImage(data []byte) (image.Image, error) {
	if len(data) > q.capacity {
		return nil, fmt.Errorf("QR payload too large: got %d bytes, capacity %d", len(data), q.capacity)
	}
	code, err := qrcode.NewWithForcedVersion(string(data), q.version, qrRecoveryLevel)
	if err != nil {
		return nil, err
	}
	code.DisableBorder = false
	code.ForegroundColor = color.Black
	code.BackgroundColor = color.White
	return code.Image(q.size), nil
}

func (q *qrFrameCodec) DecodeInto(img image.Image, dst []byte) ([]byte, error) {
	codes, err := goqr.Recognize(img)
	if err != nil {
		return dst, err
	}
	if len(codes) == 0 {
		return dst, goqr.ErrNoQRCode
	}
	payload := codes[0].Payload
	if cap(dst) < len(payload) {
		dst = make([]byte, len(payload))
	} else {
		dst = dst[:len(payload)]
	}
	copy(dst, payload)
	return dst, nil
}

func (q *qrFrameCodec) DecodeBGRAInto(pix []byte, width int, height int, stride int, dst []byte) ([]byte, error) {
	if width < q.size || height < q.size {
		return nil, fmt.Errorf("image too small: got %dx%d, need at least %dx%d", width, height, q.size, q.size)
	}
	if stride < width*4 {
		return nil, fmt.Errorf("invalid stride: got %d, need at least %d", stride, width*4)
	}
	if len(pix) < stride*height {
		return nil, fmt.Errorf("pixel buffer too short: got %d, need %d", len(pix), stride*height)
	}
	img := image.NewGray(image.Rect(0, 0, q.size, q.size))
	for y := 0; y < q.size; y++ {
		row := pix[y*stride:]
		for x := 0; x < q.size; x++ {
			off := x * 4
			b := uint32(row[off+0])
			g := uint32(row[off+1])
			r := uint32(row[off+2])
			img.Pix[y*img.Stride+x] = byte((77*r + 150*g + 29*b) >> 8)
		}
	}
	return q.DecodeInto(img, dst)
}
