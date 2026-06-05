package app

import (
	"fmt"
	"image"
	"strings"
)

const (
	BackendSymbols = "symbols"
	BackendQR      = "qr"
)

func normalizeBackend(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return BackendSymbols, nil
	}
	switch name {
	case BackendSymbols, "symbol", "cimbar", "tiles":
		return BackendSymbols, nil
	case BackendQR, "qrcode":
		return BackendQR, nil
	default:
		return "", fmt.Errorf("unknown backend %q; use symbols or qr", name)
	}
}

type frameEncoder interface {
	Encode([]byte) (*image.RGBA, error)
	EncodeBGRA([]byte, []byte) ([]byte, error)
}

type frameDecoder interface {
	DecodeInto(image.Image, []byte) ([]byte, error)
	DecodeBGRAInto([]byte, int, int, int, []byte) ([]byte, error)
}
