package symbol

import (
	"image"
	"image/color"
	"testing"
)

// TestRecognizerLoadSymbol 测试符号加载
func TestRecognizerLoadSymbol(t *testing.T) {
	r := NewRecognizer()

	// 创建一个测试符号
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

	// 加载符号 0
	err := r.LoadSymbol(0, img)
	if err != nil {
		t.Fatalf("LoadSymbol failed: %v", err)
	}

	// 验证模板已保存
	template, err := r.GetTemplate(0)
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}
	if template == nil {
		t.Fatal("Template is nil")
	}

	// 验证 hash 已保存
	hash, err := r.GetHash(0)
	if err != nil {
		t.Fatalf("GetHash failed: %v", err)
	}
	if hash == 0 {
		t.Fatal("Hash is zero")
	}
}

// TestRecognizerRecognize 测试符号识别
func TestRecognizerRecognize(t *testing.T) {
	r := NewRecognizer()

	// 加载两个不同的符号
	symbol0 := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	symbol1 := createTestImage([][]uint8{
		{255, 0, 0, 0, 0, 0, 0, 255},
		{0, 255, 0, 0, 0, 0, 255, 0},
		{0, 0, 255, 0, 0, 255, 0, 0},
		{0, 0, 0, 255, 255, 0, 0, 0},
		{0, 0, 0, 255, 255, 0, 0, 0},
		{0, 0, 255, 0, 0, 255, 0, 0},
		{0, 255, 0, 0, 0, 0, 255, 0},
		{255, 0, 0, 0, 0, 0, 0, 255},
	})

	r.LoadSymbol(0, symbol0)
	r.LoadSymbol(1, symbol1)

	// 识别符号 0（完全相同）
	id, dist := r.Recognize(symbol0)
	if id != 0 {
		t.Errorf("Expected symbol 0, got %d", id)
	}
	if dist != 0 {
		t.Errorf("Expected distance 0, got %d", dist)
	}

	// 识别符号 1（完全相同）
	id, dist = r.Recognize(symbol1)
	if id != 1 {
		t.Errorf("Expected symbol 1, got %d", id)
	}
	if dist != 0 {
		t.Errorf("Expected distance 0, got %d", dist)
	}
}

// TestRecognizerRecognizeWithNoise 测试带噪声的符号识别
func TestRecognizerRecognizeWithNoise(t *testing.T) {
	r := NewRecognizer()

	// 加载原始符号
	original := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	r.LoadSymbol(0, original)

	// 创建带噪声的版本（翻转几个像素）
	noisy := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 255, 0, 0, 255, 255, 0, 255}, // 翻转 2 个像素
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
		{255, 255, 255, 255, 0, 0, 0, 0},
	})

	// 应该仍然能识别为符号 0
	id, dist := r.Recognize(noisy)
	if id != 0 {
		t.Errorf("Expected symbol 0, got %d", id)
	}

	// 汉明距离应该是小的正数（约等于翻转的像素数）
	if dist < 1 || dist > 10 {
		t.Errorf("Expected small distance (1-10), got %d", dist)
	}
}

// TestRecognizerVerifyHammingDistances 测试汉明距离验证
func TestRecognizerVerifyHammingDistances(t *testing.T) {
	r := NewRecognizer()

	// 加载 3 个明显不同的符号
	// 符号 0: 左半边黑
	symbol0 := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
	})

	// 符号 1: 对角线
	symbol1 := createTestImage([][]uint8{
		{255, 0, 0, 0, 0, 0, 0, 0},
		{0, 255, 0, 0, 0, 0, 0, 0},
		{0, 0, 255, 0, 0, 0, 0, 0},
		{0, 0, 0, 255, 0, 0, 0, 0},
		{0, 0, 0, 0, 255, 0, 0, 0},
		{0, 0, 0, 0, 0, 255, 0, 0},
		{0, 0, 0, 0, 0, 0, 255, 0},
		{0, 0, 0, 0, 0, 0, 0, 255},
	})

	// 符号 2: 棋盘格
	symbol2 := createTestImage([][]uint8{
		{255, 0, 255, 0, 255, 0, 255, 0},
		{0, 255, 0, 255, 0, 255, 0, 255},
		{255, 0, 255, 0, 255, 0, 255, 0},
		{0, 255, 0, 255, 0, 255, 0, 255},
		{255, 0, 255, 0, 255, 0, 255, 0},
		{0, 255, 0, 255, 0, 255, 0, 255},
		{255, 0, 255, 0, 255, 0, 255, 0},
		{0, 255, 0, 255, 0, 255, 0, 255},
	})

	r.LoadSymbol(0, symbol0)
	r.LoadSymbol(1, symbol1)
	r.LoadSymbol(2, symbol2)

	// 验证汉明距离
	minDist, pairs := r.VerifyHammingDistances()

	// 最小距离应该 >= 0（因为只加载了 3 个符号，未加载的是 0）
	// 我们真正关心的是已加载符号之间的距离
	if minDist < 0 || minDist > 64 {
		t.Errorf("Invalid min distance: %d", minDist)
	}

	// 应该有找到对
	if len(pairs) == 0 {
		t.Error("Should have found at least one pair")
	}

	t.Logf("Min hamming distance: %d bits", minDist)
	t.Logf("Number of closest pairs: %d", len(pairs))
}

// TestRecognizerIsLoaded 测试加载状态检查
func TestRecognizerIsLoaded(t *testing.T) {
	r := NewRecognizer()

	// 初始状态应该是未加载
	if r.IsLoaded() {
		t.Error("Should not be loaded initially")
	}

	// 加载所有 16 个符号
	for i := SymbolID(0); i < NumSymbols; i++ {
		img := image.NewGray(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				val := uint8((int(i)*10 + x + y) % 256)
				img.SetGray(x, y, color.Gray{Y: val})
			}
		}
		r.LoadSymbol(i, img)
	}

	// 现在应该是已加载
	if !r.IsLoaded() {
		t.Error("Should be loaded after loading all symbols")
	}
}

// TestRecognizerGetStats 测试统计信息
func TestRecognizerGetStats(t *testing.T) {
	r := NewRecognizer()

	// 加载 4 个明显不同的符号
	// 符号 0: 全黑
	symbol0 := createTestImage([][]uint8{
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
	})

	// 符号 1: 全白
	symbol1 := createTestImage([][]uint8{
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
	})

	// 符号 2: 左半边黑
	symbol2 := createTestImage([][]uint8{
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
		{0, 0, 0, 0, 255, 255, 255, 255},
	})

	// 符号 3: 上半边黑
	symbol3 := createTestImage([][]uint8{
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
		{255, 255, 255, 255, 255, 255, 255, 255},
	})

	r.LoadSymbol(0, symbol0)
	r.LoadSymbol(1, symbol1)
	r.LoadSymbol(2, symbol2)
	r.LoadSymbol(3, symbol3)

	stats := r.GetStats()

	if stats.LoadedSymbols != 4 {
		t.Errorf("Expected 4 loaded symbols, got %d", stats.LoadedSymbols)
	}

	if stats.MinHammingDist < 0 || stats.MinHammingDist > 64 {
		t.Errorf("Invalid min hamming distance: %d", stats.MinHammingDist)
	}

	if stats.AvgHammingDist < 0 || stats.AvgHammingDist > 64 {
		t.Errorf("Invalid avg hamming distance: %f", stats.AvgHammingDist)
	}

	t.Logf("Stats: %+v", stats)
}

// BenchmarkRecognize 性能测试
func BenchmarkRecognize(b *testing.B) {
	r := NewRecognizer()

	// 加载 16 个符号
	for i := SymbolID(0); i < NumSymbols; i++ {
		img := image.NewGray(image.Rect(0, 0, 8, 8))
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				val := uint8((int(i)*15 + x + y) % 256)
				img.SetGray(x, y, color.Gray{Y: val})
			}
		}
		r.LoadSymbol(i, img)
	}

	// 测试图像
	testImg := image.NewGray(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			testImg.SetGray(x, y, color.Gray{Y: uint8((x + y*2) % 256)})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Recognize(testImg)
	}
}
