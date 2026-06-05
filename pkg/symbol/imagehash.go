package symbol

import (
	"image"
	"image/color"
	"math/bits"
)

// ImageHash 计算 8x8 图像的阈值化 hash
// 参考 libcimbar 的实现，使用简单的阈值化方法
func ImageHash(img image.Image) uint64 {
	return ImageHashWithSize(img, 8, 8)
}

// ImageHashWithSize 计算指定 tile 尺寸图像的阈值化 hash。
// 目前支持最多 64 个采样点，因此 8x8、6x6、4x4 都可以放进 uint64。
func ImageHashWithSize(img image.Image, width int, height int) uint64 {
	// 2. 计算平均灰度作为阈值
	threshold := computeAverageGray(img)

	// 3. 生成 hash
	// 每个像素大于阈值则为 1，否则为 0
	// 从左到右，从上到下
	var hash uint64
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			gray := getGrayscale(img, x, y)
			bit := uint64(0)
			if gray > threshold {
				bit = 1
			}
			hash = (hash << 1) | bit
		}
	}

	return hash
}

// HammingDistance 计算两个 64-bit hash 的汉明距离
func HammingDistance(a, b uint64) int {
	// XOR 后计算 1 的个数
	return bits.OnesCount64(a ^ b)
}

// computeAverageGray 计算图像的平均灰度值
func computeAverageGray(img image.Image) uint8 {
	bounds := img.Bounds()
	sum := uint64(0)
	count := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := getGrayscale(img, x, y)
			sum += uint64(gray)
			count++
		}
	}

	if count == 0 {
		return 128
	}
	return uint8(sum / uint64(count))
}

// getGrayscale 获取指定像素的灰度值
func getGrayscale(img image.Image, x, y int) uint8 {
	c := img.At(x, y)
	r, g, b, _ := c.RGBA()

	// RGBA 返回的是 16-bit 值，需要转为 8-bit
	// 使用标准的灰度转换公式
	gray := (299*r + 587*g + 114*b) / 1000 / 257

	return uint8(gray)
}

// ResizeToTile 将图像缩放到 8x8
// 使用最近邻插值，保持清晰边缘
func ResizeToTile(img image.Image) *image.Gray {
	return ResizeToTileWithSize(img, 8, 8)
}

// ResizeToTileWithSize 将图像缩放到指定 tile 尺寸。
func ResizeToTileWithSize(img image.Image, width int, height int) *image.Gray {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	dst := image.NewGray(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// 最近邻插值
			srcX := x * srcW / width
			srcY := y * srcH / height

			gray := getGrayscale(img, bounds.Min.X+srcX, bounds.Min.Y+srcY)
			dst.SetGray(x, y, color.Gray{Y: gray})
		}
	}

	return dst
}
