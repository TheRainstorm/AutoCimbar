package fountain

import (
	"fmt"
	"math/rand"
)

const MaxDecoderBlockCount = 1 << 20

type Encoder struct {
	data       []byte
	blockSize  int
	blockCount int
	blocks     [][]byte
}

type EncodedBlock struct {
	FrameID uint32
	Data    []byte
}

func NewEncoder(data []byte, blockSize int) (*Encoder, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}

	blockCount := (len(data) + blockSize - 1) / blockSize
	if blockCount == 0 {
		blockCount = 1
	}

	blocks := make([][]byte, blockCount)
	for i := 0; i < blockCount; i++ {
		block := make([]byte, blockSize)
		start := i * blockSize
		end := start + blockSize
		if end > len(data) {
			end = len(data)
		}
		if start < len(data) {
			copy(block, data[start:end])
		}
		blocks[i] = block
	}

	return &Encoder{
		data:       data,
		blockSize:  blockSize,
		blockCount: blockCount,
		blocks:     blocks,
	}, nil
}

func (e *Encoder) FileSize() int {
	return len(e.data)
}

func (e *Encoder) BlockSize() int {
	return e.blockSize
}

func (e *Encoder) BlockCount() int {
	return e.blockCount
}

func (e *Encoder) Encode(frameID uint32) EncodedBlock {
	coeff := CoeffForFrame(e.blockCount, frameID)
	out := make([]byte, e.blockSize)

	if int(frameID) < e.blockCount {
		copy(out, e.blocks[frameID])
	} else {
		for i := 0; i < e.blockCount; i++ {
			if CoeffBit(coeff, i) {
				xorBytes(out, e.blocks[i])
			}
		}
	}

	return EncodedBlock{
		FrameID: frameID,
		Data:    out,
	}
}

func CoeffForFrame(blockCount int, frameID uint32) []byte {
	coeff := make([]byte, CoeffSize(blockCount))
	if int(frameID) < blockCount {
		SetCoeffBit(coeff, int(frameID))
		return coeff
	}

	rng := rand.New(rand.NewSource(int64(splitmix64(uint64(frameID)<<32 | uint64(blockCount)))))
	set := false
	for i := 0; i < blockCount; i++ {
		if rng.Intn(2) == 0 {
			SetCoeffBit(coeff, i)
			set = true
		}
	}
	if !set {
		SetCoeffBit(coeff, int(frameID)%blockCount)
	}
	return coeff
}

type Decoder struct {
	fileSize   int
	blockSize  int
	blockCount int
	coeffSize  int
	basis      []*row
	rank       int
}

type row struct {
	coeff []byte
	data  []byte
}

func NewDecoder(fileSize int, blockSize int, blockCount int) (*Decoder, error) {
	if fileSize < 0 {
		return nil, fmt.Errorf("file size must be >= 0")
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	if blockCount <= 0 {
		return nil, fmt.Errorf("block count must be > 0")
	}
	if blockCount > MaxDecoderBlockCount {
		return nil, fmt.Errorf("block count %d exceeds max %d", blockCount, MaxDecoderBlockCount)
	}
	minBlockCount := fileSize / blockSize
	if fileSize%blockSize != 0 {
		minBlockCount++
	}
	if minBlockCount == 0 {
		minBlockCount = 1
	}
	if minBlockCount > blockCount {
		return nil, fmt.Errorf("file size %d requires %d block(s), got %d", fileSize, minBlockCount, blockCount)
	}

	return &Decoder{
		fileSize:   fileSize,
		blockSize:  blockSize,
		blockCount: blockCount,
		coeffSize:  CoeffSize(blockCount),
		basis:      make([]*row, blockCount),
	}, nil
}

func (d *Decoder) AddFrame(frameID uint32, data []byte) (bool, error) {
	return d.Add(CoeffForFrame(d.blockCount, frameID), data)
}

func (d *Decoder) Add(coeff []byte, data []byte) (bool, error) {
	if len(coeff) != d.coeffSize {
		return false, fmt.Errorf("invalid coeff size: got %d, want %d", len(coeff), d.coeffSize)
	}
	if len(data) != d.blockSize {
		return false, fmt.Errorf("invalid block size: got %d, want %d", len(data), d.blockSize)
	}

	c := append([]byte(nil), coeff...)
	v := append([]byte(nil), data...)

	for {
		pivot := FirstCoeffBit(c, d.blockCount)
		if pivot < 0 {
			return false, nil
		}

		if d.basis[pivot] == nil {
			d.basis[pivot] = &row{coeff: c, data: v}
			d.rank++
			return true, nil
		}

		xorBytes(c, d.basis[pivot].coeff)
		xorBytes(v, d.basis[pivot].data)
	}
}

func (d *Decoder) Rank() int {
	return d.rank
}

func (d *Decoder) Complete() bool {
	return d.rank == d.blockCount
}

func (d *Decoder) Decode() ([]byte, error) {
	if !d.Complete() {
		return nil, fmt.Errorf("not enough independent blocks: rank %d of %d", d.rank, d.blockCount)
	}

	blocks := make([][]byte, d.blockCount)
	for i := d.blockCount - 1; i >= 0; i-- {
		if d.basis[i] == nil {
			return nil, fmt.Errorf("missing pivot %d", i)
		}

		block := append([]byte(nil), d.basis[i].data...)
		for j := i + 1; j < d.blockCount; j++ {
			if CoeffBit(d.basis[i].coeff, j) {
				xorBytes(block, blocks[j])
			}
		}
		blocks[i] = block
	}

	out := make([]byte, 0, d.blockCount*d.blockSize)
	for _, block := range blocks {
		out = append(out, block...)
	}
	if len(out) < d.fileSize {
		return nil, fmt.Errorf("decoded data too short: got %d, need %d", len(out), d.fileSize)
	}
	return out[:d.fileSize], nil
}

func CoeffSize(blockCount int) int {
	return (blockCount + 7) / 8
}

func CoeffBit(coeff []byte, i int) bool {
	return coeff[i/8]&(1<<uint(i%8)) != 0
}

func SetCoeffBit(coeff []byte, i int) {
	coeff[i/8] |= 1 << uint(i%8)
}

func FirstCoeffBit(coeff []byte, blockCount int) int {
	for i := 0; i < blockCount; i++ {
		if CoeffBit(coeff, i) {
			return i
		}
	}
	return -1
}

func xorBytes(dst []byte, src []byte) {
	for i := range dst {
		dst[i] ^= src[i]
	}
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
