package symbol

import (
	"fmt"
	"image"
)

// SymbolID 符号标识符 (0-15)
type SymbolID uint8

const (
	// NumSymbols 符号总数（16 个符号编码 4 bits）
	NumSymbols        = 16
	DefaultShapeBits  = 4
	DefaultTileWidth  = 8
	DefaultTileHeight = 8
)

type Spec struct {
	Width     int
	Height    int
	ShapeBits int
}

func DefaultSpec() Spec {
	return Spec{
		Width:     DefaultTileWidth,
		Height:    DefaultTileHeight,
		ShapeBits: DefaultShapeBits,
	}
}

func NewSpec(width int, height int, shapeBits int) (Spec, error) {
	spec := Spec{
		Width:     width,
		Height:    height,
		ShapeBits: shapeBits,
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func (s Spec) Validate() error {
	if s.Width <= 0 || s.Height <= 0 {
		return fmt.Errorf("tile size must be > 0, got %dx%d", s.Width, s.Height)
	}
	if s.Width*s.Height > 64 {
		return fmt.Errorf("tile size %dx%d has %d bits, max is 64", s.Width, s.Height, s.Width*s.Height)
	}
	if s.ShapeBits <= 0 || s.ShapeBits > 8 {
		return fmt.Errorf("shape bits must be 1..8, got %d", s.ShapeBits)
	}
	return nil
}

func (s Spec) SymbolCount() int {
	if s.ShapeBits <= 0 {
		return 0
	}
	return 1 << s.ShapeBits
}

func (s Spec) TileBits() int {
	return s.Width * s.Height
}

// Recognizer 符号识别器
type Recognizer struct {
	spec      Spec
	hashes    []uint64
	templates []*image.Gray
}

// NewRecognizer 创建新的符号识别器
func NewRecognizer() *Recognizer {
	return NewRecognizerWithSpec(DefaultSpec())
}

func NewRecognizerWithSpec(spec Spec) *Recognizer {
	if err := spec.Validate(); err != nil {
		panic(err)
	}
	return &Recognizer{
		spec:      spec,
		hashes:    make([]uint64, spec.SymbolCount()),
		templates: make([]*image.Gray, spec.SymbolCount()),
	}
}

func (r *Recognizer) Spec() Spec {
	return r.spec
}

func (r *Recognizer) SymbolCount() int {
	return len(r.hashes)
}

// LoadSymbol 加载单个符号
// id: 符号 ID (0-15)
// img: 符号图像（会被缩放到 8x8）
func (r *Recognizer) LoadSymbol(id SymbolID, img image.Image) error {
	if int(id) >= r.SymbolCount() {
		return fmt.Errorf("invalid symbol ID: %d (must be 0-%d)", id, r.SymbolCount()-1)
	}

	tile := ResizeToTileWithSize(img, r.spec.Width, r.spec.Height)

	hash := ImageHashWithSize(tile, r.spec.Width, r.spec.Height)

	// 保存
	r.hashes[id] = hash
	r.templates[id] = tile

	return nil
}

// Recognize 识别符号
// cellImg: 待识别的 cell 图像
// 返回：最匹配的符号 ID 和汉明距离
func (r *Recognizer) Recognize(cellImg image.Image) (SymbolID, int) {
	tile := ResizeToTileWithSize(cellImg, r.spec.Width, r.spec.Height)

	hash := ImageHashWithSize(tile, r.spec.Width, r.spec.Height)

	return r.RecognizeHash(hash)
}

// RecognizeHash 根据已计算好的 8x8 image hash 识别符号。
func (r *Recognizer) RecognizeHash(hash uint64) (SymbolID, int) {
	minDist := r.spec.TileBits() + 1
	bestID := SymbolID(0)

	for id := 0; id < r.SymbolCount(); id++ {
		dist := HammingDistance(hash, r.hashes[id])
		if dist < minDist {
			minDist = dist
			bestID = SymbolID(id)
		}
	}

	return bestID, minDist
}

// GetTemplate 获取符号的 8x8 模板
func (r *Recognizer) GetTemplate(id SymbolID) (*image.Gray, error) {
	if int(id) >= r.SymbolCount() {
		return nil, fmt.Errorf("invalid symbol ID: %d", id)
	}

	if r.templates[id] == nil {
		return nil, fmt.Errorf("symbol %d not loaded", id)
	}

	return r.templates[id], nil
}

// GetHash 获取符号的 image hash
func (r *Recognizer) GetHash(id SymbolID) (uint64, error) {
	if int(id) >= r.SymbolCount() {
		return 0, fmt.Errorf("invalid symbol ID: %d", id)
	}

	return r.hashes[id], nil
}

// VerifyHammingDistances 验证所有符号对的汉明距离
// 返回最小汉明距离
func (r *Recognizer) VerifyHammingDistances() (minDist int, pairs [][2]SymbolID) {
	minDist = r.spec.TileBits() + 1
	pairs = make([][2]SymbolID, 0)

	for i := 0; i < r.SymbolCount(); i++ {
		for j := i + 1; j < r.SymbolCount(); j++ {
			dist := HammingDistance(r.hashes[i], r.hashes[j])

			if dist < minDist {
				minDist = dist
				pairs = [][2]SymbolID{{SymbolID(i), SymbolID(j)}}
			} else if dist == minDist {
				pairs = append(pairs, [2]SymbolID{SymbolID(i), SymbolID(j)})
			}
		}
	}

	return minDist, pairs
}

// IsLoaded 检查所有符号是否已加载
func (r *Recognizer) IsLoaded() bool {
	for id := 0; id < r.SymbolCount(); id++ {
		if r.templates[id] == nil {
			return false
		}
	}
	return true
}

// Stats 返回识别器统计信息
type Stats struct {
	LoadedSymbols  int
	MinHammingDist int
	AvgHammingDist float64
	ClosestPairs   [][2]SymbolID
}

// GetStats 获取识别器统计信息
func (r *Recognizer) GetStats() Stats {
	stats := Stats{}

	// 计算已加载的符号数量
	for id := 0; id < r.SymbolCount(); id++ {
		if r.templates[id] != nil {
			stats.LoadedSymbols++
		}
	}

	// 计算汉明距离统计
	if stats.LoadedSymbols >= 2 {
		totalDist := 0
		count := 0
		minDist := r.spec.TileBits() + 1

		for i := 0; i < r.SymbolCount(); i++ {
			if r.templates[i] == nil {
				continue
			}
			for j := i + 1; j < r.SymbolCount(); j++ {
				if r.templates[j] == nil {
					continue
				}

				dist := HammingDistance(r.hashes[i], r.hashes[j])
				totalDist += dist
				count++

				if dist < minDist {
					minDist = dist
					stats.ClosestPairs = [][2]SymbolID{{SymbolID(i), SymbolID(j)}}
				} else if dist == minDist {
					stats.ClosestPairs = append(stats.ClosestPairs, [2]SymbolID{SymbolID(i), SymbolID(j)})
				}
			}
		}

		stats.MinHammingDist = minDist
		if count > 0 {
			stats.AvgHammingDist = float64(totalDist) / float64(count)
		}
	}

	return stats
}
