package color

import (
	"image"
	"image/color"
	"testing"
)

// TestRecognizer4ColorRecognize 测试 4 色识别
func TestRecognizer4ColorRecognize(t *testing.T) {
	r := NewRecognizer4Color()

	tests := []struct {
		name     string
		color    color.RGBA
		expected ColorID
	}{
		{"Green", color.RGBA{R: 0, G: 255, B: 0, A: 255}, ColorGreen},
		{"White", color.RGBA{R: 255, G: 255, B: 255, A: 255}, ColorWhite},
		{"Red", color.RGBA{R: 255, G: 0, B: 0, A: 255}, ColorRed},
		{"Blue", color.RGBA{R: 0, G: 0, B: 255, A: 255}, ColorBlue},
		// 近似颜色
		{"Dark green", color.RGBA{R: 0, G: 200, B: 0, A: 255}, ColorGreen},
		{"Light gray (should be white)", color.RGBA{R: 220, G: 220, B: 220, A: 255}, ColorWhite},
		{"Orange-red (should be red)", color.RGBA{R: 255, G: 50, B: 0, A: 255}, ColorRed},
		{"Dark blue", color.RGBA{R: 0, G: 0, B: 200, A: 255}, ColorBlue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := createSolidColorImage(tt.color, 8, 8)
			id, dist := r.Recognize(img)

			if id != tt.expected {
				t.Errorf("Expected %v, got %v (distance: %f)", tt.expected, id, dist)
			}

			t.Logf("%s: ID=%d, Distance=%f", tt.name, id, dist)
		})
	}
}

// TestRGBToLAB 测试 RGB 到 LAB 转换
func TestRGBToLAB(t *testing.T) {
	tests := []struct {
		name string
		rgb  color.RGBA
		// 预期的大致 LAB 值（允许一定误差）
		expectL   float64
		tolerance float64
	}{
		{"Black", color.RGBA{R: 0, G: 0, B: 0, A: 255}, 0, 5},
		{"White", color.RGBA{R: 255, G: 255, B: 255, A: 255}, 100, 5},
		{"Red", color.RGBA{R: 255, G: 0, B: 0, A: 255}, 53, 5},
		{"Blue", color.RGBA{R: 0, G: 0, B: 255, A: 255}, 32, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lab := RGBToLAB(tt.rgb)

			// 检查 L 值是否在合理范围
			if lab.L < tt.expectL-tt.tolerance || lab.L > tt.expectL+tt.tolerance {
				t.Errorf("L value out of range: got %f, expected %f±%f",
					lab.L, tt.expectL, tt.tolerance)
			}

			t.Logf("%s: RGB(%d,%d,%d) -> LAB(%.2f, %.2f, %.2f)",
				tt.name, tt.rgb.R, tt.rgb.G, tt.rgb.B, lab.L, lab.A, lab.B)
		})
	}
}

func TestRecognizeColorRGB(t *testing.T) {
	r := NewRecognizer4Color()

	tests := []struct {
		name     string
		color    color.RGBA
		expected ColorID
	}{
		{"Green", color.RGBA{R: 0, G: 240, B: 0, A: 255}, ColorGreen},
		{"White", color.RGBA{R: 230, G: 230, B: 230, A: 255}, ColorWhite},
		{"Red", color.RGBA{R: 240, G: 20, B: 10, A: 255}, ColorRed},
		{"Blue", color.RGBA{R: 10, G: 20, B: 240, A: 255}, ColorBlue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, _ := r.RecognizeColorRGB(tt.color)
			if id != tt.expected {
				t.Fatalf("RecognizeColorRGB = %d, want %d", id, tt.expected)
			}
		})
	}
}

// TestColorDistance 测试颜色距离计算
func TestColorDistance(t *testing.T) {
	black := RGBToLAB(color.RGBA{R: 0, G: 0, B: 0, A: 255})
	white := RGBToLAB(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	red := RGBToLAB(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	blue := RGBToLAB(color.RGBA{R: 0, G: 0, B: 255, A: 255})

	// 相同颜色距离应该是 0
	distSame := colorDistance(black, black)
	if distSame != 0 {
		t.Errorf("Distance between same color should be 0, got %f", distSame)
	}

	// 黑白距离应该很大
	distBlackWhite := colorDistance(black, white)
	if distBlackWhite < 50 {
		t.Errorf("Distance between black and white should be large, got %f", distBlackWhite)
	}

	// 红蓝距离也应该较大
	distRedBlue := colorDistance(red, blue)
	if distRedBlue < 30 {
		t.Errorf("Distance between red and blue should be large, got %f", distRedBlue)
	}

	t.Logf("Black-White distance: %.2f", distBlackWhite)
	t.Logf("Red-Blue distance: %.2f", distRedBlue)
	t.Logf("Black-Red distance: %.2f", colorDistance(black, red))
	t.Logf("Black-Blue distance: %.2f", colorDistance(black, blue))
}

// TestRecognizeWithNoise 测试带噪声的颜色识别
func TestRecognizeWithNoise(t *testing.T) {
	r := NewRecognizer4Color()

	// 创建主要是红色但有一些噪声的图像
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%3 == 0 {
				// 10% 的像素是白色噪声
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				// 90% 的像素是红色
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			}
		}
	}

	id, dist := r.Recognize(img)

	// 应该仍然识别为红色
	if id != ColorRed {
		t.Errorf("Expected red, got %v (distance: %f)", id, dist)
	}

	t.Logf("Noisy red image recognized as: %v, distance: %f", id, dist)
}

// TestGetColor 测试获取颜色
func TestGetColor(t *testing.T) {
	r := NewRecognizer4Color()

	tests := []struct {
		id       ColorID
		expected color.RGBA
	}{
		{ColorGreen, color.RGBA{R: 0, G: 255, B: 0, A: 255}},
		{ColorWhite, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{ColorRed, color.RGBA{R: 255, G: 0, B: 0, A: 255}},
		{ColorBlue, color.RGBA{R: 0, G: 0, B: 255, A: 255}},
	}

	for _, tt := range tests {
		c := r.GetColor(tt.id)
		if c != tt.expected {
			t.Errorf("GetColor(%v) = %v, want %v", tt.id, c, tt.expected)
		}
	}
}

// TestComputeAverageColor 测试平均颜色计算
func TestComputeAverageColor(t *testing.T) {
	// 创建一半红一半蓝的图像
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if x < 4 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255}) // 红色
			} else {
				img.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255}) // 蓝色
			}
		}
	}

	avgColor := computeAverageColor(img)

	// 平均颜色应该是紫色（红+蓝的混合）
	t.Logf("Average color: RGB(%d, %d, %d)", avgColor.R, avgColor.G, avgColor.B)

	// R 应该约为 128 (255/2)
	if avgColor.R < 120 || avgColor.R > 135 {
		t.Errorf("Expected R around 128, got %d", avgColor.R)
	}

	// G 应该约为 0
	if avgColor.G > 10 {
		t.Errorf("Expected G around 0, got %d", avgColor.G)
	}

	// B 应该约为 128 (255/2)
	if avgColor.B < 120 || avgColor.B > 135 {
		t.Errorf("Expected B around 128, got %d", avgColor.B)
	}
}

// createSolidColorImage 创建纯色图像
func createSolidColorImage(c color.RGBA, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// BenchmarkRecognize 性能测试
func BenchmarkRecognize(b *testing.B) {
	r := NewRecognizer4Color()
	img := createSolidColorImage(color.RGBA{R: 255, G: 0, B: 0, A: 255}, 8, 8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Recognize(img)
	}
}

// BenchmarkRGBToLAB RGB 到 LAB 转换性能测试
func BenchmarkRGBToLAB(b *testing.B) {
	c := color.RGBA{R: 128, G: 64, B: 200, A: 255}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RGBToLAB(c)
	}
}

func BenchmarkRecognizeColorRGB(b *testing.B) {
	r := NewRecognizer4Color()
	c := color.RGBA{R: 128, G: 64, B: 200, A: 255}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.RecognizeColorRGB(c)
	}
}
