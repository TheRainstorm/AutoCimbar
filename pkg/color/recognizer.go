package color

import (
	"image"
	"image/color"
	"math"
)

// ColorID 颜色标识符
type ColorID uint8

const (
	// 4 色模式 (2 bits)
	ColorGreen ColorID = 0
	ColorWhite ColorID = 1
	ColorRed   ColorID = 2
	ColorBlue  ColorID = 3

	// ColorBlack is kept as a compatibility alias for older callers.
	ColorBlack ColorID = ColorGreen

	// NumColors2 2 色模式的颜色数量
	NumColors2 = 2
	// NumColors4 4 色模式的颜色数量
	NumColors4 = 4
	// NumColors8 8 色模式的颜色数量
	NumColors8 = 8
	// NumColors16 16 色模式的颜色数量
	NumColors16 = 16
)

// Color2Palette 2 色调色板。使用 4 色模式的前 2 项，便于低误码链路测试。
var Color2Palette = []color.RGBA{
	{R: 0, G: 255, B: 0, A: 255},     // 绿色
	{R: 255, G: 255, B: 255, A: 255}, // 白色
}

// Color4Palette 4 色调色板（高对比度）
var Color4Palette = []color.RGBA{
	{R: 0, G: 255, B: 0, A: 255},     // 绿色
	{R: 255, G: 255, B: 255, A: 255}, // 白色
	{R: 255, G: 0, B: 0, A: 255},     // 红色
	{R: 0, G: 0, B: 255, A: 255},     // 蓝色
}

// Color8Palette 8 色调色板。前 4 项保持 4 色模式的 ID 顺序。
var Color8Palette = []color.RGBA{
	{R: 0, G: 255, B: 0, A: 255},
	{R: 255, G: 255, B: 255, A: 255},
	{R: 255, G: 0, B: 0, A: 255},
	{R: 0, G: 0, B: 255, A: 255},
	{R: 255, G: 255, B: 0, A: 255},
	{R: 255, G: 0, B: 255, A: 255},
	{R: 0, G: 255, B: 255, A: 255},
	{R: 255, G: 96, B: 0, A: 255},
}

// Color16Palette 16 色调色板。前 4 项保持 4 色模式的 ID 顺序。
var Color16Palette = []color.RGBA{
	{R: 0, G: 255, B: 0, A: 255},
	{R: 255, G: 255, B: 255, A: 255},
	{R: 255, G: 0, B: 0, A: 255},
	{R: 0, G: 0, B: 255, A: 255},
	{R: 255, G: 255, B: 0, A: 255},
	{R: 255, G: 0, B: 255, A: 255},
	{R: 0, G: 255, B: 255, A: 255},
	{R: 255, G: 96, B: 160, A: 255},
	{R: 96, G: 160, B: 255, A: 255},
	{R: 96, G: 255, B: 160, A: 255},
	{R: 0, G: 96, B: 255, A: 255},
	{R: 255, G: 96, B: 0, A: 255},
	{R: 160, G: 96, B: 255, A: 255},
	{R: 255, G: 160, B: 96, A: 255},
	{R: 255, G: 160, B: 255, A: 255},
	{R: 160, G: 255, B: 96, A: 255},
}

// Recognizer 颜色识别器
type Recognizer struct {
	// 参考颜色（LAB 色彩空间）
	referenceLAB []LABColor
	// 原始 RGB 颜色
	referenceRGB []color.RGBA
	// 近似视频编码色彩空间，用于解码热路径的快速颜色匹配。
	referenceYCbCr []YCbCrColor
}

// LABColor LAB 色彩空间颜色
// LAB 比 RGB 更接近人眼感知，距离计算更准确
type LABColor struct {
	L, A, B float64
}

type YCbCrColor struct {
	Y, Cb, Cr int32
}

func NewRecognizer2Color() *Recognizer {
	return NewRecognizer(Color2Palette)
}

// NewRecognizer4Color 创建 4 色识别器
func NewRecognizer4Color() *Recognizer {
	return NewRecognizer(Color4Palette)
}

func NewRecognizer8Color() *Recognizer {
	return NewRecognizer(Color8Palette)
}

func NewRecognizer16Color() *Recognizer {
	return NewRecognizer(Color16Palette)
}

func NewRecognizer(palette []color.RGBA) *Recognizer {
	referenceRGB := append([]color.RGBA(nil), palette...)
	r := &Recognizer{
		referenceRGB:   referenceRGB,
		referenceLAB:   make([]LABColor, len(referenceRGB)),
		referenceYCbCr: make([]YCbCrColor, len(referenceRGB)),
	}

	for i, c := range referenceRGB {
		r.referenceLAB[i] = RGBToLAB(c)
		r.referenceYCbCr[i] = RGBToYCbCr(c)
	}

	return r
}

func (r *Recognizer) NumColors() int {
	return len(r.referenceRGB)
}

// Recognize 识别颜色
// cellImg: cell 图像
// 返回：最匹配的颜色 ID 和距离
func (r *Recognizer) Recognize(cellImg image.Image) (ColorID, float64) {
	// 计算平均颜色
	avgColor := computeAverageColor(cellImg)

	return r.RecognizeColor(avgColor)
}

// RecognizeColor 识别单个 RGB 颜色。
func (r *Recognizer) RecognizeColor(c color.RGBA) (ColorID, float64) {
	// 转换到 LAB 空间
	avgLAB := RGBToLAB(c)

	// 与参考颜色比较
	minDist := math.MaxFloat64
	bestID := ColorID(0)

	for i, refLAB := range r.referenceLAB {
		dist := colorDistance(avgLAB, refLAB)
		if dist < minDist {
			minDist = dist
			bestID = ColorID(i)
		}
	}

	return bestID, minDist
}

func (r *Recognizer) RecognizeColorRGB(c color.RGBA) (ColorID, uint32) {
	input := RGBToYCbCr(c)
	bestID := ColorID(0)
	var minDist uint32 = ^uint32(0)

	for i, ref := range r.referenceYCbCr {
		dist := ycbcrDistance(input, ref)
		if dist < minDist {
			minDist = dist
			bestID = ColorID(i)
		}
	}

	return bestID, minDist
}

func RGBToYCbCr(c color.RGBA) YCbCrColor {
	r := int32(c.R)
	g := int32(c.G)
	b := int32(c.B)
	y := (299*r + 587*g + 114*b + 500) / 1000
	cb := (565*(b-y) + signedRoundOffset(b-y, 1000)) / 1000
	cr := (713*(r-y) + signedRoundOffset(r-y, 1000)) / 1000
	return YCbCrColor{Y: y, Cb: cb, Cr: cr}
}

func ycbcrDistance(a, b YCbCrColor) uint32 {
	dy := a.Y - b.Y
	dcb := a.Cb - b.Cb
	dcr := a.Cr - b.Cr
	return uint32(dy*dy + dcb*dcb + dcr*dcr)
}

func signedRoundOffset(v int32, scale int32) int32 {
	if v < 0 {
		return -scale / 2
	}
	return scale / 2
}

// GetColor 获取颜色 ID 对应的 RGB 颜色
func (r *Recognizer) GetColor(id ColorID) color.RGBA {
	if int(id) >= len(r.referenceRGB) {
		return color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}
	return r.referenceRGB[id]
}

// computeAverageColor 计算图像的平均颜色
func computeAverageColor(img image.Image) color.RGBA {
	bounds := img.Bounds()
	var sumR, sumG, sumB uint64
	count := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA 返回 16-bit 值，转为 8-bit
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
			count++
		}
	}

	if count == 0 {
		return color.RGBA{R: 0, G: 0, B: 0, A: 255}
	}

	return color.RGBA{
		R: uint8(sumR / uint64(count)),
		G: uint8(sumG / uint64(count)),
		B: uint8(sumB / uint64(count)),
		A: 255,
	}
}

// RGBToLAB 将 RGB 转换为 LAB 色彩空间
// LAB 色彩空间更适合颜色距离计算
func RGBToLAB(c color.RGBA) LABColor {
	// 1. RGB -> XYZ
	r := float64(c.R) / 255.0
	g := float64(c.G) / 255.0
	b := float64(c.B) / 255.0

	// Gamma 校正
	if r > 0.04045 {
		r = math.Pow((r+0.055)/1.055, 2.4)
	} else {
		r = r / 12.92
	}

	if g > 0.04045 {
		g = math.Pow((g+0.055)/1.055, 2.4)
	} else {
		g = g / 12.92
	}

	if b > 0.04045 {
		b = math.Pow((b+0.055)/1.055, 2.4)
	} else {
		b = b / 12.92
	}

	// 转 XYZ（使用 D65 光源）
	x := r*0.4124564 + g*0.3575761 + b*0.1804375
	y := r*0.2126729 + g*0.7151522 + b*0.0721750
	z := r*0.0193339 + g*0.1191920 + b*0.9503041

	// 2. XYZ -> LAB
	// D65 白点
	const xn = 0.95047
	const yn = 1.00000
	const zn = 1.08883

	x = x / xn
	y = y / yn
	z = z / zn

	// f 函数
	fx := labF(x)
	fy := labF(y)
	fz := labF(z)

	// LAB 值
	l := 116.0*fy - 16.0
	a := 500.0 * (fx - fy)
	bVal := 200.0 * (fy - fz)

	return LABColor{L: l, A: a, B: bVal}
}

// labF LAB 转换的辅助函数
func labF(t float64) float64 {
	const delta = 6.0 / 29.0
	if t > delta*delta*delta {
		return math.Pow(t, 1.0/3.0)
	}
	return t/(3.0*delta*delta) + 4.0/29.0
}

// colorDistance 计算 LAB 色彩空间中的欧几里得距离
// 这比 RGB 空间的距离更接近人眼感知
func colorDistance(c1, c2 LABColor) float64 {
	dL := c1.L - c2.L
	dA := c1.A - c2.A
	dB := c1.B - c2.B
	return math.Sqrt(dL*dL + dA*dA + dB*dB)
}
