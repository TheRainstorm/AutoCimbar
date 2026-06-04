package app

import (
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/kbinani/screenshot"
)

type ScreenEncodeConfig struct {
	InputPath string
	GridSize  int
	Scale     int
	SymbolDir string
	BlockSize int
	Region    string
	FPS       int
	Addr      string
	Open      bool
	Progress  io.Writer
}

type ScreenDecodeConfig struct {
	OutputPath string
	GridSize   int
	Scale      int
	SymbolDir  string
	BlockSize  int
	Region     string
	FPS        int
	Timeout    time.Duration
	Progress   io.Writer
}

func DisplayBounds() []image.Rectangle {
	count := screenshot.NumActiveDisplays()
	displays := make([]image.Rectangle, 0, count)
	for i := 0; i < count; i++ {
		displays = append(displays, screenshot.GetDisplayBounds(i))
	}
	return displays
}

func EncodeFileToScreen(cfg ScreenEncodeConfig) (*EncodeResult, error) {
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if err := validateGridScale(cfg.GridSize, cfg.Scale); err != nil {
		return nil, err
	}

	payloadCapacity := PayloadCapacityBytes(cfg.GridSize)
	if payloadCapacity <= 0 {
		return nil, fmt.Errorf("grid Q=%d capacity is too small: frame capacity %d, frame header %d", cfg.GridSize, GridCapacityBytes(cfg.GridSize), FrameHeaderSize)
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}

	data, err := os.ReadFile(cfg.InputPath)
	if err != nil {
		return nil, fmt.Errorf("read input file: %w", err)
	}
	checksum := crc32.ChecksumIEEE(data)
	fountainEnc, err := fountain.NewEncoder(data, blockSize)
	if err != nil {
		return nil, err
	}

	symRec, err := LoadLibcimbarSymbols(cfg.SymbolDir)
	if err != nil {
		return nil, err
	}
	codecEnc := codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(cfg.Scale), cfg.GridSize)

	imageSize := cfg.GridSize * CellSize(cfg.Scale)
	rect, err := ResolveEncoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return nil, err
	}

	result := &EncodeResult{
		GridSize:        cfg.GridSize,
		Scale:           cfg.Scale,
		CellSize:        CellSize(cfg.Scale),
		ImageSize:       imageSize,
		FrameCapacity:   GridCapacityBytes(cfg.GridSize),
		PayloadCapacity: payloadCapacity,
		FileSize:        len(data),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
	}
	progress := newScreenEncoderProgress(cfg.Progress, result, checksum)
	progress.startSummary()
	defer progress.finishLine()

	source := &screenFrameSource{
		codecEnc:    codecEnc,
		fountainEnc: fountainEnc,
		fileSize:    len(data),
		width:       imageSize,
		height:      imageSize,
		progress:    progress,
	}

	if err := runScreenEncoderBackend(cfg, source, rect); err != nil {
		return result, err
	}
	return result, nil
}

func DecodeScreenToFile(cfg ScreenDecodeConfig) error {
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if err := validateGridScale(cfg.GridSize, cfg.Scale); err != nil {
		return err
	}

	payloadCapacity := PayloadCapacityBytes(cfg.GridSize)
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}

	symRec, err := LoadLibcimbarSymbols(cfg.SymbolDir)
	if err != nil {
		return err
	}
	codecDec := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(cfg.Scale), cfg.GridSize)
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1

	imageSize := cfg.GridSize * CellSize(cfg.Scale)
	rect, err := ResolveDecoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return err
	}
	capturer, err := newScreenCapturer(rect)
	if err != nil {
		return err
	}
	defer capturer.Close()

	deadline := time.Now().Add(cfg.Timeout)
	interval := time.Second / time.Duration(cfg.FPS)
	if interval <= 0 {
		interval = time.Millisecond
	}
	progress := newScreenDecoderProgress(cfg.Progress, blockSize)
	defer progress.finishLine()

	nextCapture := time.Now()
	var packetBuf []byte
	for time.Now().Before(deadline) {
		if now := time.Now(); now.Before(nextCapture) {
			time.Sleep(time.Until(nextCapture))
		} else if now.Sub(nextCapture) > interval {
			nextCapture = now
		}
		nextCapture = nextCapture.Add(interval)

		img, err := capturer.Capture()
		if err != nil {
			return fmt.Errorf("capture screen: %w", err)
		}
		progress.noteCaptured()
		packet, err := codecDec.DecodeInto(img, packetBuf)
		if err != nil {
			progress.noteInvalid()
			continue
		}
		packetBuf = packet
		frame, err := ParsePacket(packet, blockSize)
		if err != nil {
			progress.noteInvalid()
			continue
		}
		if fountainDec == nil {
			fileSize = frame.FileSize
			blockCount = blockCountForFile(fileSize, blockSize)
			fountainDec, err = fountain.NewDecoder(fileSize, blockSize, blockCount)
			if err != nil {
				return err
			}
			progress.noteStarted(fileSize, blockCount)
		} else if frame.FileSize != fileSize {
			progress.noteInvalid()
			continue
		}
		if _, err := fountainDec.AddFrame(frame.FrameID, frame.Payload); err != nil {
			return fmt.Errorf("add captured frame: %w", err)
		}
		progress.noteValid(fountainDec.Rank())
		if fountainDec.Complete() {
			result, err := fountainDec.Decode()
			if err != nil {
				return err
			}
			return os.WriteFile(cfg.OutputPath, result, 0644)
		}
	}

	rank := 0
	if fountainDec != nil {
		rank = fountainDec.Rank()
	}
	return fmt.Errorf("timeout waiting for enough frames: rank %d of %d", rank, blockCount)
}

type screenFrameSource struct {
	codecEnc    *codec.Encoder
	fountainEnc *fountain.Encoder
	fileSize    int
	frameID     uint32
	width       int
	height      int
	progress    *screenEncoderProgress
	blockBuf    []byte
	packetBuf   []byte
	mu          sync.Mutex
}

func (s *screenFrameSource) NextImage() (*image.RGBA, error) {
	s.mu.Lock()
	packet := append([]byte(nil), s.nextPacketLocked()...)
	s.mu.Unlock()

	img, err := s.codecEnc.Encode(packet)
	if err != nil {
		return nil, err
	}
	s.progress.noteEncoded()
	return img, nil
}

func (s *screenFrameSource) NextBGRA(dst []byte) ([]byte, error) {
	s.mu.Lock()
	packet := s.nextPacketLocked()
	pixels, err := s.codecEnc.EncodeBGRA(packet, dst)
	s.mu.Unlock()

	if err != nil {
		return nil, err
	}
	s.progress.noteEncoded()
	return pixels, nil
}

func (s *screenFrameSource) nextPacketLocked() []byte {
	if cap(s.blockBuf) < s.fountainEnc.BlockSize() {
		s.blockBuf = make([]byte, s.fountainEnc.BlockSize())
	} else {
		s.blockBuf = s.blockBuf[:s.fountainEnc.BlockSize()]
	}
	block := s.fountainEnc.EncodeInto(s.frameID, s.blockBuf)
	s.frameID++
	s.packetBuf = BuildPacketInto(s.packetBuf, s.fileSize, block.FrameID, block.Data)
	return s.packetBuf
}

func (s *screenFrameSource) notePresented() {
	s.progress.notePresented()
}

func ResolveEncoderRegion(region string, width int, height int) (image.Rectangle, error) {
	if region == "" {
		region = "0:0"
	}
	return resolveScreenRegion(region, width, height, true, "encoder")
}

func ResolveDecoderRegion(region string, width int, height int) (image.Rectangle, error) {
	if region == "" {
		return image.Rectangle{}, fmt.Errorf("decoder region must be SCREEN:X:Y")
	}
	return resolveScreenRegion(region, width, height, false, "decoder")
}

func resolveScreenRegion(region string, width int, height int, allowImplicitScreen bool, label string) (image.Rectangle, error) {
	screenIndex, xToken, yToken, err := parseRegionSpec(region, allowImplicitScreen)
	if err != nil {
		if allowImplicitScreen {
			return image.Rectangle{}, fmt.Errorf("%s region must be X:Y or SCREEN:X:Y: %w", label, err)
		}
		return image.Rectangle{}, fmt.Errorf("%s region must be SCREEN:X:Y: %w", label, err)
	}
	if screenIndex < 0 || screenIndex >= screenshot.NumActiveDisplays() {
		return image.Rectangle{}, fmt.Errorf("screen index %d out of range", screenIndex)
	}
	bounds := screenshot.GetDisplayBounds(screenIndex)
	x, err := resolveAxis(xToken, bounds.Min.X, bounds.Max.X, width)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region x: %w", err)
	}
	y, err := resolveAxis(yToken, bounds.Min.Y, bounds.Max.Y, height)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("invalid region y: %w", err)
	}
	return image.Rect(x, y, x+width, y+height), nil
}

func parseRegionSpec(region string, allowImplicitScreen bool) (screenIndex int, xToken string, yToken string, err error) {
	parts := strings.Split(region, ":")
	switch len(parts) {
	case 2:
		if !allowImplicitScreen {
			return 0, "", "", fmt.Errorf("missing screen index")
		}
		return 0, parts[0], parts[1], nil
	case 3:
		screenIndex, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", "", fmt.Errorf("invalid screen index: %w", err)
		}
		return screenIndex, parts[1], parts[2], nil
	default:
		return 0, "", "", fmt.Errorf("invalid field count %d", len(parts))
	}
}

func resolveAxis(token string, min int, max int, size int) (int, error) {
	if token == "" {
		return 0, fmt.Errorf("empty coordinate")
	}
	if strings.HasPrefix(token, "-") {
		offsetText := strings.TrimPrefix(token, "-")
		if offsetText == "" {
			offsetText = "0"
		}
		offset, err := strconv.Atoi(offsetText)
		if err != nil {
			return 0, err
		}
		return max - size - offset, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil {
		return 0, err
	}
	return min + offset, nil
}

func validateGridScale(gridSize int, scale int) error {
	if gridSize <= 0 {
		return fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return fmt.Errorf("B must be > 0")
	}
	return nil
}
