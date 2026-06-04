package symbol

import (
	"image"
	"image/color"
	"testing"
)

// TestImageHashSameImage 测试相同图像生成相同 hash
func TestImageHashSameImage(t *testing.T) {
	img := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	hash1 := ImageHash(img)
	hash2 := ImageHash(img)

	if hash1 != hash2 {
		t.Errorf("Same image should produce same hash, got %x and %x", hash1, hash2)
	}
}

// TestImageHashDifferent 测试不同图像生成不同 hash
func TestImageHashDifferent(t *testing.T) {
	img1 := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	img2 := createTestImage([][]uint8{
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
	})

	hash1 := ImageHash(img1)
	hash2 := ImageHash(img2)

	if hash1 == hash2 {
		t.Errorf("Different images should produce different hash")
	}
}

// TestHammingDistance 测试汉明距离计算
func TestHammingDistance(t *testing.T) {
	tests := []struct {
		a, b     uint64
		expected int
	}{
		{0x0000000000000000, 0x0000000000000000, 0},  // 相同
		{0x0000000000000001, 0x0000000000000000, 1},  // 差 1 bit
		{0x000000000000000F, 0x0000000000000000, 4},  // 差 4 bits
		{0xFFFFFFFFFFFFFFFF, 0x0000000000000000, 64}, // 完全相反
		{0xAAAAAAAAAAAAAAAA, 0x5555555555555555, 64}, // 完全相反（交替）
	}

	for _, tt := range tests {
		result := HammingDistance(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("HammingDistance(%x, %x) = %d, want %d",
				tt.a, tt.b, result, tt.expected)
		}
	}
}

// TestResizeToTile 测试图像缩放
func TestResizeToTile(t *testing.T) {
	// 创建一个 16x16 的图像
	img := image.NewGray(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			// 左半边黑，右半边白
			if x < 8 {
				img.SetGray(x, y, color.Gray{Y: 0})
			} else {
				img.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	// 缩放到 8x8
	resized := ResizeToTile(img)

	// 检查尺寸
	bounds := resized.Bounds()
	if bounds.Dx() != 8 || bounds.Dy() != 8 {
		t.Errorf("Resized image should be 8x8, got %dx%d", bounds.Dx(), bounds.Dy())
	}

	// 检查内容（左半边应该是暗的，右半边应该是亮的）
	for y := 0; y < 8; y++ {
		leftGray := resized.GrayAt(0, y).Y
		rightGray := resized.GrayAt(7, y).Y

		if leftGray > 128 {
			t.Errorf("Left side should be dark at y=%d, got gray=%d", y, leftGray)
		}
		if rightGray < 128 {
			t.Errorf("Right side should be bright at y=%d, got gray=%d", y, rightGray)
		}
	}
}

// createTestImage 创建测试用的 8x8 图像
func createTestImage(pixels [][]uint8) image.Image {
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.SetGray(x, y, color.Gray{Y: pixels[y][x]})
		}
	}
	return img
}

// BenchmarkImageHash 性能测试
func BenchmarkImageHash(b *testing.B) {
	img := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ImageHash(img)
	}
}

// BenchmarkHammingDistance 汉明距离性能测试
func BenchmarkHammingDistance(b *testing.B) {
	a := uint64(0xAAAAAAAAAAAAAAAA)
	b_val := uint64(0x5555555555555555)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HammingDistance(a, b_val)
	}
}
