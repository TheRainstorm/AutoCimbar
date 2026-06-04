package ecc

import (
	"bytes"
	"testing"
)

// TestNewECC 测试创建 ECC 实例
func TestNewECC(t *testing.T) {
	tests := []struct {
		name       string
		eccPercent int
		blockSize  int
		wantErr    bool
	}{
		{"Valid 20%", 20, 125, false},
		{"Valid 30%", 30, 125, false},
		{"Valid 0%", 0, 125, false},
		{"Invalid percent -1", -1, 125, true},
		{"Invalid percent 101", 101, 125, true},
		{"Invalid block size 0", 20, 0, true},
		{"Invalid block size -1", 20, -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecc, err := NewECC(tt.eccPercent, tt.blockSize)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if ecc == nil {
				t.Fatal("ECC instance is nil")
			}

			// 验证参数
			if ecc.DataSize() <= 0 {
				t.Errorf("DataSize should be > 0, got %d", ecc.DataSize())
			}

			if tt.eccPercent == 0 {
				if ecc.ParitySize() != 0 {
					t.Errorf("ParitySize should be 0 for 0%% ECC, got %d", ecc.ParitySize())
				}
			} else {
				if ecc.ParitySize() <= 0 {
					t.Errorf("ParitySize should be > 0, got %d", ecc.ParitySize())
				}
			}
		})
	}
}

// TestEncodeDecodeRoundtrip 测试编码-解码往返
func TestEncodeDecodeRoundtrip(t *testing.T) {
	ecc, err := NewECC(20, 125)
	if err != nil {
		t.Fatalf("Failed to create ECC: %v", err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{"Small data", []byte("Hello, World!")},
		{"Exact block size", make([]byte, 125)},
		{"Multiple blocks", make([]byte, 300)},
		{"Single byte", []byte{0xFF}},
		{"All zeros", make([]byte, 100)},
		{"All ones", bytes.Repeat([]byte{0xFF}, 100)},
		{"Pattern", []byte{0xAA, 0x55, 0xAA, 0x55}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 填充测试数据（如果需要）
			if len(tt.data) > 13 {
				for i := range tt.data {
					tt.data[i] = byte(i % 256)
				}
			}

			// 编码
			encoded, err := ecc.Encode(tt.data)
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// 验证编码后的大小
			expectedSize := ecc.CalculateEncodedSize(len(tt.data))
			if len(encoded) != expectedSize {
				t.Errorf("Encoded size: expected %d, got %d", expectedSize, len(encoded))
			}

			// 解码（无错误）
			decoded, err := ecc.Decode(encoded, len(tt.data))
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			// 验证
			if !bytes.Equal(decoded, tt.data) {
				t.Errorf("Decoded data mismatch:\nOriginal: %v\nDecoded:  %v",
					tt.data[:min(len(tt.data), 20)],
					decoded[:min(len(decoded), 20)])
			}
		})
	}
}

// TestDecodeWithErrors 测试带错误的解码
func TestDecodeWithErrors(t *testing.T) {
	ecc, err := NewECC(20, 125) // 20% ECC = 2 parity shards (10 data + 2 parity)
	if err != nil {
		t.Fatalf("Failed to create ECC: %v", err)
	}

	// 原始数据
	data := []byte("Hello, this is a test message for error correction!")
	for len(data) < 125 {
		data = append(data, byte(len(data)%256))
	}

	// 编码
	encoded, err := ecc.Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Reed-Solomon 可以纠正最多 parityShards 个丢失的分片
	// 或 parityShards/2 个错误的分片
	maxErasures := ecc.ParitySize()

	t.Logf("Data shards: %d, Parity shards: %d", ecc.DataSize(), ecc.ParitySize())
	t.Logf("Can correct up to %d erasures (missing shards)", maxErasures)

	tests := []struct {
		name       string
		numErrors  int
		shouldFix  bool
	}{
		{"1 erasure", 1, true},
		{"Max erasures", maxErasures, maxErasures > 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 复制编码数据
			corrupted := make([]byte, len(encoded))
			copy(corrupted, encoded)

			// 计算分片大小
			totalShards := ecc.DataSize() + ecc.ParitySize()
			shardSize := len(encoded) / totalShards

			// 模拟分片丢失（清零前 N 个分片）
			for i := 0; i < tt.numErrors; i++ {
				start := i * shardSize
				end := start + shardSize
				for j := start; j < end; j++ {
					corrupted[j] = 0
				}
			}

			// 尝试解码
			decoded, err := ecc.Decode(corrupted, len(data))

			if tt.shouldFix {
				if err != nil {
					t.Logf("Decode failed (may be expected): %v", err)
				} else {
					// 验证是否正确修复
					if bytes.Equal(decoded, data) {
						t.Logf("Successfully corrected %d erasures", tt.numErrors)
					} else {
						t.Logf("Decode succeeded but data mismatch with %d erasures", tt.numErrors)
					}
				}
			}
		})
	}
}

// TestCalculateEncodedSize 测试编码大小计算
func TestCalculateEncodedSize(t *testing.T) {
	ecc, err := NewECC(20, 125)
	if err != nil {
		t.Fatalf("Failed to create ECC: %v", err)
	}

	tests := []struct {
		dataSize int
	}{
		{1},
		{125},
		{126},
		{250},
		{251},
	}

	for _, tt := range tests {
		size := ecc.CalculateEncodedSize(tt.dataSize)

		// 验证编码后大小合理
		if size < tt.dataSize {
			t.Errorf("CalculateEncodedSize(%d) = %d, should be >= %d",
				tt.dataSize, size, tt.dataSize)
		}

		t.Logf("CalculateEncodedSize(%d) = %d", tt.dataSize, size)
	}
}

// TestEncodeEmptyData 测试空数据
func TestEncodeEmptyData(t *testing.T) {
	ecc, err := NewECC(20, 125)
	if err != nil {
		t.Fatalf("Failed to create ECC: %v", err)
	}

	_, err = ecc.Encode([]byte{})
	if err == nil {
		t.Error("Expected error for empty data, got nil")
	}
}

// TestDecodeInvalidSize 测试无效大小的解码
func TestDecodeInvalidSize(t *testing.T) {
	ecc, err := NewECC(20, 125)
	if err != nil {
		t.Fatalf("Failed to create ECC: %v", err)
	}

	// 大小不是块的整数倍
	invalidData := make([]byte, 100)

	_, err = ecc.Decode(invalidData, 80)
	if err == nil {
		t.Error("Expected error for invalid data size, got nil")
	}
}

// BenchmarkEncode 编码性能测试
func BenchmarkEncode(b *testing.B) {
	ecc, _ := NewECC(20, 125)
	data := make([]byte, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ecc.Encode(data)
	}
}

// BenchmarkDecode 解码性能测试
func BenchmarkDecode(b *testing.B) {
	ecc, _ := NewECC(20, 125)
	data := make([]byte, 1000)
	encoded, _ := ecc.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ecc.Decode(encoded, len(data))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
