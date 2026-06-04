package symbol

import (
	"fmt"
	"image"
)

// SymbolID 符号标识符 (0-15)
type SymbolID uint8

const (
	// NumSymbols 符号总数（16 个符号编码 4 bits）
	NumSymbols = 16
)

// Recognizer 符号识别器
type Recognizer struct {
	// 16 个参考符号的 image hash
	hashes [NumSymbols]uint64
	// 16 个参考符号的 8x8 模板（用于绘制）
	templates [NumSymbols]*image.Gray
}

// NewRecognizer 创建新的符号识别器
func NewRecognizer() *Recognizer {
	return &Recognizer{}
}

// LoadSymbol 加载单个符号
// id: 符号 ID (0-15)
// img: 符号图像（会被缩放到 8x8）
func (r *Recognizer) LoadSymbol(id SymbolID, img image.Image) error {
	if id >= NumSymbols {
		return fmt.Errorf("invalid symbol ID: %d (must be 0-15)", id)
	}

	// 缩放到 8x8
	tile := ResizeToTile(img)

	// 计算 image hash
	hash := ImageHash(tile)

	// 保存
	r.hashes[id] = hash
	r.templates[id] = tile

	return nil
}

// Recognize 识别符号
// cellImg: 待识别的 cell 图像
// 返回：最匹配的符号 ID 和汉明距离
func (r *Recognizer) Recognize(cellImg image.Image) (SymbolID, int) {
	// 缩放到 8x8
	tile := ResizeToTile(cellImg)

	// 计算 image hash
	hash := ImageHash(tile)

	// 与 16 个参考符号比较汉明距离
	minDist := 65 // 最大汉明距离是 64
	bestID := SymbolID(0)

	for id := SymbolID(0); id < NumSymbols; id++ {
		dist := HammingDistance(hash, r.hashes[id])
		if dist < minDist {
			minDist = dist
			bestID = id
		}
	}

	return bestID, minDist
}

// GetTemplate 获取符号的 8x8 模板
func (r *Recognizer) GetTemplate(id SymbolID) (*image.Gray, error) {
	if id >= NumSymbols {
		return nil, fmt.Errorf("invalid symbol ID: %d", id)
	}

	if r.templates[id] == nil {
		return nil, fmt.Errorf("symbol %d not loaded", id)
	}

	return r.templates[id], nil
}

// GetHash 获取符号的 image hash
func (r *Recognizer) GetHash(id SymbolID) (uint64, error) {
	if id >= NumSymbols {
		return 0, fmt.Errorf("invalid symbol ID: %d", id)
	}

	return r.hashes[id], nil
}

// VerifyHammingDistances 验证所有符号对的汉明距离
// 返回最小汉明距离
func (r *Recognizer) VerifyHammingDistances() (minDist int, pairs [][2]SymbolID) {
	minDist = 65
	pairs = make([][2]SymbolID, 0)

	for i := SymbolID(0); i < NumSymbols; i++ {
		for j := i + 1; j < NumSymbols; j++ {
			dist := HammingDistance(r.hashes[i], r.hashes[j])

			if dist < minDist {
				minDist = dist
				pairs = [][2]SymbolID{{i, j}}
			} else if dist == minDist {
				pairs = append(pairs, [2]SymbolID{i, j})
			}
		}
	}

	return minDist, pairs
}

// IsLoaded 检查所有符号是否已加载
func (r *Recognizer) IsLoaded() bool {
	for id := SymbolID(0); id < NumSymbols; id++ {
		if r.templates[id] == nil {
			return false
		}
	}
	return true
}

// Stats 返回识别器统计信息
type Stats struct {
	LoadedSymbols   int
	MinHammingDist  int
	AvgHammingDist  float64
	ClosestPairs    [][2]SymbolID
}

// GetStats 获取识别器统计信息
func (r *Recognizer) GetStats() Stats {
	stats := Stats{}

	// 计算已加载的符号数量
	for id := SymbolID(0); id < NumSymbols; id++ {
		if r.templates[id] != nil {
			stats.LoadedSymbols++
		}
	}

	// 计算汉明距离统计
	if stats.LoadedSymbols >= 2 {
		totalDist := 0
		count := 0
		minDist := 65

		for i := SymbolID(0); i < NumSymbols; i++ {
			if r.templates[i] == nil {
				continue
			}
			for j := i + 1; j < NumSymbols; j++ {
				if r.templates[j] == nil {
					continue
				}

				dist := HammingDistance(r.hashes[i], r.hashes[j])
				totalDist += dist
				count++

				if dist < minDist {
					minDist = dist
					stats.ClosestPairs = [][2]SymbolID{{i, j}}
				} else if dist == minDist {
					stats.ClosestPairs = append(stats.ClosestPairs, [2]SymbolID{i, j})
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
