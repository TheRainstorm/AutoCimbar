package codec

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/symbol"
)

// Cell 一个编码单元
type Cell struct {
	Color colorpkg.ColorID
	Shape symbol.SymbolID
}

// CellBits 每个 cell 的总位数
// 4 色 (2 bits) + 16 形状 (4 bits) = 6 bits
const CellBits = 6

// ColorBits 颜色位数 (4 色 = 2 bits)
const ColorBits = 2

// ShapeBits 形状位数 (16 种形状 = 4 bits)
const ShapeBits = 4

// Encoder 编码器
type Encoder struct {
	symbolRecognizer *symbol.Recognizer
	colorRecognizer  *colorpkg.Recognizer
	cellSize         int // cell 的屏幕像素大小 (4 * B)
	gridSize         int // Q 参数，grid 的大小
}

// NewEncoder 创建编码器
// symbolRecognizer: 符号识别器（用于获取符号模板）
// colorRecognizer: 颜色识别器（用于获取颜色）
// cellSize: cell 的屏幕像素大小（例如 8 表示 8x8 像素）
// gridSize: grid 大小（例如 50 表示 50x50 cells）
func NewEncoder(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int) *Encoder {
	return &Encoder{
		symbolRecognizer: symbolRecognizer,
		colorRecognizer:  colorRecognizer,
		cellSize:         cellSize,
		gridSize:         gridSize,
	}
}

// Encode 将数据编码为二维码图像
// data: 待编码的字节数组
// 返回：二维码图像
func (e *Encoder) Encode(data []byte) (*image.RGBA, error) {
	// 1. 将字节数组转换为 cells
	cells, err := e.bytesToCells(data)
	if err != nil {
		return nil, err
	}

	// 2. 渲染为图像
	img := e.renderImage(cells)

	return img, nil
}

// bytesToCells 将字节数组转换为 cells
func (e *Encoder) bytesToCells(data []byte) ([]Cell, error) {
	// 计算需要多少个 cells
	totalBits := len(data) * 8
	numCells := (totalBits + CellBits - 1) / CellBits

	// 检查是否超出 grid 容量
	maxCells := e.gridSize * e.gridSize
	if numCells > maxCells {
		return nil, fmt.Errorf("data too large: need %d cells but grid only has %d cells", numCells, maxCells)
	}

	cells := make([]Cell, numCells)

	bitIndex := 0
	for i := 0; i < numCells; i++ {
		// 读取 6 bits（从左到右）
		var bits uint8 = 0
		bitsRead := 0

		for j := 0; j < CellBits && bitIndex < len(data)*8; j++ {
			byteIndex := bitIndex / 8
			bitOffset := 7 - (bitIndex % 8)

			bit := (data[byteIndex] >> bitOffset) & 1
			bits = (bits << 1) | bit

			bitIndex++
			bitsRead++
		}

		// 如果不足 6 bits，左移补齐
		if bitsRead < CellBits {
			bits <<= (CellBits - bitsRead)
		}

		// 拆分为颜色和形状
		// 高 2 bits = 颜色，低 4 bits = 形状
		colorID := colorpkg.ColorID((bits >> ShapeBits) & 0x03)
		shapeID := symbol.SymbolID(bits & 0x0F)

		cells[i] = Cell{
			Color: colorID,
			Shape: shapeID,
		}
	}

	return cells, nil
}

// renderImage 渲染 cells 为图像
func (e *Encoder) renderImage(cells []Cell) *image.RGBA {
	imageSize := e.gridSize * e.cellSize
	img := image.NewRGBA(image.Rect(0, 0, imageSize, imageSize))

	// 填充背景色（黑色）
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)

	// 绘制每个 cell
	for i, cell := range cells {
		x := (i % e.gridSize) * e.cellSize
		y := (i / e.gridSize) * e.cellSize

		e.drawCell(img, x, y, cell)
	}

	return img
}

// drawCell 绘制单个 cell
func (e *Encoder) drawCell(img *image.RGBA, x, y int, cell Cell) {
	// 获取符号模板（8x8）
	template, err := e.symbolRecognizer.GetTemplate(cell.Shape)
	if err != nil {
		// 如果获取失败，绘制一个纯色方块
		c := e.colorRecognizer.GetColor(cell.Color)
		for dy := 0; dy < e.cellSize; dy++ {
			for dx := 0; dx < e.cellSize; dx++ {
				img.SetRGBA(x+dx, y+dy, c)
			}
		}
		return
	}

	// 获取颜色
	cellColor := e.colorRecognizer.GetColor(cell.Color)

	// 缩放并绘制符号
	for dy := 0; dy < e.cellSize; dy++ {
		for dx := 0; dx < e.cellSize; dx++ {
			// 从 8x8 模板映射到 cellSize x cellSize
			srcX := dx * 8 / e.cellSize
			srcY := dy * 8 / e.cellSize

			// 如果模板 hash 位是前景，使用指定颜色；否则使用黑色背景。
			var finalColor color.RGBA
			if templateForeground(template, srcX, srcY) {
				finalColor = cellColor
			} else {
				finalColor = color.RGBA{R: 0, G: 0, B: 0, A: 255}
			}

			img.SetRGBA(x+dx, y+dy, finalColor)
		}
	}
}

// Decoder 解码器
type Decoder struct {
	symbolRecognizer *symbol.Recognizer
	colorRecognizer  *colorpkg.Recognizer
	cellSize         int
	gridSize         int
}

// NewDecoder 创建解码器
func NewDecoder(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int) *Decoder {
	return &Decoder{
		symbolRecognizer: symbolRecognizer,
		colorRecognizer:  colorRecognizer,
		cellSize:         cellSize,
		gridSize:         gridSize,
	}
}

// Decode 解码二维码图像
// img: 二维码图像
// 返回：解码后的字节数组
func (d *Decoder) Decode(img image.Image) ([]byte, error) {
	// 1. 提取所有 cells
	cells, err := d.extractCells(img)
	if err != nil {
		return nil, err
	}

	// 2. 将 cells 转换为字节数组
	data := d.cellsToBytes(cells)

	return data, nil
}

// extractCells 从图像提取 cells
func (d *Decoder) extractCells(img image.Image) ([]Cell, error) {
	numCells := d.gridSize * d.gridSize
	cells := make([]Cell, numCells)

	for i := 0; i < numCells; i++ {
		x := (i % d.gridSize) * d.cellSize
		y := (i / d.gridSize) * d.cellSize

		// 提取 cell 子图像
		cellImg := extractSubImage(img, x, y, d.cellSize, d.cellSize)

		// 先识别形状，再只从形状前景像素采样颜色，避免黑色背景稀释颜色。
		shapeID, _ := d.symbolRecognizer.Recognize(cellImg)
		colorID := d.recognizeCellColor(cellImg, shapeID)

		cells[i] = Cell{
			Color: colorID,
			Shape: shapeID,
		}
	}

	return cells, nil
}

func (d *Decoder) recognizeCellColor(cellImg image.Image, shapeID symbol.SymbolID) colorpkg.ColorID {
	template, err := d.symbolRecognizer.GetTemplate(shapeID)
	if err != nil {
		colorID, _ := d.colorRecognizer.Recognize(cellImg)
		return colorID
	}

	bounds := cellImg.Bounds()
	var sumR, sumG, sumB uint64
	count := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			tx := (x - bounds.Min.X) * 8 / bounds.Dx()
			ty := (y - bounds.Min.Y) * 8 / bounds.Dy()
			if !templateForeground(template, tx, ty) {
				continue
			}

			r, g, b, _ := cellImg.At(x, y).RGBA()
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
			count++
		}
	}

	if count == 0 {
		colorID, _ := d.colorRecognizer.Recognize(cellImg)
		return colorID
	}

	avg := color.RGBA{
		R: uint8(sumR / uint64(count)),
		G: uint8(sumG / uint64(count)),
		B: uint8(sumB / uint64(count)),
		A: 255,
	}
	colorID, _ := d.colorRecognizer.RecognizeColor(avg)
	return colorID
}

func templateForeground(template *image.Gray, x, y int) bool {
	return template.GrayAt(x, y).Y > templateAverageGray(template)
}

func templateAverageGray(template *image.Gray) uint8 {
	bounds := template.Bounds()
	var sum uint64
	count := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			sum += uint64(template.GrayAt(x, y).Y)
			count++
		}
	}
	if count == 0 {
		return 128
	}
	return uint8(sum / uint64(count))
}

// cellsToBytes 将 cells 转换为字节数组
func (d *Decoder) cellsToBytes(cells []Cell) []byte {
	// 计算总位数
	totalBits := len(cells) * CellBits
	numBytes := (totalBits + 7) / 8

	data := make([]byte, numBytes)

	bitIndex := 0
	for _, cell := range cells {
		// 组合颜色和形状为 6 bits
		// 高 2 bits = 颜色，低 4 bits = 形状
		bits := (uint8(cell.Color) << ShapeBits) | uint8(cell.Shape)

		// 写入 6 bits
		for j := 0; j < CellBits && bitIndex < len(data)*8; j++ {
			bit := (bits >> (CellBits - 1 - j)) & 1

			byteIndex := bitIndex / 8
			bitOffset := 7 - (bitIndex % 8)

			if bit == 1 {
				data[byteIndex] |= (1 << bitOffset)
			}

			bitIndex++
		}
	}

	return data
}

// extractSubImage 提取子图像
func extractSubImage(img image.Image, x, y, width, height int) image.Image {
	subImg := image.NewRGBA(image.Rect(0, 0, width, height))

	for dy := 0; dy < height; dy++ {
		for dx := 0; dx < width; dx++ {
			c := img.At(x+dx, y+dy)
			subImg.Set(dx, dy, c)
		}
	}

	return subImg
}
