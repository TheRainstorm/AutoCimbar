package app

import (
	"errors"
	"fmt"
	"image"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/autocambar/autocambar/pkg/symbol"
	"github.com/kbinani/screenshot"
)

type ScreenEncodeConfig struct {
	InputPath       string
	GridSize        int
	Scale           int
	SymbolDir       string
	BlockSize       int
	ECCPercent      int
	NoZstd          bool
	ColorBits       int
	ShapeBits       int
	Tile            string
	PacketsPerFrame int
	Region          string
	FPS             int
	Addr            string
	Open            bool
	Progress        io.Writer
}

type ScreenDecodeConfig struct {
	OutputPath      string
	GridSize        int
	Scale           int
	SymbolDir       string
	BlockSize       int
	ECCPercent      int
	ColorBits       int
	ShapeBits       int
	Tile            string
	PacketsPerFrame int
	Region          string
	FPS             int
	Timeout         time.Duration
	Progress        io.Writer
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
	colorBits := normalizeColorBits(cfg.ColorBits)
	packetsPerFrame := normalizePacketsPerFrame(cfg.PacketsPerFrame)
	shapeBits := cfg.ShapeBits
	if shapeBits == 0 {
		shapeBits = symbol.DefaultShapeBits
	}
	spec, err := ParseTileSpec(cfg.Tile, shapeBits)
	if err != nil {
		return nil, err
	}

	payloadCapacity, err := PayloadCapacityBytesWithECCAndSpecAndPackets(cfg.GridSize, cfg.ECCPercent, spec, colorBits, packetsPerFrame)
	if err != nil {
		return nil, err
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithSpecAndPackets(cfg.GridSize, cfg.ECCPercent, blockSize, spec, colorBits, packetsPerFrame)
	if err != nil {
		return nil, err
	}

	compress := !cfg.NoZstd
	sourceData, fileSize, md5Hex, err := BuildSourceDataFromFileWithCompression(cfg.InputPath, compress)
	if err != nil {
		return nil, err
	}
	fountainEnc, err := fountain.NewEncoder(sourceData, blockSize)
	if err != nil {
		return nil, err
	}

	symRec, err := LoadSymbols(cfg.SymbolDir, spec)
	if err != nil {
		return nil, err
	}
	colorRec, err := colorRecognizerForBits(colorBits)
	if err != nil {
		return nil, err
	}
	codecEnc, err := codec.NewEncoderWithColorBits(symRec, colorRec, CellSizeForSpec(cfg.Scale, spec), cfg.GridSize, colorBits)
	if err != nil {
		return nil, err
	}

	imageSize := cfg.GridSize * CellSizeForSpec(cfg.Scale, spec)
	rect, err := ResolveEncoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return nil, err
	}

	result := &EncodeResult{
		GridSize:        cfg.GridSize,
		Scale:           cfg.Scale,
		CellSize:        CellSizeForSpec(cfg.Scale, spec),
		ImageSize:       imageSize,
		FrameCapacity:   GridCapacityBytesWithSpec(cfg.GridSize, spec, colorBits),
		PayloadCapacity: payloadCapacity,
		ECCPercent:      cfg.ECCPercent,
		ECCBytes:        packetCodec.ParitySize(),
		PacketBytes:     packetCodec.EncodedSize(),
		FileSize:        fileSize,
		CompressedSize:  len(sourceData) - SourceHeaderSize,
		TransferSize:    len(sourceData),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
		MD5:             md5Hex,
		Compression:     sourceCompression(compress),
		ColorBits:       colorBits,
		ShapeBits:       spec.ShapeBits,
		TileWidth:       spec.Width,
		TileHeight:      spec.Height,
		PacketsPerFrame: packetsPerFrame,
	}
	progress := newScreenEncoderProgress(cfg.Progress, result)
	progress.startSummary()
	defer progress.finishLine()

	source := &screenFrameSource{
		codecEnc:        codecEnc,
		fountainEnc:     fountainEnc,
		fileSize:        len(sourceData),
		width:           imageSize,
		height:          imageSize,
		progress:        progress,
		packetCodec:     packetCodec,
		packetsPerFrame: packetsPerFrame,
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
	colorBits := normalizeColorBits(cfg.ColorBits)
	packetsPerFrame := normalizePacketsPerFrame(cfg.PacketsPerFrame)
	shapeBits := cfg.ShapeBits
	if shapeBits == 0 {
		shapeBits = symbol.DefaultShapeBits
	}
	spec, err := ParseTileSpec(cfg.Tile, shapeBits)
	if err != nil {
		return err
	}

	payloadCapacity, err := PayloadCapacityBytesWithECCAndSpecAndPackets(cfg.GridSize, cfg.ECCPercent, spec, colorBits, packetsPerFrame)
	if err != nil {
		return err
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithSpecAndPackets(cfg.GridSize, cfg.ECCPercent, blockSize, spec, colorBits, packetsPerFrame)
	if err != nil {
		return err
	}

	symRec, err := LoadSymbols(cfg.SymbolDir, spec)
	if err != nil {
		return err
	}
	colorRec, err := colorRecognizerForBits(colorBits)
	if err != nil {
		return err
	}
	codecDec, err := codec.NewDecoderWithColorBits(symRec, colorRec, CellSizeForSpec(cfg.Scale, spec), cfg.GridSize, colorBits)
	if err != nil {
		return err
	}
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1

	imageSize := cfg.GridSize * CellSizeForSpec(cfg.Scale, spec)
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

	frames := make(chan *capturedScreenFrame, 1)
	freeBuffers := make(chan []byte, 4)
	captureErr := make(chan error, 1)
	stopCapture := make(chan struct{})
	captureDone := make(chan struct{})
	go runScreenCaptureLoop(capturer, interval, progress, frames, freeBuffers, captureErr, stopCapture, captureDone)
	defer func() {
		close(stopCapture)
		<-captureDone
	}()

	var encodedFrameBuf []byte
	var packetBuf []byte
	seenFrameIDs := make(map[uint32]struct{})
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timeout := time.NewTimer(remaining)
		var captured *capturedScreenFrame
		select {
		case captured = <-frames:
		case err := <-captureErr:
			timeout.Stop()
			if errors.Is(err, ErrScreenCapture) {
				return fmt.Errorf("capture screen: %w", err)
			}
			return err
		case <-timeout.C:
			continue
		}
		if !timeout.Stop() {
			select {
			case <-timeout.C:
			default:
			}
		}

		encodedFrame, err := decodeCapturedFrame(codecDec, captured, encodedFrameBuf)
		recycleCapturedFrame(captured, freeBuffers)
		if err != nil {
			progress.noteInvalid()
			continue
		}
		progress.noteDecoded()
		encodedFrameBuf = encodedFrame
		packetBytes := packetCodec.EncodedSize()
		for packetIndex := 0; packetIndex < packetsPerFrame; packetIndex++ {
			start := packetIndex * packetBytes
			end := start + packetBytes
			if end > len(encodedFrame) {
				progress.noteInvalid()
				continue
			}
			packet, err := packetCodec.DecodeInto(encodedFrame[start:end], packetBuf)
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
			if _, ok := seenFrameIDs[frame.FrameID]; ok {
				progress.noteValid(fountainDec.Rank(), false, true)
				continue
			}
			seenFrameIDs[frame.FrameID] = struct{}{}
			added, err := fountainDec.AddFrame(frame.FrameID, frame.Payload)
			if err != nil {
				return fmt.Errorf("add captured frame: %w", err)
			}
			progress.noteValid(fountainDec.Rank(), added, false)
			if fountainDec.Complete() {
				progress.noteComplete()
				result, err := fountainDec.Decode()
				if err != nil {
					return err
				}
				if _, err := WriteSourceDataToFile(result, cfg.OutputPath); err != nil {
					return fmt.Errorf("write decoded source: %w", err)
				}
				return nil
			}
		}
	}

	rank := 0
	if fountainDec != nil {
		rank = fountainDec.Rank()
	}
	return fmt.Errorf("timeout waiting for enough frames: rank %d of %d", rank, blockCount)
}

func runScreenCaptureLoop(capturer *screenCapturer, interval time.Duration, progress *screenDecoderProgress, frames chan *capturedScreenFrame, freeBuffers chan []byte, captureErr chan<- error, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	nextCapture := time.Now()
	var buf []byte
	for {
		if now := time.Now(); now.Before(nextCapture) {
			select {
			case <-stop:
				return
			case <-time.After(time.Until(nextCapture)):
			}
		} else if now.Sub(nextCapture) > interval {
			nextCapture = now
		}
		nextCapture = nextCapture.Add(interval)

		select {
		case <-stop:
			return
		case buf = <-freeBuffers:
		default:
			buf = nil
		}

		frame, err := capturer.CaptureFrame(buf)
		if err != nil {
			select {
			case captureErr <- err:
			default:
			}
			return
		}
		progress.noteCaptured()

		select {
		case frames <- frame:
		default:
			select {
			case old := <-frames:
				recycleCapturedFrame(old, freeBuffers)
			default:
			}
			select {
			case frames <- frame:
			default:
				recycleCapturedFrame(frame, freeBuffers)
			}
		}
	}
}

func decodeCapturedFrame(dec *codec.Decoder, frame *capturedScreenFrame, dst []byte) ([]byte, error) {
	if frame.BGRA {
		return dec.DecodeBGRAInto(frame.Pix, frame.Width, frame.Height, frame.Stride, dst)
	}
	return dec.DecodeInto(frame.Img, dst)
}

func recycleCapturedFrame(frame *capturedScreenFrame, freeBuffers chan<- []byte) {
	if frame == nil || !frame.BGRA || frame.Pix == nil {
		return
	}
	select {
	case freeBuffers <- frame.Pix:
	default:
	}
}

type screenFrameSource struct {
	codecEnc         *codec.Encoder
	fountainEnc      *fountain.Encoder
	fileSize         int
	frameID          uint32
	width            int
	height           int
	progress         *screenEncoderProgress
	packetsPerFrame  int
	blockBuf         []byte
	packetBuf        []byte
	frameBuf         []byte
	encodedPacketBuf []byte
	packetCodec      interface {
		EncodeInto([]byte, []byte) ([]byte, error)
		EncodedSize() int
	}
	mu sync.Mutex
}

func (s *screenFrameSource) NextImage() (*image.RGBA, error) {
	s.mu.Lock()
	frame, packetCount, err := s.nextFrameLocked()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	frame = append([]byte(nil), frame...)
	s.mu.Unlock()

	img, err := s.codecEnc.Encode(frame)
	if err != nil {
		return nil, err
	}
	s.progress.noteEncoded(packetCount)
	return img, nil
}

func (s *screenFrameSource) NextBGRA(dst []byte) ([]byte, error) {
	s.mu.Lock()
	frame, packetCount, err := s.nextFrameLocked()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	pixels, err := s.codecEnc.EncodeBGRA(frame, dst)
	s.mu.Unlock()

	if err != nil {
		return nil, err
	}
	s.progress.noteEncoded(packetCount)
	return pixels, nil
}

func (s *screenFrameSource) nextFrameLocked() ([]byte, int, error) {
	packetsPerFrame := s.packetsPerFrame
	if packetsPerFrame <= 0 {
		packetsPerFrame = 1
	}
	packetBytes := 0
	if s.packetCodec != nil {
		packetBytes = s.packetCodec.EncodedSize()
	}
	if packetBytes <= 0 {
		packetBytes = FrameHeaderSize + s.fountainEnc.BlockSize()
	}
	need := packetBytes * packetsPerFrame
	if cap(s.frameBuf) < need {
		s.frameBuf = make([]byte, need)
	} else {
		s.frameBuf = s.frameBuf[:need]
		clear(s.frameBuf)
	}

	for i := 0; i < packetsPerFrame; i++ {
		packet, err := s.nextPacketLocked()
		if err != nil {
			return nil, 0, err
		}
		copy(s.frameBuf[i*packetBytes:(i+1)*packetBytes], packet)
	}
	return s.frameBuf, packetsPerFrame, nil
}

func (s *screenFrameSource) nextPacketLocked() ([]byte, error) {
	if cap(s.blockBuf) < s.fountainEnc.BlockSize() {
		s.blockBuf = make([]byte, s.fountainEnc.BlockSize())
	} else {
		s.blockBuf = s.blockBuf[:s.fountainEnc.BlockSize()]
	}
	block := s.fountainEnc.EncodeInto(s.frameID, s.blockBuf)
	s.frameID++
	s.packetBuf = BuildPacketInto(s.packetBuf, s.fileSize, block.FrameID, block.Data)
	if s.packetCodec == nil {
		return s.packetBuf, nil
	}
	encoded, err := s.packetCodec.EncodeInto(s.packetBuf, s.encodedPacketBuf)
	if err != nil {
		return nil, err
	}
	s.encodedPacketBuf = encoded
	return s.encodedPacketBuf, nil
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
