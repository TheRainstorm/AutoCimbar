package ecc

import (
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// ECC Reed-Solomon 纠错码
type ECC struct {
	encoder reedsolomon.Encoder
	// dataShards 数据分片数量
	dataShards int
	// parityShards 纠错分片数量
	parityShards int
}

// NewECC 创建 ECC 实例
// eccPercent: 纠错百分比（例如 20 表示 20% 纠错）
// blockSize: 数据块大小（字节）
func NewECC(eccPercent int, blockSize int) (*ECC, error) {
	if eccPercent < 0 || eccPercent > 100 {
		return nil, fmt.Errorf("invalid ecc percent: %d (must be 0-100)", eccPercent)
	}

	if blockSize <= 0 {
		return nil, fmt.Errorf("invalid block size: %d (must be > 0)", blockSize)
	}

	// 计算数据块和纠错块数量
	// 使用固定的分片数量以简化实现
	// 例如：10 个数据分片 + 2 个纠错分片
	dataShards := 10
	parityShards := dataShards * eccPercent / 100

	if parityShards == 0 && eccPercent > 0 {
		parityShards = 1 // 至少 1 个纠错分片
	}

	if parityShards == 0 {
		// 0% ECC，不需要纠错
		return &ECC{
			encoder:      nil,
			dataShards:   1,
			parityShards: 0,
		}, nil
	}

	// 创建 Reed-Solomon 编码器
	encoder, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("failed to create reed-solomon encoder: %w", err)
	}

	return &ECC{
		encoder:      encoder,
		dataShards:   dataShards,
		parityShards: parityShards,
	}, nil
}

// Encode 编码数据（添加纠错码）
// data: 原始数据
// 返回：编码后的数据（包含纠错信息）
func (e *ECC) Encode(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// 如果没有纠错，直接返回
	if e.parityShards == 0 {
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	}

	totalShards := e.dataShards + e.parityShards

	// 计算每个分片的大小
	shardSize := (len(data) + e.dataShards - 1) / e.dataShards

	// 填充数据
	paddedSize := shardSize * e.dataShards
	padded := make([]byte, paddedSize)
	copy(padded, data)

	// 创建分片
	shards := make([][]byte, totalShards)
	for i := 0; i < e.dataShards; i++ {
		shards[i] = make([]byte, shardSize)
		copy(shards[i], padded[i*shardSize:(i+1)*shardSize])
	}

	// 创建纠错分片
	for i := e.dataShards; i < totalShards; i++ {
		shards[i] = make([]byte, shardSize)
	}

	// 计算纠错码
	err := e.encoder.Encode(shards)
	if err != nil {
		return nil, fmt.Errorf("failed to encode: %w", err)
	}

	// 组合所有分片
	encoded := make([]byte, shardSize*totalShards)
	for i, shard := range shards {
		copy(encoded[i*shardSize:(i+1)*shardSize], shard)
	}

	return encoded, nil
}

// Decode 解码数据（修复错误）
// data: 编码后的数据（可能有错误）
// originalSize: 原始数据大小（解码后需要截断到这个大小）
// 返回：解码后的数据（已修复错误）
func (e *ECC) Decode(data []byte, originalSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	// 如果没有纠错，直接返回
	if e.parityShards == 0 {
		if originalSize > len(data) {
			return nil, fmt.Errorf("original size %d exceeds data size %d", originalSize, len(data))
		}
		result := make([]byte, originalSize)
		copy(result, data[:originalSize])
		return result, nil
	}

	totalShards := e.dataShards + e.parityShards

	// 计算分片大小
	if len(data)%totalShards != 0 {
		return nil, fmt.Errorf("invalid data size: %d (not divisible by %d)", len(data), totalShards)
	}

	shardSize := len(data) / totalShards

	// 分割分片
	shards := make([][]byte, totalShards)
	for i := 0; i < totalShards; i++ {
		shards[i] = make([]byte, shardSize)
		copy(shards[i], data[i*shardSize:(i+1)*shardSize])
	}

	// 尝试重建
	err := e.encoder.Reconstruct(shards)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct: %w", err)
	}

	// 提取数据部分
	decoded := make([]byte, shardSize*e.dataShards)
	for i := 0; i < e.dataShards; i++ {
		copy(decoded[i*shardSize:(i+1)*shardSize], shards[i])
	}

	// 截断到原始大小
	if originalSize > len(decoded) {
		return nil, fmt.Errorf("original size %d exceeds decoded size %d", originalSize, len(decoded))
	}

	return decoded[:originalSize], nil
}

// DataSize 返回数据分片数量
func (e *ECC) DataSize() int {
	return e.dataShards
}

// ParitySize 返回纠错分片数量
func (e *ECC) ParitySize() int {
	return e.parityShards
}

// TotalSize 返回总分片数量
func (e *ECC) TotalSize() int {
	return e.dataShards + e.parityShards
}

// CalculateEncodedSize 计算编码后的总大小
func (e *ECC) CalculateEncodedSize(dataSize int) int {
	if e.parityShards == 0 {
		return dataSize
	}

	totalShards := e.dataShards + e.parityShards
	shardSize := (dataSize + e.dataShards - 1) / e.dataShards
	return shardSize * totalShards
}

// PacketCodec adds per-frame Reed-Solomon parity bytes. It is intended for
// correcting byte errors inside one decoded image frame, before the fountain
// decoder sees the packet.
type PacketCodec struct {
	percent     int
	dataSize    int
	encodedSize int
	words       []packetWord
}

type packetWord struct {
	dataBytes   int
	parityBytes int
	encodedSize int
}

// NewPacketCodec creates a fixed-size packet codec. Both sides must construct
// it with the same percent and dataSize; these values are not stored on the wire.
func NewPacketCodec(percent int, dataSize int) (*PacketCodec, error) {
	if percent < 0 || percent > 100 {
		return nil, fmt.Errorf("invalid ecc percent: %d (must be 0-100)", percent)
	}
	if dataSize <= 0 {
		return nil, fmt.Errorf("invalid packet data size: %d", dataSize)
	}
	words, encodedSize, err := buildPacketWords(percent, dataSize)
	if err != nil {
		return nil, err
	}
	return &PacketCodec{
		percent:     percent,
		dataSize:    dataSize,
		encodedSize: encodedSize,
		words:       words,
	}, nil
}

func PacketEncodedSize(percent int, dataSize int) (int, error) {
	_, encodedSize, err := buildPacketWords(percent, dataSize)
	return encodedSize, err
}

func MaxPacketDataSize(encodedCapacity int, percent int) (int, error) {
	if encodedCapacity <= 0 {
		return 0, fmt.Errorf("invalid encoded capacity: %d", encodedCapacity)
	}
	if percent < 0 || percent > 100 {
		return 0, fmt.Errorf("invalid ecc percent: %d (must be 0-100)", percent)
	}
	lo, hi := 1, encodedCapacity
	best := 0
	for lo <= hi {
		mid := lo + (hi-lo)/2
		size, err := PacketEncodedSize(percent, mid)
		if err != nil {
			return 0, err
		}
		if size <= encodedCapacity {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best <= 0 {
		return 0, fmt.Errorf("encoded capacity %d is too small for ecc percent %d", encodedCapacity, percent)
	}
	return best, nil
}

func (p *PacketCodec) Percent() int {
	return p.percent
}

func (p *PacketCodec) DataSize() int {
	return p.dataSize
}

func (p *PacketCodec) EncodedSize() int {
	return p.encodedSize
}

func (p *PacketCodec) ParitySize() int {
	return p.encodedSize - p.dataSize
}

func (p *PacketCodec) Enabled() bool {
	return p.percent > 0
}

func (p *PacketCodec) Encode(data []byte) ([]byte, error) {
	return p.EncodeInto(data, nil)
}

func (p *PacketCodec) EncodeInto(data []byte, dst []byte) ([]byte, error) {
	if len(data) != p.dataSize {
		return nil, fmt.Errorf("packet data size mismatch: got %d, want %d", len(data), p.dataSize)
	}
	if p.percent == 0 {
		if cap(dst) < len(data) {
			dst = make([]byte, len(data))
		} else {
			dst = dst[:len(data)]
		}
		copy(dst, data)
		return dst, nil
	}

	codewords := make([][]byte, len(p.words))
	dataOffset := 0
	for i, word := range p.words {
		chunk := data[dataOffset : dataOffset+word.dataBytes]
		codeword, err := rsEncodeCodeword(chunk, word.parityBytes)
		if err != nil {
			return nil, err
		}
		codewords[i] = codeword
		dataOffset += word.dataBytes
	}

	if cap(dst) < p.encodedSize {
		dst = make([]byte, p.encodedSize)
	} else {
		dst = dst[:p.encodedSize]
	}
	interleaveCodewords(codewords, dst)
	return dst, nil
}

func (p *PacketCodec) Decode(data []byte) ([]byte, error) {
	return p.DecodeInto(data, nil)
}

func (p *PacketCodec) DecodeInto(data []byte, dst []byte) ([]byte, error) {
	if len(data) < p.encodedSize {
		return nil, fmt.Errorf("encoded packet too short: got %d, need %d", len(data), p.encodedSize)
	}
	if p.percent == 0 {
		if cap(dst) < p.dataSize {
			dst = make([]byte, p.dataSize)
		} else {
			dst = dst[:p.dataSize]
		}
		copy(dst, data[:p.dataSize])
		return dst, nil
	}

	codewords := make([][]byte, len(p.words))
	for i, word := range p.words {
		codewords[i] = make([]byte, word.encodedSize)
	}
	deinterleaveCodewords(data[:p.encodedSize], codewords)

	if cap(dst) < p.dataSize {
		dst = make([]byte, p.dataSize)
	} else {
		dst = dst[:p.dataSize]
	}
	dataOffset := 0
	for i, word := range p.words {
		decoded, err := rsDecodeCodeword(codewords[i], word.parityBytes)
		if err != nil {
			return nil, fmt.Errorf("ecc codeword %d: %w", i, err)
		}
		copy(dst[dataOffset:dataOffset+word.dataBytes], decoded[:word.dataBytes])
		dataOffset += word.dataBytes
	}
	return dst, nil
}

func buildPacketWords(percent int, dataSize int) ([]packetWord, int, error) {
	if percent < 0 || percent > 100 {
		return nil, 0, fmt.Errorf("invalid ecc percent: %d (must be 0-100)", percent)
	}
	if dataSize <= 0 {
		return nil, 0, fmt.Errorf("invalid packet data size: %d", dataSize)
	}
	if percent == 0 {
		return []packetWord{{dataBytes: dataSize, encodedSize: dataSize}}, dataSize, nil
	}

	maxData := maxCodewordDataBytes(percent)
	if maxData <= 0 {
		return nil, 0, fmt.Errorf("invalid max codeword data for ecc percent %d", percent)
	}
	wordCount := (dataSize + maxData - 1) / maxData
	base := dataSize / wordCount
	rem := dataSize % wordCount
	words := make([]packetWord, wordCount)
	encodedSize := 0
	for i := range words {
		dataBytes := base
		if i < rem {
			dataBytes++
		}
		parityBytes := parityBytesFor(dataBytes, percent)
		if dataBytes+parityBytes > 255 {
			return nil, 0, fmt.Errorf("ecc codeword too large: data=%d parity=%d", dataBytes, parityBytes)
		}
		words[i] = packetWord{
			dataBytes:   dataBytes,
			parityBytes: parityBytes,
			encodedSize: dataBytes + parityBytes,
		}
		encodedSize += words[i].encodedSize
	}
	return words, encodedSize, nil
}

func maxCodewordDataBytes(percent int) int {
	for dataBytes := 255; dataBytes >= 1; dataBytes-- {
		if dataBytes+parityBytesFor(dataBytes, percent) <= 255 {
			return dataBytes
		}
	}
	return 0
}

func parityBytesFor(dataBytes int, percent int) int {
	if percent <= 0 {
		return 0
	}
	parity := (dataBytes*percent + 99) / 100
	if parity < 1 {
		parity = 1
	}
	return parity
}

func interleaveCodewords(codewords [][]byte, dst []byte) {
	offset := 0
	for column := 0; offset < len(dst); column++ {
		for _, codeword := range codewords {
			if column < len(codeword) {
				dst[offset] = codeword[column]
				offset++
			}
		}
	}
}

func deinterleaveCodewords(src []byte, codewords [][]byte) {
	offset := 0
	for column := 0; offset < len(src); column++ {
		for _, codeword := range codewords {
			if column < len(codeword) {
				codeword[column] = src[offset]
				offset++
			}
		}
	}
}

var gfExp [512]byte
var gfLog [256]byte

func init() {
	x := uint16(1)
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[byte(x)] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < len(gfExp); i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a byte, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

func gfDiv(a byte, b byte) byte {
	if b == 0 {
		panic("gf divide by zero")
	}
	if a == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+255-int(gfLog[b])]
}

func gfPow2(power int) byte {
	power %= 255
	if power < 0 {
		power += 255
	}
	return gfExp[power]
}

func rsEncodeCodeword(data []byte, parityBytes int) ([]byte, error) {
	if parityBytes <= 0 {
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}
	if len(data)+parityBytes > 255 {
		return nil, fmt.Errorf("codeword too large: data=%d parity=%d", len(data), parityBytes)
	}
	gen := rsGenerator(parityBytes)
	out := make([]byte, len(data)+parityBytes)
	copy(out, data)
	for i := 0; i < len(data); i++ {
		coef := out[i]
		if coef == 0 {
			continue
		}
		for j := 1; j < len(gen); j++ {
			out[i+j] ^= gfMul(gen[j], coef)
		}
	}
	copy(out, data)
	return out, nil
}

func rsDecodeCodeword(codeword []byte, parityBytes int) ([]byte, error) {
	if parityBytes <= 0 {
		out := make([]byte, len(codeword))
		copy(out, codeword)
		return out, nil
	}
	if len(codeword) > 255 || len(codeword) <= parityBytes {
		return nil, fmt.Errorf("invalid codeword size: total=%d parity=%d", len(codeword), parityBytes)
	}
	msg := make([]byte, len(codeword))
	copy(msg, codeword)
	synd := rsSyndromes(msg, parityBytes)
	if syndromesZero(synd) {
		return msg[:len(msg)-parityBytes], nil
	}

	locator, errs, err := rsFindErrorLocator(synd, parityBytes)
	if err != nil {
		return nil, err
	}
	positions, err := rsFindErrorPositions(locator, errs, len(msg))
	if err != nil {
		return nil, err
	}
	magnitudes, err := rsSolveErrorMagnitudes(synd, positions, len(msg))
	if err != nil {
		return nil, err
	}
	for i, pos := range positions {
		msg[pos] ^= magnitudes[i]
	}
	if !syndromesZero(rsSyndromes(msg, parityBytes)) {
		return nil, fmt.Errorf("too many errors for %d parity bytes", parityBytes)
	}
	return msg[:len(msg)-parityBytes], nil
}

func rsGenerator(parityBytes int) []byte {
	gen := []byte{1}
	for i := 0; i < parityBytes; i++ {
		next := []byte{1, gfPow2(i)}
		gen = polyMulDesc(gen, next)
	}
	return gen
}

func polyMulDesc(a []byte, b []byte) []byte {
	out := make([]byte, len(a)+len(b)-1)
	for i, av := range a {
		if av == 0 {
			continue
		}
		for j, bv := range b {
			if bv != 0 {
				out[i+j] ^= gfMul(av, bv)
			}
		}
	}
	return out
}

func rsSyndromes(msg []byte, parityBytes int) []byte {
	synd := make([]byte, parityBytes)
	for i := 0; i < parityBytes; i++ {
		synd[i] = polyEvalDesc(msg, gfPow2(i))
	}
	return synd
}

func polyEvalDesc(poly []byte, x byte) byte {
	y := byte(0)
	for _, coef := range poly {
		y = gfMul(y, x) ^ coef
	}
	return y
}

func syndromesZero(synd []byte) bool {
	for _, s := range synd {
		if s != 0 {
			return false
		}
	}
	return true
}

func rsFindErrorLocator(synd []byte, parityBytes int) ([]byte, int, error) {
	c := []byte{1}
	b := []byte{1}
	l := 0
	m := 1
	bb := byte(1)

	for n := 0; n < parityBytes; n++ {
		discrepancy := synd[n]
		for i := 1; i <= l; i++ {
			if i < len(c) && n-i >= 0 {
				discrepancy ^= gfMul(c[i], synd[n-i])
			}
		}
		if discrepancy == 0 {
			m++
			continue
		}

		t := append([]byte(nil), c...)
		coef := gfDiv(discrepancy, bb)
		if len(c) < len(b)+m {
			grown := make([]byte, len(b)+m)
			copy(grown, c)
			c = grown
		}
		for i, bv := range b {
			if bv != 0 {
				c[i+m] ^= gfMul(coef, bv)
			}
		}
		if 2*l <= n {
			l = n + 1 - l
			b = t
			bb = discrepancy
			m = 1
		} else {
			m++
		}
	}
	if l*2 > parityBytes {
		return nil, 0, fmt.Errorf("too many errors for %d parity bytes", parityBytes)
	}
	return c[:l+1], l, nil
}

func rsFindErrorPositions(locator []byte, errs int, msgLen int) ([]int, error) {
	positions := make([]int, 0, errs)
	for pos := 0; pos < msgLen; pos++ {
		x := gfPow2(pos + 1 - msgLen)
		if polyEvalAsc(locator, x) == 0 {
			positions = append(positions, pos)
		}
	}
	if len(positions) != errs {
		return nil, fmt.Errorf("could not locate errors: got %d, want %d", len(positions), errs)
	}
	return positions, nil
}

func polyEvalAsc(poly []byte, x byte) byte {
	y := byte(0)
	for i := len(poly) - 1; i >= 0; i-- {
		y = gfMul(y, x) ^ poly[i]
	}
	return y
}

func rsSolveErrorMagnitudes(synd []byte, positions []int, msgLen int) ([]byte, error) {
	count := len(positions)
	if count == 0 {
		return nil, nil
	}
	matrix := make([][]byte, count)
	for row := 0; row < count; row++ {
		matrix[row] = make([]byte, count+1)
		for col, pos := range positions {
			matrix[row][col] = gfPow2(row * (msgLen - 1 - pos))
		}
		matrix[row][count] = synd[row]
	}

	for col := 0; col < count; col++ {
		pivot := col
		for pivot < count && matrix[pivot][col] == 0 {
			pivot++
		}
		if pivot == count {
			return nil, fmt.Errorf("singular error magnitude matrix")
		}
		if pivot != col {
			matrix[pivot], matrix[col] = matrix[col], matrix[pivot]
		}
		inv := gfDiv(1, matrix[col][col])
		for j := col; j <= count; j++ {
			matrix[col][j] = gfMul(matrix[col][j], inv)
		}
		for row := 0; row < count; row++ {
			if row == col || matrix[row][col] == 0 {
				continue
			}
			factor := matrix[row][col]
			for j := col; j <= count; j++ {
				matrix[row][j] ^= gfMul(factor, matrix[col][j])
			}
		}
	}

	out := make([]byte, count)
	for i := range out {
		out[i] = matrix[i][count]
	}
	return out, nil
}
