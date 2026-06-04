package codec

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/symbol"
)

// TestBytesToCells 测试字节到 cells 转换
func TestBytesToCells(t *testing.T) {
	// 创建测试用的识别器
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)

	// 测试数据：1 字节 = 0b10110011
	data := []byte{0b10110011}

	cells, err := encoder.bytesToCells(data)
	if err != nil {
		t.Fatalf("bytesToCells failed: %v", err)
	}

	// 1 字节 = 8 bits，需要 2 个 cells (每个 cell 6 bits)
	// Cell 0: 101100 = color=10 (2), shape=1100 (12)
	// Cell 1: 110000 = color=11 (3), shape=0000 (0) [最后 2 bits 补 0]
	if len(cells) != 2 {
		t.Errorf("Expected 2 cells, got %d", len(cells))
	}

	// 验证第一个 cell
	if cells[0].Color != 2 || cells[0].Shape != 12 {
		t.Errorf("Cell 0: expected color=2 shape=12, got color=%d shape=%d",
			cells[0].Color, cells[0].Shape)
	}

	// 验证第二个 cell
	if cells[1].Color != 3 || cells[1].Shape != 0 {
		t.Errorf("Cell 1: expected color=3 shape=0, got color=%d shape=%d",
			cells[1].Color, cells[1].Shape)
	}
}

// TestCellsToBytes 测试 cells 到字节转换
func TestCellsToBytes(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	decoder := NewDecoder(symRec, colorRec, 8, 10)

	// 构造 cells
	cells := []Cell{
		{Color: 2, Shape: 12}, // 101100
		{Color: 3, Shape: 0},  // 110000
	}

	data := decoder.cellsToBytes(cells)

	// 应该得到 0b10110011 0b00000000
	expected := []byte{0b10110011, 0b00000000}

	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}
}

// TestEncodeDecodeRoundtrip 测试编码-解码往返
func TestEncodeDecodeRoundtrip(t *testing.T) {
	// 创建识别器
	symRec, colorRec := createTestRecognizers(t)

	// 创建编码器和解码器
	encoder := NewEncoder(symRec, colorRec, 8, 10)
	decoder := NewDecoder(symRec, colorRec, 8, 10)

	// 测试数据
	tests := []struct {
		name string
		data []byte
	}{
		{"Single byte", []byte{0xFF}},
		{"Multiple bytes", []byte{0x12, 0x34, 0x56, 0x78}},
		{"ASCII string", []byte("Hello")},
		{"Zero bytes", []byte{0x00, 0x00, 0x00}},
		{"Mixed pattern", []byte{0xAA, 0x55, 0xF0, 0x0F}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 先测试不经过图像的往返（纯逻辑）
			cells, err := encoder.bytesToCells(tt.data)
			if err != nil {
				t.Fatalf("bytesToCells failed: %v", err)
			}

			decoded := decoder.cellsToBytes(cells)

			// 计算需要的字节数
			totalBits := len(tt.data) * 8
			numCells := (totalBits + CellBits - 1) / CellBits
			decodedBits := numCells * CellBits
			decodedBytes := (decodedBits + 7) / 8

			// 只比较有效数据部分
			minLen := len(tt.data)
			if len(decoded) < minLen {
				t.Fatalf("Decoded data too short: expected at least %d bytes, got %d",
					minLen, len(decoded))
			}

			// 比较数据（考虑最后可能有填充位）
			lastByteIndex := len(tt.data) - 1
			for i := 0; i < lastByteIndex; i++ {
				if decoded[i] != tt.data[i] {
					t.Errorf("Byte %d mismatch: expected 0x%02X, got 0x%02X",
						i, tt.data[i], decoded[i])
				}
			}

			// 最后一个字节可能有填充，需要掩码比较
			validBits := (len(tt.data) * 8) % 8
			if validBits == 0 {
				validBits = 8
			}

			if validBits == 8 {
				// 完整字节
				if decoded[lastByteIndex] != tt.data[lastByteIndex] {
					t.Errorf("Last byte mismatch: expected 0x%02X, got 0x%02X",
						tt.data[lastByteIndex], decoded[lastByteIndex])
				}
			} else {
				// 部分字节，需要掩码
				mask := uint8(0xFF << (8 - validBits))
				expected := tt.data[lastByteIndex] & mask
				actual := decoded[lastByteIndex] & mask
				if actual != expected {
					t.Errorf("Last byte (masked) mismatch: expected 0x%02X, got 0x%02X (mask 0x%02X)",
						expected, actual, mask)
				}
			}

			t.Logf("Original: %v", tt.data)
			t.Logf("Decoded:  %v (first %d bytes)", decoded[:decodedBytes], len(tt.data))
		})
	}
}

func TestBytesToCellsRoundTripVariableColorBits(t *testing.T) {
	symRec, _ := createTestRecognizers(t)
	tests := []struct {
		name         string
		colorBits    int
		colorRec     *colorpkg.Recognizer
		expectedCell int
	}{
		{"2color", 1, colorpkg.NewRecognizer2Color(), 5},
		{"4color", 2, colorpkg.NewRecognizer4Color(), 6},
		{"8color", 3, colorpkg.NewRecognizer8Color(), 7},
		{"16color", 4, colorpkg.NewRecognizer16Color(), 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder, err := NewEncoderWithColorBits(symRec, tt.colorRec, 8, 10, tt.colorBits)
			if err != nil {
				t.Fatalf("NewEncoderWithColorBits failed: %v", err)
			}
			decoder, err := NewDecoderWithColorBits(symRec, tt.colorRec, 8, 10, tt.colorBits)
			if err != nil {
				t.Fatalf("NewDecoderWithColorBits failed: %v", err)
			}
			if encoder.cellBits != tt.expectedCell || decoder.cellBits != tt.expectedCell {
				t.Fatalf("cellBits encoder=%d decoder=%d, want %d", encoder.cellBits, decoder.cellBits, tt.expectedCell)
			}

			data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
			cells, err := encoder.bytesToCells(data)
			if err != nil {
				t.Fatalf("bytesToCells failed: %v", err)
			}
			decoded := decoder.cellsToBytes(cells)
			if !bytes.Equal(decoded[:len(data)], data) {
				t.Fatalf("decoded prefix = %x, want %x", decoded[:len(data)], data)
			}
		})
	}
}

func TestBytesToCells16ColorNibbles(t *testing.T) {
	symRec, _ := createTestRecognizers(t)
	colorRec := colorpkg.NewRecognizer16Color()
	encoder, err := NewEncoderWithColorBits(symRec, colorRec, 8, 10, 4)
	if err != nil {
		t.Fatalf("NewEncoderWithColorBits failed: %v", err)
	}

	data := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
	cells, err := encoder.bytesToCells(data)
	if err != nil {
		t.Fatalf("bytesToCells failed: %v", err)
	}
	if len(cells) != len(data) {
		t.Fatalf("cells len = %d, want %d", len(cells), len(data))
	}
	for i, cell := range cells {
		if uint8(cell.Color) != data[i]>>4 || uint8(cell.Shape) != data[i]&0x0f {
			t.Fatalf("cell %d = color %d shape %d, want color %d shape %d", i, cell.Color, cell.Shape, data[i]>>4, data[i]&0x0f)
		}
	}
}

// TestEncodeTooLarge 测试数据过大的情况
func TestEncodeTooLarge(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)

	// 10x10 grid = 100 cells
	// 100 cells * 6 bits = 600 bits = 75 bytes
	// 尝试编码 100 bytes（应该失败）
	largeData := make([]byte, 100)

	_, err := encoder.Encode(largeData)
	if err == nil {
		t.Error("Expected error for too large data, got nil")
	}
}

func TestEncodeBGRAMatchesEncode(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)
	data := []byte("bgra output should match rgba output")

	img, err := encoder.Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	bgra, err := encoder.EncodeBGRA(data, nil)
	if err != nil {
		t.Fatalf("EncodeBGRA failed: %v", err)
	}
	if len(bgra) != len(img.Pix) {
		t.Fatalf("BGRA len = %d, want %d", len(bgra), len(img.Pix))
	}
	for i := 0; i < len(img.Pix); i += 4 {
		if bgra[i+0] != img.Pix[i+2] || bgra[i+1] != img.Pix[i+1] || bgra[i+2] != img.Pix[i+0] || bgra[i+3] != img.Pix[i+3] {
			t.Fatalf("pixel %d BGRA=(%d,%d,%d,%d) RGBA=(%d,%d,%d,%d)",
				i/4, bgra[i+0], bgra[i+1], bgra[i+2], bgra[i+3],
				img.Pix[i+0], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3])
		}
	}
}

func TestDecodeIntoMatchesDecode(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)
	decoder := NewDecoder(symRec, colorRec, 8, 10)
	data := []byte("decode into should match decode")

	img, err := encoder.Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	decoded, err := decoder.Decode(img)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	into, err := decoder.DecodeInto(img, nil)
	if err != nil {
		t.Fatalf("DecodeInto failed: %v", err)
	}
	if !bytes.Equal(into, decoded) {
		t.Fatal("DecodeInto output differs from Decode")
	}
}

func TestDecodeBGRAIntoMatchesDecode(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)
	decoder := NewDecoder(symRec, colorRec, 8, 10)
	data := []byte("decode bgra into should match decode")

	img, err := encoder.Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	bgra, err := encoder.EncodeBGRA(data, nil)
	if err != nil {
		t.Fatalf("EncodeBGRA failed: %v", err)
	}
	decoded, err := decoder.Decode(img)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	into, err := decoder.DecodeBGRAInto(bgra, img.Bounds().Dx(), img.Bounds().Dy(), img.Bounds().Dx()*4, nil)
	if err != nil {
		t.Fatalf("DecodeBGRAInto failed: %v", err)
	}
	if !bytes.Equal(into, decoded) {
		t.Fatal("DecodeBGRAInto output differs from Decode")
	}
}

// TestDrawCell 测试 cell 绘制
func TestDrawCell(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 10)

	// 创建测试图像
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	// 绘制一个红色符号 0
	cell := Cell{Color: colorpkg.ColorRed, Shape: 0}
	encoder.drawCell(img, 0, 0, cell)

	// 验证至少有一些像素被绘制了
	hasNonBlack := false
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := img.At(x, y)
			r, g, b, _ := c.RGBA()
			if r > 0 || g > 0 || b > 0 {
				hasNonBlack = true
				break
			}
		}
		if hasNonBlack {
			break
		}
	}

	if !hasNonBlack {
		t.Error("Cell was not drawn (all pixels are black)")
	}
}

// TestExtractCells 测试 cell 提取
func TestExtractCells(t *testing.T) {
	symRec, colorRec := createTestRecognizers(t)
	encoder := NewEncoder(symRec, colorRec, 8, 2)
	decoder := NewDecoder(symRec, colorRec, 8, 10)

	// 创建一个简单的测试图像（2x2 grid）
	decoder.gridSize = 2
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	inputCells := []Cell{
		{Color: colorpkg.ColorGreen, Shape: 0},
		{Color: colorpkg.ColorWhite, Shape: 1},
		{Color: colorpkg.ColorRed, Shape: 2},
		{Color: colorpkg.ColorBlue, Shape: 3},
	}
	for i, cell := range inputCells {
		x0 := (i % 2) * 8
		y0 := (i / 2) * 8
		encoder.drawCell(img, x0, y0, cell)
	}

	// 提取 cells
	cells, err := decoder.extractCells(img)
	if err != nil {
		t.Fatalf("extractCells failed: %v", err)
	}

	if len(cells) != 4 {
		t.Errorf("Expected 4 cells, got %d", len(cells))
	}

	// 验证颜色识别
	expectedColors := []colorpkg.ColorID{
		colorpkg.ColorGreen,
		colorpkg.ColorWhite,
		colorpkg.ColorRed,
		colorpkg.ColorBlue,
	}

	for i, cell := range cells {
		if cell.Color != expectedColors[i] {
			t.Errorf("Cell %d: expected color %d, got %d",
				i, expectedColors[i], cell.Color)
		}
	}
}

// createTestRecognizers 创建测试用的识别器
func createTestRecognizers(t *testing.T) (*symbol.Recognizer, *colorpkg.Recognizer) {
	t.Helper()

	// 创建符号识别器并加载测试符号
	symRec := symbol.NewRecognizer()

	// 加载 16 个简单的测试符号
	for i := symbol.SymbolID(0); i < symbol.NumSymbols; i++ {
		img := createTestSymbol(int(i))
		if err := symRec.LoadSymbol(i, img); err != nil {
			t.Fatalf("Failed to load symbol %d: %v", i, err)
		}
	}

	// 创建颜色识别器
	colorRec := colorpkg.NewRecognizer4Color()

	return symRec, colorRec
}

// createTestSymbol 创建测试符号
func createTestSymbol(id int) image.Image {
	img := image.NewGray(image.Rect(0, 0, 8, 8))

	// 根据 ID 创建不同的图案
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			// 使用简单的模式：基于 ID 和坐标生成不同的图案
			val := uint8(((id*7 + x*3 + y*5) % 2) * 255)
			img.SetGray(x, y, color.Gray{Y: val})
		}
	}

	return img
}

// BenchmarkEncode 编码性能测试
func BenchmarkEncode(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)

	data := make([]byte, 100) // 100 字节测试数据

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encoder.Encode(data)
	}
}

func BenchmarkEncodeFullFrame(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)

	data := make([]byte, 1875)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encoder.Encode(data)
	}
}

func BenchmarkEncodeBGRAFullFrame(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)

	data := make([]byte, 1875)
	var dst []byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		dst, err = encoder.EncodeBGRA(data, dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecode 解码性能测试
func BenchmarkDecode(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)
	decoder := NewDecoder(symRec, colorRec, 8, 50)

	data := make([]byte, 100)
	img, _ := encoder.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decoder.Decode(img)
	}
}

func BenchmarkDecodeFullFrame(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)
	decoder := NewDecoder(symRec, colorRec, 8, 50)

	data := make([]byte, 1875)
	img, _ := encoder.Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decoder.Decode(img)
	}
}

func BenchmarkDecodeIntoFullFrame(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)
	decoder := NewDecoder(symRec, colorRec, 8, 50)

	data := make([]byte, 1875)
	img, _ := encoder.Encode(data)
	var dst []byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		dst, err = decoder.DecodeInto(img, dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeBGRAIntoFullFrame(b *testing.B) {
	symRec, colorRec := createBenchRecognizers(b)
	encoder := NewEncoder(symRec, colorRec, 8, 50)
	decoder := NewDecoder(symRec, colorRec, 8, 50)

	data := make([]byte, 1875)
	bgra, _ := encoder.EncodeBGRA(data, nil)
	width := 50 * 8
	stride := width * 4
	var dst []byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		dst, err = decoder.DecodeBGRAInto(bgra, width, width, stride, dst)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func createBenchRecognizers(b *testing.B) (*symbol.Recognizer, *colorpkg.Recognizer) {
	b.Helper()

	symRec := symbol.NewRecognizer()
	for i := symbol.SymbolID(0); i < symbol.NumSymbols; i++ {
		img := createTestSymbol(int(i))
		symRec.LoadSymbol(i, img)
	}

	colorRec := colorpkg.NewRecognizer4Color()

	return symRec, colorRec
}
