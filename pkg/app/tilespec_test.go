package app

import "testing"

func TestResolveGridSizeReferenceQ(t *testing.T) {
	tests := []struct {
		name string
		tile string
		rq   int
		want int
	}{
		{"8x8", "8x8", 120, 120},
		{"4x4", "4x4", 120, 240},
		{"6x6", "6x6", 120, 160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseTileSpec(tt.tile, 4)
			if err != nil {
				t.Fatalf("ParseTileSpec: %v", err)
			}
			got, err := ResolveGridSize(1, tt.rq, spec)
			if err != nil {
				t.Fatalf("ResolveGridSize: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveGridSize = %d, want %d", got, tt.want)
			}
		})
	}
}
