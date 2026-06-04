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
