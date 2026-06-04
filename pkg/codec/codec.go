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

// CellBits 每个 cell 的默认总位数
// 4 色 (2 bits) + 16 形状 (4 bits) = 6 bits
const CellBits = 6

// ColorBits 默认颜色位数 (4 色 = 2 bits)
const ColorBits = 2

// ShapeBits 形状位数 (16 种形状 = 4 bits)
const ShapeBits = 4

// Encoder 编码器
type Encoder struct {
	symbolRecognizer *symbol.Recognizer
	colorRecognizer  *colorpkg.Recognizer
	cellSize         int // cell 的屏幕像素大小 (4 * B)
	gridSize         int // Q 参数，grid 的大小
	colorBits        int
	cellBits         int
	colorMask        uint8
	templateMasks    [symbol.NumSymbols][64]bool
	templatesReady   [symbol.NumSymbols]bool
	tileCache        [][]byte
	tileCacheBGRA    [][]byte
}

// NewEncoder 创建编码器
// symbolRecognizer: 符号识别器（用于获取符号模板）
// colorRecognizer: 颜色识别器（用于获取颜色）
// cellSize: cell 的屏幕像素大小（例如 8 表示 8x8 像素）
// gridSize: grid 大小（例如 50 表示 50x50 cells）
func NewEncoder(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int) *Encoder {
	enc, err := NewEncoderWithColorBits(symbolRecognizer, colorRecognizer, cellSize, gridSize, ColorBits)
	if err != nil {
		panic(err)
	}
	return enc
}

func NewEncoderWithColorBits(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int, colorBits int) (*Encoder, error) {
	if err := validateColorBits(colorRecognizer, colorBits); err != nil {
		return nil, err
	}
	e := &Encoder{
		symbolRecognizer: symbolRecognizer,
		colorRecognizer:  colorRecognizer,
		cellSize:         cellSize,
		gridSize:         gridSize,
		colorBits:        colorBits,
		cellBits:         ShapeBits + colorBits,
		colorMask:        uint8((1 << colorBits) - 1),
	}
	e.templateMasks, e.templatesReady = buildTemplateMasks(symbolRecognizer)
	e.buildTileCache()
	return e, nil
}

// Encode 将数据编码为二维码图像
// data: 待编码的字节数组
// 返回：二维码图像
func (e *Encoder) Encode(data []byte) (*image.RGBA, error) {
	numCells, err := e.encodedCellCount(data)
	if err != nil {
		return nil, err
	}

	return e.renderData(data, numCells), nil
}

func (e *Encoder) EncodeBGRA(data []byte, dst []byte) ([]byte, error) {
	numCells, err := e.encodedCellCount(data)
	if err != nil {
		return nil, err
	}
	return e.renderDataBGRA(data, numCells, dst), nil
}

func (e *Encoder) encodedCellCount(data []byte) (int, error) {
	totalBits := len(data) * 8
	numCells := (totalBits + e.cellBits - 1) / e.cellBits

	maxCells := e.gridSize * e.gridSize
	if numCells > maxCells {
		return 0, fmt.Errorf("data too large: need %d cells but grid only has %d cells", numCells, maxCells)
	}
	return numCells, nil
}

// bytesToCells 将字节数组转换为 cells
func (e *Encoder) bytesToCells(data []byte) ([]Cell, error) {
	// 计算需要多少个 cells
	totalBits := len(data) * 8
	numCells := (totalBits + e.cellBits - 1) / e.cellBits

	// 检查是否超出 grid 容量
	maxCells := e.gridSize * e.gridSize
	if numCells > maxCells {
		return nil, fmt.Errorf("data too large: need %d cells but grid only has %d cells", numCells, maxCells)
	}

	cells := make([]Cell, numCells)

	bitIndex := 0
	for i := 0; i < numCells; i++ {
		// 读取一个 cell 的 bits（从左到右）
		var bits uint8 = 0
		bitsRead := 0

		for j := 0; j < e.cellBits && bitIndex < len(data)*8; j++ {
			byteIndex := bitIndex / 8
			bitOffset := 7 - (bitIndex % 8)

			bit := (data[byteIndex] >> bitOffset) & 1
			bits = (bits << 1) | bit

			bitIndex++
			bitsRead++
		}

		if bitsRead < e.cellBits {
			bits <<= (e.cellBits - bitsRead)
		}

		colorID := colorpkg.ColorID((bits >> ShapeBits) & e.colorMask)
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

func (e *Encoder) renderData(data []byte, numCells int) *image.RGBA {
	imageSize := e.gridSize * e.cellSize
	img := image.NewRGBA(image.Rect(0, 0, imageSize, imageSize))
	maxCells := e.gridSize * e.gridSize
	if numCells < maxCells {
		fillBlackAlpha(img.Pix)
	}

	for i := 0; i < numCells; i++ {
		bits := readCellBits(data, i, e.cellBits)
		colorID := bits >> ShapeBits
		shapeID := bits & 0x0f
		tile := e.tileCache[int(colorID)*symbol.NumSymbols+int(shapeID)]
		cellX := i % e.gridSize
		cellY := i / e.gridSize
		copyTileRGBA(img.Pix, img.Stride, cellX*e.cellSize*4, cellY*e.cellSize, e.cellSize, tile)
	}

	return img
}

func (e *Encoder) renderDataBGRA(data []byte, numCells int, dst []byte) []byte {
	imageSize := e.gridSize * e.cellSize
	need := imageSize * imageSize * 4
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
	}
	maxCells := e.gridSize * e.gridSize
	if numCells < maxCells {
		clear(dst)
		fillBlackAlpha(dst)
	}

	stride := imageSize * 4
	for i := 0; i < numCells; i++ {
		bits := readCellBits(data, i, e.cellBits)
		colorID := bits >> ShapeBits
		shapeID := bits & 0x0f
		tile := e.tileCacheBGRA[int(colorID)*symbol.NumSymbols+int(shapeID)]
		cellX := i % e.gridSize
		cellY := i / e.gridSize
		copyTileRGBA(dst, stride, cellX*e.cellSize*4, cellY*e.cellSize, e.cellSize, tile)
	}

	return dst
}

func (e *Encoder) buildTileCache() {
	colorCount := 1 << e.colorBits
	e.tileCache = make([][]byte, colorCount*symbol.NumSymbols)
	e.tileCacheBGRA = make([][]byte, colorCount*symbol.NumSymbols)
	for colorID := 0; colorID < colorCount; colorID++ {
		cellColor := e.colorRecognizer.GetColor(colorpkg.ColorID(colorID))
		for shapeID := 0; shapeID < symbol.NumSymbols; shapeID++ {
			tile := e.buildTile(color.RGBA(cellColor), symbol.SymbolID(shapeID))
			index := colorID*symbol.NumSymbols + shapeID
			e.tileCache[index] = tile
			e.tileCacheBGRA[index] = rgbaTileToBGRA(tile)
		}
	}
}

func (e *Encoder) buildTile(cellColor color.RGBA, shapeID symbol.SymbolID) []byte {
	tile := make([]byte, e.cellSize*e.cellSize*4)
	template, err := e.symbolRecognizer.GetTemplate(shapeID)
	for dy := 0; dy < e.cellSize; dy++ {
		for dx := 0; dx < e.cellSize; dx++ {
			srcX := dx * 8 / e.cellSize
			srcY := dy * 8 / e.cellSize
			foreground := true
			if err == nil {
				foreground = e.templateForeground(shapeID, template, srcX, srcY)
			}
			dst := (dy*e.cellSize + dx) * 4
			if foreground {
				tile[dst+0] = cellColor.R
				tile[dst+1] = cellColor.G
				tile[dst+2] = cellColor.B
				tile[dst+3] = cellColor.A
			} else {
				tile[dst+3] = 255
			}
		}
	}
	return tile
}

func readCellBits(data []byte, cellIndex int, cellBits int) uint8 {
	bitIndex := cellIndex * cellBits
	byteIndex := bitIndex / 8
	bitOffset := bitIndex % 8

	var v uint32
	for i := 0; i < 3; i++ {
		if byteIndex+i >= len(data) {
			break
		}
		v |= uint32(data[byteIndex+i]) << uint(16-8*i)
	}
	mask := uint32((1 << cellBits) - 1)
	return uint8((v >> uint(24-bitOffset-cellBits)) & mask)
}

func copyTileRGBA(dst []byte, stride int, xByte int, y int, cellSize int, tile []byte) {
	rowBytes := cellSize * 4
	for dy := 0; dy < cellSize; dy++ {
		dstStart := (y+dy)*stride + xByte
		srcStart := dy * rowBytes
		copy(dst[dstStart:dstStart+rowBytes], tile[srcStart:srcStart+rowBytes])
	}
}

func fillBlackAlpha(pix []byte) {
	for i := 3; i < len(pix); i += 4 {
		pix[i] = 255
	}
}

func rgbaTileToBGRA(tile []byte) []byte {
	out := make([]byte, len(tile))
	for i := 0; i < len(tile); i += 4 {
		out[i+0] = tile[i+2]
		out[i+1] = tile[i+1]
		out[i+2] = tile[i+0]
		out[i+3] = tile[i+3]
	}
	return out
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
			if e.templateForeground(cell.Shape, template, srcX, srcY) {
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
	colorBits        int
	cellBits         int
	colorMask        uint8
	templateMasks    [symbol.NumSymbols][64]bool
	templatesReady   [symbol.NumSymbols]bool
	foregroundPixels [symbol.NumSymbols][]image.Point
}

// NewDecoder 创建解码器
func NewDecoder(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int) *Decoder {
	dec, err := NewDecoderWithColorBits(symbolRecognizer, colorRecognizer, cellSize, gridSize, ColorBits)
	if err != nil {
		panic(err)
	}
	return dec
}

func NewDecoderWithColorBits(symbolRecognizer *symbol.Recognizer, colorRecognizer *colorpkg.Recognizer, cellSize, gridSize int, colorBits int) (*Decoder, error) {
	if err := validateColorBits(colorRecognizer, colorBits); err != nil {
		return nil, err
	}
	d := &Decoder{
		symbolRecognizer: symbolRecognizer,
		colorRecognizer:  colorRecognizer,
		cellSize:         cellSize,
		gridSize:         gridSize,
		colorBits:        colorBits,
		cellBits:         ShapeBits + colorBits,
		colorMask:        uint8((1 << colorBits) - 1),
	}
	d.templateMasks, d.templatesReady = buildTemplateMasks(symbolRecognizer)
	d.buildForegroundPixels()
	return d, nil
}

// Decode 解码二维码图像
// img: 二维码图像
// 返回：解码后的字节数组
func (d *Decoder) Decode(img image.Image) ([]byte, error) {
	return d.DecodeInto(img, nil)
}

func (d *Decoder) DecodeInto(img image.Image, dst []byte) ([]byte, error) {
	numCells := d.gridSize * d.gridSize
	totalBits := numCells * d.cellBits
	numBytes := (totalBits + 7) / 8
	if cap(dst) < numBytes {
		dst = make([]byte, numBytes)
	} else {
		dst = dst[:numBytes]
		clear(dst)
	}

	bounds := img.Bounds()
	requiredW := d.gridSize * d.cellSize
	requiredH := d.gridSize * d.cellSize
	if bounds.Dx() < requiredW || bounds.Dy() < requiredH {
		return nil, fmt.Errorf("image too small: got %dx%d, need at least %dx%d", bounds.Dx(), bounds.Dy(), requiredW, requiredH)
	}

	if rgba, ok := img.(*image.RGBA); ok {
		d.decodeRGBAInto(rgba, bounds, numCells, dst)
		return dst, nil
	}

	for i := 0; i < numCells; i++ {
		x := bounds.Min.X + (i%d.gridSize)*d.cellSize
		y := bounds.Min.Y + (i/d.gridSize)*d.cellSize

		hash := d.cellHash(img, x, y)
		shapeID, _ := d.symbolRecognizer.RecognizeHash(hash)
		colorID := d.recognizeCellColorAt(img, x, y, shapeID)
		writeCellBits(dst, i, d.cellBits, (uint8(colorID&colorpkg.ColorID(d.colorMask))<<ShapeBits)|uint8(shapeID))
	}

	return dst, nil
}

func (d *Decoder) DecodeBGRAInto(pix []byte, width int, height int, stride int, dst []byte) ([]byte, error) {
	requiredW := d.gridSize * d.cellSize
	requiredH := d.gridSize * d.cellSize
	if width < requiredW || height < requiredH {
		return nil, fmt.Errorf("image too small: got %dx%d, need at least %dx%d", width, height, requiredW, requiredH)
	}
	if stride < width*4 {
		return nil, fmt.Errorf("invalid stride: got %d, need at least %d", stride, width*4)
	}
	if len(pix) < stride*height {
		return nil, fmt.Errorf("pixel buffer too short: got %d, need %d", len(pix), stride*height)
	}

	numCells := d.gridSize * d.gridSize
	totalBits := numCells * d.cellBits
	numBytes := (totalBits + 7) / 8
	if cap(dst) < numBytes {
		dst = make([]byte, numBytes)
	} else {
		dst = dst[:numBytes]
		clear(dst)
	}

	d.decodeBGRAInto(pix, stride, numCells, dst)
	return dst, nil
}

func (d *Decoder) decodeRGBAInto(img *image.RGBA, bounds image.Rectangle, numCells int, dst []byte) {
	for i := 0; i < numCells; i++ {
		x := bounds.Min.X + (i%d.gridSize)*d.cellSize
		y := bounds.Min.Y + (i/d.gridSize)*d.cellSize

		hash := d.cellHashRGBA(img, x, y)
		shapeID, _ := d.symbolRecognizer.RecognizeHash(hash)
		colorID := d.recognizeCellColorRGBA(img, x, y, shapeID)
		writeCellBits(dst, i, d.cellBits, (uint8(colorID&colorpkg.ColorID(d.colorMask))<<ShapeBits)|uint8(shapeID))
	}
}

func (d *Decoder) decodeBGRAInto(pix []byte, stride int, numCells int, dst []byte) {
	for i := 0; i < numCells; i++ {
		x := (i % d.gridSize) * d.cellSize
		y := (i / d.gridSize) * d.cellSize

		hash := d.cellHashBGRA(pix, stride, x, y)
		shapeID, _ := d.symbolRecognizer.RecognizeHash(hash)
		colorID := d.recognizeCellColorBGRA(pix, stride, x, y, shapeID)
		writeCellBits(dst, i, d.cellBits, (uint8(colorID&colorpkg.ColorID(d.colorMask))<<ShapeBits)|uint8(shapeID))
	}
}

func writeCellBits(dst []byte, cellIndex int, cellBits int, bits uint8) {
	bitIndex := cellIndex * cellBits
	for j := 0; j < cellBits && bitIndex < len(dst)*8; j++ {
		bit := (bits >> (cellBits - 1 - j)) & 1
		if bit != 0 {
			byteIndex := bitIndex / 8
			bitOffset := 7 - (bitIndex % 8)
			dst[byteIndex] |= 1 << bitOffset
		}
		bitIndex++
	}
}

func (d *Decoder) cellHashBGRA(pix []byte, stride int, x int, y int) uint64 {
	var samples [64]uint8
	var sum uint64

	for ty := 0; ty < 8; ty++ {
		for tx := 0; tx < 8; tx++ {
			sx := x + tx*d.cellSize/8
			sy := y + ty*d.cellSize/8
			offset := sy*stride + sx*4
			b := pix[offset]
			g := pix[offset+1]
			r := pix[offset+2]
			gray := uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000)
			samples[ty*8+tx] = gray
			sum += uint64(gray)
		}
	}

	threshold := uint8(sum / 64)
	var hash uint64
	for _, gray := range samples {
		bit := uint64(0)
		if gray > threshold {
			bit = 1
		}
		hash = (hash << 1) | bit
	}
	return hash
}

func (d *Decoder) recognizeCellColorBGRA(pix []byte, stride int, x int, y int, shapeID symbol.SymbolID) colorpkg.ColorID {
	if shapeID >= symbol.NumSymbols || len(d.foregroundPixels[shapeID]) == 0 {
		return colorpkg.ColorID(0)
	}

	var sumR, sumG, sumB uint64
	for _, p := range d.foregroundPixels[shapeID] {
		offset := (y+p.Y)*stride + (x+p.X)*4
		sumB += uint64(pix[offset])
		sumG += uint64(pix[offset+1])
		sumR += uint64(pix[offset+2])
	}
	count := uint64(len(d.foregroundPixels[shapeID]))
	avg := color.RGBA{
		R: uint8(sumR / count),
		G: uint8(sumG / count),
		B: uint8(sumB / count),
		A: 255,
	}
	colorID, _ := d.colorRecognizer.RecognizeColorRGB(avg)
	return colorID
}

func (d *Decoder) decodeViaCells(img image.Image) ([]byte, error) {
	cells, err := d.extractCells(img)
	if err != nil {
		return nil, err
	}

	data := d.cellsToBytes(cells)
	return data, nil
}

// extractCells 从图像提取 cells
func (d *Decoder) extractCells(img image.Image) ([]Cell, error) {
	numCells := d.gridSize * d.gridSize
	cells := make([]Cell, numCells)

	bounds := img.Bounds()
	requiredW := d.gridSize * d.cellSize
	requiredH := d.gridSize * d.cellSize
	if bounds.Dx() < requiredW || bounds.Dy() < requiredH {
		return nil, fmt.Errorf("image too small: got %dx%d, need at least %dx%d", bounds.Dx(), bounds.Dy(), requiredW, requiredH)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return d.extractCellsRGBA(rgba, bounds, numCells), nil
	}

	for i := 0; i < numCells; i++ {
		x := (i % d.gridSize) * d.cellSize
		y := (i / d.gridSize) * d.cellSize

		// 直接从原图采样 8x8 hash，避免为每个 cell 分配子图。
		hash := d.cellHash(img, bounds.Min.X+x, bounds.Min.Y+y)
		shapeID, _ := d.symbolRecognizer.RecognizeHash(hash)
		colorID := d.recognizeCellColorAt(img, bounds.Min.X+x, bounds.Min.Y+y, shapeID)

		cells[i] = Cell{
			Color: colorID,
			Shape: shapeID,
		}
	}

	return cells, nil
}

func (d *Decoder) extractCellsRGBA(img *image.RGBA, bounds image.Rectangle, numCells int) []Cell {
	cells := make([]Cell, numCells)

	for i := 0; i < numCells; i++ {
		x := bounds.Min.X + (i%d.gridSize)*d.cellSize
		y := bounds.Min.Y + (i/d.gridSize)*d.cellSize

		hash := d.cellHashRGBA(img, x, y)
		shapeID, _ := d.symbolRecognizer.RecognizeHash(hash)
		colorID := d.recognizeCellColorRGBA(img, x, y, shapeID)

		cells[i] = Cell{
			Color: colorID,
			Shape: shapeID,
		}
	}

	return cells
}

func (d *Decoder) buildForegroundPixels() {
	for shapeID := symbol.SymbolID(0); shapeID < symbol.NumSymbols; shapeID++ {
		if !d.templatesReady[shapeID] {
			continue
		}
		pixels := make([]image.Point, 0, d.cellSize*d.cellSize/2)
		for dy := 0; dy < d.cellSize; dy++ {
			for dx := 0; dx < d.cellSize; dx++ {
				tx := dx * 8 / d.cellSize
				ty := dy * 8 / d.cellSize
				if d.templateMasks[shapeID][ty*8+tx] {
					pixels = append(pixels, image.Point{X: dx, Y: dy})
				}
			}
		}
		d.foregroundPixels[shapeID] = pixels
	}
}

func (d *Decoder) cellHashRGBA(img *image.RGBA, x, y int) uint64 {
	var samples [64]uint8
	var sum uint64

	for ty := 0; ty < 8; ty++ {
		for tx := 0; tx < 8; tx++ {
			sx := x + tx*d.cellSize/8
			sy := y + ty*d.cellSize/8
			pix := img.PixOffset(sx, sy)
			r := img.Pix[pix]
			g := img.Pix[pix+1]
			b := img.Pix[pix+2]
			gray := uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000)
			samples[ty*8+tx] = gray
			sum += uint64(gray)
		}
	}

	threshold := uint8(sum / 64)
	var hash uint64
	for _, gray := range samples {
		bit := uint64(0)
		if gray > threshold {
			bit = 1
		}
		hash = (hash << 1) | bit
	}
	return hash
}

func (d *Decoder) recognizeCellColorRGBA(img *image.RGBA, x, y int, shapeID symbol.SymbolID) colorpkg.ColorID {
	if shapeID >= symbol.NumSymbols || len(d.foregroundPixels[shapeID]) == 0 {
		return d.recognizeCellColorAt(img, x, y, shapeID)
	}

	var sumR, sumG, sumB uint64
	for _, p := range d.foregroundPixels[shapeID] {
		pix := img.PixOffset(x+p.X, y+p.Y)
		sumR += uint64(img.Pix[pix])
		sumG += uint64(img.Pix[pix+1])
		sumB += uint64(img.Pix[pix+2])
	}
	count := uint64(len(d.foregroundPixels[shapeID]))
	avg := color.RGBA{
		R: uint8(sumR / count),
		G: uint8(sumG / count),
		B: uint8(sumB / count),
		A: 255,
	}
	colorID, _ := d.colorRecognizer.RecognizeColorRGB(avg)
	return colorID
}

func (d *Decoder) cellHash(img image.Image, x, y int) uint64 {
	var samples [64]uint8
	var sum uint64

	for ty := 0; ty < 8; ty++ {
		for tx := 0; tx < 8; tx++ {
			sx := x + tx*d.cellSize/8
			sy := y + ty*d.cellSize/8
			gray := fastGrayAt(img, sx, sy)
			samples[ty*8+tx] = gray
			sum += uint64(gray)
		}
	}

	threshold := uint8(sum / 64)
	var hash uint64
	for _, gray := range samples {
		bit := uint64(0)
		if gray > threshold {
			bit = 1
		}
		hash = (hash << 1) | bit
	}
	return hash
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
			if !d.templateForeground(shapeID, template, tx, ty) {
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
	colorID, _ := d.colorRecognizer.RecognizeColorRGB(avg)
	return colorID
}

func (d *Decoder) recognizeCellColorAt(img image.Image, x, y int, shapeID symbol.SymbolID) colorpkg.ColorID {
	template, err := d.symbolRecognizer.GetTemplate(shapeID)
	if err != nil {
		colorID, _ := d.colorRecognizer.Recognize(extractSubImage(img, x, y, d.cellSize, d.cellSize))
		return colorID
	}

	var sumR, sumG, sumB uint64
	count := 0

	for dy := 0; dy < d.cellSize; dy++ {
		for dx := 0; dx < d.cellSize; dx++ {
			tx := dx * 8 / d.cellSize
			ty := dy * 8 / d.cellSize
			if !d.templateForeground(shapeID, template, tx, ty) {
				continue
			}

			r, g, b := fastRGBAt(img, x+dx, y+dy)
			sumR += uint64(r)
			sumG += uint64(g)
			sumB += uint64(b)
			count++
		}
	}

	if count == 0 {
		colorID, _ := d.colorRecognizer.Recognize(extractSubImage(img, x, y, d.cellSize, d.cellSize))
		return colorID
	}

	avg := color.RGBA{
		R: uint8(sumR / uint64(count)),
		G: uint8(sumG / uint64(count)),
		B: uint8(sumB / uint64(count)),
		A: 255,
	}
	colorID, _ := d.colorRecognizer.RecognizeColorRGB(avg)
	return colorID
}

func (e *Encoder) templateForeground(shapeID symbol.SymbolID, template *image.Gray, x, y int) bool {
	if shapeID < symbol.NumSymbols && e.templatesReady[shapeID] {
		return e.templateMasks[shapeID][y*8+x]
	}
	return templateForeground(template, x, y)
}

func (d *Decoder) templateForeground(shapeID symbol.SymbolID, template *image.Gray, x, y int) bool {
	if shapeID < symbol.NumSymbols && d.templatesReady[shapeID] {
		return d.templateMasks[shapeID][y*8+x]
	}
	return templateForeground(template, x, y)
}

func buildTemplateMasks(rec *symbol.Recognizer) ([symbol.NumSymbols][64]bool, [symbol.NumSymbols]bool) {
	var masks [symbol.NumSymbols][64]bool
	var ready [symbol.NumSymbols]bool

	for id := symbol.SymbolID(0); id < symbol.NumSymbols; id++ {
		template, err := rec.GetTemplate(id)
		if err != nil {
			continue
		}
		avg := templateAverageGray(template)
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				masks[id][y*8+x] = template.GrayAt(x, y).Y > avg
			}
		}
		ready[id] = true
	}

	return masks, ready
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

func fastGrayAt(img image.Image, x, y int) uint8 {
	r, g, b := fastRGBAt(img, x, y)
	return uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000)
}

func fastRGBAt(img image.Image, x, y int) (uint8, uint8, uint8) {
	switch src := img.(type) {
	case *image.RGBA:
		c := src.RGBAAt(x, y)
		return c.R, c.G, c.B
	case *image.NRGBA:
		c := src.NRGBAAt(x, y)
		if c.A == 255 {
			return c.R, c.G, c.B
		}
		if c.A == 0 {
			return 0, 0, 0
		}
		a := uint32(c.A)
		return uint8(uint32(c.R) * a / 255), uint8(uint32(c.G) * a / 255), uint8(uint32(c.B) * a / 255)
	default:
		r, g, b, _ := img.At(x, y).RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
}

// cellsToBytes 将 cells 转换为字节数组
func (d *Decoder) cellsToBytes(cells []Cell) []byte {
	// 计算总位数
	totalBits := len(cells) * d.cellBits
	numBytes := (totalBits + 7) / 8

	data := make([]byte, numBytes)

	bitIndex := 0
	for _, cell := range cells {
		bits := (uint8(cell.Color&colorpkg.ColorID(d.colorMask)) << ShapeBits) | uint8(cell.Shape)

		for j := 0; j < d.cellBits && bitIndex < len(data)*8; j++ {
			bit := (bits >> (d.cellBits - 1 - j)) & 1

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

func validateColorBits(colorRecognizer *colorpkg.Recognizer, colorBits int) error {
	if colorBits != 2 && colorBits != 4 {
		return fmt.Errorf("color bits must be 2 or 4, got %d", colorBits)
	}
	if colorRecognizer == nil {
		return fmt.Errorf("color recognizer is nil")
	}
	needColors := 1 << colorBits
	if colorRecognizer.NumColors() < needColors {
		return fmt.Errorf("color recognizer has %d colors, need %d for %d color bits", colorRecognizer.NumColors(), needColors, colorBits)
	}
	return nil
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
