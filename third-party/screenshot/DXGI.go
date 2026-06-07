package screenshot

import (
	"fmt"
	"github.com/ghp3000/screenshot/d3d"
	"image"
	"sync/atomic"
	"unsafe"
)

type DXGIScreenshot struct {
	rect      image.Rectangle
	device    *d3d.ID3D11Device
	deviceCtx *d3d.ID3D11DeviceContext
	ddup      *d3d.OutputDuplicator
	display   int //截取哪个屏幕
	cursor    int32
	bgra      *image.RGBA
}

func NewDXGIScreenshot() *DXGIScreenshot {
	return &DXGIScreenshot{}
}

func (s *DXGIScreenshot) Init(display int) error {
	if display < 0 {
		return fmt.Errorf("display %d is invalid", display)
	}
	s.Release()
	s.display = display
	s.rect = GetDisplayBounds(display)
	var err error
	s.device, s.deviceCtx, err = d3d.NewD3D11Device()
	if err != nil {
		return err
	}
	s.ddup, err = d3d.NewIDXGIOutputDuplication(s.device, s.deviceCtx, uint(s.display))
	if err != nil {
		return err
	}

	return nil
}

func (s *DXGIScreenshot) Capture() (*image.RGBA, error) {
	if s.ddup == nil {
		return nil, fmt.Errorf("no init,please run Init first")
	}
	rect := GetDisplayBounds(s.display)
	// 如果发现屏幕范围发生了变化就重新初始化
	if rect != s.rect {
		s.Release()
		if err := s.Init(s.display); err != nil {
			return nil, err
		}
	}

	imgBuf := image.NewRGBA(s.rect)

	err := s.ddup.GetImage(imgBuf, 0, atomic.LoadInt32(&s.cursor) == 1)
	if err != nil {
		return nil, err
	}
	return imgBuf, err
}

func (s *DXGIScreenshot) CaptureBGRA() (*image.RGBA, error) {
	if s.ddup == nil {
		return nil, fmt.Errorf("no init,please run Init first")
	}
	rect := GetDisplayBounds(s.display)
	// 如果发现屏幕范围发生了变化就重新初始化
	if rect != s.rect {
		s.Release()
		if err := s.Init(s.display); err != nil {
			return nil, err
		}
	}

	unmap, mappedRect, size, err := s.ddup.Snapshot2(0)
	if err != nil {
		return nil, err
	}
	defer unmap()

	width := int(size.X)
	height := int(size.Y)
	if s.bgra == nil || s.bgra.Bounds().Dx() != width || s.bgra.Bounds().Dy() != height {
		s.bgra = image.NewRGBA(image.Rect(0, 0, width, height))
	}
	img := s.bgra
	rowBytes := width * 4
	srcPitch := int(mappedRect.Pitch)
	if srcPitch == rowBytes {
		src := unsafe.Slice((*byte)(unsafe.Pointer(mappedRect.PBits)), rowBytes*height)
		copy(img.Pix, src)
		return img, nil
	}
	for y := 0; y < height; y++ {
		src := unsafe.Slice((*byte)(unsafe.Pointer(mappedRect.PBits+uintptr(y*srcPitch))), rowBytes)
		copy(img.Pix[y*img.Stride:y*img.Stride+rowBytes], src)
	}
	return img, nil
}

func (s *DXGIScreenshot) Release() {
	if s.device != nil {
		s.device.Release()
	}
	if s.deviceCtx != nil {
		s.deviceCtx.Release()
	}
	if s.ddup != nil {
		s.ddup.Release()
	}
}

func (s *DXGIScreenshot) GetDisplayId() int {
	return s.display
}
func (s *DXGIScreenshot) GetCaptureName() string {
	return "DXGI"
}
func (s *DXGIScreenshot) DrawCursor(cursor int32) {
	atomic.StoreInt32(&s.cursor, cursor)
}
