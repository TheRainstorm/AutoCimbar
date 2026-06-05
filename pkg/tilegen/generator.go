package tilegen

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/autocambar/autocambar/pkg/symbol"
)

type Options struct {
	Spec           symbol.Spec
	Seed           uint64
	Attempts       int
	TargetDistance int
}

type Result struct {
	Spec        symbol.Spec `json:"spec"`
	Seed        uint64      `json:"seed"`
	Attempts    int         `json:"attempts"`
	MinDistance int         `json:"min_distance"`
	AvgDistance float64     `json:"avg_distance"`
	Hashes      []uint64    `json:"hashes"`
}

func Generate(opts Options) (*Result, error) {
	if err := opts.Spec.Validate(); err != nil {
		return nil, err
	}
	if opts.Attempts <= 0 {
		opts.Attempts = defaultAttempts(opts.Spec)
	}
	if opts.TargetDistance <= 0 {
		opts.TargetDistance = defaultTargetDistance(opts.Spec)
	}
	if opts.Seed == 0 {
		opts.Seed = 1
	}

	rng := rand.New(rand.NewPCG(opts.Seed, opts.Seed^0x9e3779b97f4a7c15))
	hashes := make([]uint64, 0, opts.Spec.SymbolCount())
	seen := make(map[uint64]struct{}, opts.Spec.SymbolCount())
	area := opts.Spec.TileBits()
	wantOnes := area / 2
	if wantOnes == 0 {
		wantOnes = 1
	}

	for len(hashes) < opts.Spec.SymbolCount() {
		var best uint64
		bestDist := -1
		for attempt := 0; attempt < opts.Attempts; attempt++ {
			candidate := randomBalancedMask(rng, area, wantOnes)
			if attempt%3 == 0 {
				candidate = randomBlockyMask(rng, opts.Spec.Width, opts.Spec.Height, wantOnes)
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			if candidate == 0 || bits.OnesCount64(candidate) == area {
				continue
			}
			dist := minDistance(candidate, hashes)
			if dist > bestDist || (dist == bestDist && balanceScore(candidate, area) > balanceScore(best, area)) {
				best = candidate
				bestDist = dist
				if bestDist >= opts.TargetDistance {
					break
				}
			}
		}
		if bestDist < 0 {
			return nil, fmt.Errorf("failed to find tile %d after %d attempts", len(hashes), opts.Attempts)
		}
		hashes = append(hashes, best)
		seen[best] = struct{}{}
	}

	minDist, avgDist := distances(hashes)
	return &Result{
		Spec:        opts.Spec,
		Seed:        opts.Seed,
		Attempts:    opts.Attempts,
		MinDistance: minDist,
		AvgDistance: avgDist,
		Hashes:      hashes,
	}, nil
}

func Save(result *Result, dir string) error {
	if result == nil {
		return fmt.Errorf("nil result")
	}
	if err := result.Spec.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create tile dir: %w", err)
	}
	for id, hash := range result.Hashes {
		path := filepath.Join(dir, fmt.Sprintf("%02x.png", id))
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if err := png.Encode(f, HashImage(hash, result.Spec)); err != nil {
			_ = f.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close %s: %w", path, err)
		}
	}

	manifest, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(manifest, '\n'), 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	text := fmt.Sprintf("tile=%dx%d\nshape_bits=%d\nsymbols=%d\nseed=%d\nattempts=%d\nmin_distance=%d\navg_distance=%.2f\n",
		result.Spec.Width, result.Spec.Height, result.Spec.ShapeBits, result.Spec.SymbolCount(), result.Seed, result.Attempts, result.MinDistance, result.AvgDistance)
	if err := os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(text), 0644); err != nil {
		return fmt.Errorf("write manifest text: %w", err)
	}
	return nil
}

func HashImage(hash uint64, spec symbol.Spec) image.Image {
	img := image.NewGray(image.Rect(0, 0, spec.Width, spec.Height))
	for y := 0; y < spec.Height; y++ {
		for x := 0; x < spec.Width; x++ {
			index := y*spec.Width + x
			shift := uint(spec.TileBits() - 1 - index)
			if (hash>>shift)&1 == 1 {
				img.SetGray(x, y, color.Gray{Y: 255})
			} else {
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return img
}

func defaultAttempts(spec symbol.Spec) int {
	switch {
	case spec.TileBits() <= 16:
		return 2000
	case spec.TileBits() <= 36:
		return 8000
	default:
		return 20000
	}
}

func defaultTargetDistance(spec symbol.Spec) int {
	area := spec.TileBits()
	switch area {
	case 64:
		if spec.ShapeBits >= 6 {
			return 20
		}
		if spec.ShapeBits == 5 {
			return 22
		}
		return 24
	case 36:
		return 11
	case 16:
		if spec.ShapeBits <= 2 {
			return 8
		}
		if spec.ShapeBits == 3 {
			return 6
		}
		return 4
	default:
		return area / 3
	}
}

func randomBalancedMask(rng *rand.Rand, area int, ones int) uint64 {
	positions := make([]int, area)
	for i := range positions {
		positions[i] = i
	}
	rng.Shuffle(len(positions), func(i, j int) {
		positions[i], positions[j] = positions[j], positions[i]
	})
	var mask uint64
	for _, pos := range positions[:ones] {
		shift := uint(area - 1 - pos)
		mask |= 1 << shift
	}
	return mask
}

func randomBlockyMask(rng *rand.Rand, width int, height int, ones int) uint64 {
	area := width * height
	var mask uint64
	for bits.OnesCount64(mask) < ones {
		x := rng.IntN(width)
		y := rng.IntN(height)
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				xx := x + dx
				yy := y + dy
				if xx >= width || yy >= height {
					continue
				}
				pos := yy*width + xx
				shift := uint(area - 1 - pos)
				mask |= 1 << shift
			}
		}
	}
	for bits.OnesCount64(mask) > ones {
		pos := rng.IntN(area)
		shift := uint(area - 1 - pos)
		mask &^= 1 << shift
	}
	return mask
}

func minDistance(hash uint64, hashes []uint64) int {
	if len(hashes) == 0 {
		return 1 << 30
	}
	minDist := 1 << 30
	for _, existing := range hashes {
		dist := bits.OnesCount64(hash ^ existing)
		if dist < minDist {
			minDist = dist
		}
	}
	return minDist
}

func balanceScore(hash uint64, area int) int {
	ones := bits.OnesCount64(hash)
	diff := ones - area/2
	if diff < 0 {
		diff = -diff
	}
	return -diff
}

func distances(hashes []uint64) (int, float64) {
	if len(hashes) < 2 {
		return 0, 0
	}
	minDist := 1 << 30
	total := 0
	count := 0
	for i := 0; i < len(hashes); i++ {
		for j := i + 1; j < len(hashes); j++ {
			dist := bits.OnesCount64(hashes[i] ^ hashes[j])
			if dist < minDist {
				minDist = dist
			}
			total += dist
			count++
		}
	}
	return minDist, float64(total) / float64(count)
}
