package app

import (
	"errors"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/autocambar/autocambar/pkg/symbol"
)

type ScreenEncodeConfig struct {
	InputPath       string
	Backend         string
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
	Stop            <-chan struct{}
}

type ScreenDecodeConfig struct {
	OutputPath       string
	Backend          string
	GridSize         int
	Scale            int
	SymbolDir        string
	BlockSize        int
	ECCPercent       int
	ColorBits        int
	ShapeBits        int
	Tile             string
	PacketsPerFrame  int
	Region           string
	FPS              int
	DecodeWorkers    int
	CaptureBackend   string
	DebugCapturePath string
	Timeout          time.Duration
	Progress         io.Writer
	Stop             <-chan struct{}
	Pause            <-chan bool
}

var ErrStopped = errors.New("operation stopped")

func DisplayBounds() []image.Rectangle {
	count := activeDisplayCount()
	displays := make([]image.Rectangle, 0, count)
	for i := 0; i < count; i++ {
		displays = append(displays, displayBounds(i))
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
	backend, err := normalizeBackend(cfg.Backend)
	if err != nil {
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

	var frameCapacity int
	var payloadCapacity int
	var imageSize int
	var cellSize int
	var tileWidth int
	var tileHeight int
	var frameEnc frameEncoder
	if backend == BackendQR {
		qr, err := newQRFrameCodec(cfg.GridSize, cfg.Scale)
		if err != nil {
			return nil, err
		}
		frameCapacity = qr.capacity
		payloadCapacity, err = PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, packetsPerFrame)
		if err != nil {
			return nil, err
		}
		imageSize = qr.size
		cellSize = cfg.Scale
		tileWidth = qr.modules
		tileHeight = qr.modules
		frameEnc = qr
	} else {
		frameCapacity = GridCapacityBytesWithSpec(cfg.GridSize, spec, colorBits)
		payloadCapacity, err = PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, packetsPerFrame)
		if err != nil {
			return nil, err
		}
		cellSize = CellSizeForSpec(cfg.Scale, spec)
		imageSize = cfg.GridSize * cellSize
		tileWidth = spec.Width
		tileHeight = spec.Height
		symRec, err := LoadSymbols(cfg.SymbolDir, spec)
		if err != nil {
			return nil, err
		}
		colorRec, err := colorRecognizerForBits(colorBits)
		if err != nil {
			return nil, err
		}
		codecEnc, err := codec.NewEncoderWithColorBits(symRec, colorRec, cellSize, cfg.GridSize, colorBits)
		if err != nil {
			return nil, err
		}
		frameEnc = codecEnc
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, blockSize, packetsPerFrame)
	if err != nil {
		return nil, err
	}
	compress := !cfg.NoZstd
	sourceData, fileSize, md5Hex, err := BuildSourceDataFromFileWithCompression(cfg.InputPath, compress)
	if err != nil {
		return nil, err
	}
	sourcePayloadSize, err := SourcePayloadSize(sourceData)
	if err != nil {
		return nil, err
	}
	fountainEnc, err := fountain.NewEncoder(sourceData, blockSize)
	if err != nil {
		return nil, err
	}

	rect, err := ResolveEncoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return nil, err
	}

	result := &EncodeResult{
		Backend:         backend,
		GridSize:        cfg.GridSize,
		Scale:           cfg.Scale,
		CellSize:        cellSize,
		ImageSize:       imageSize,
		FrameCapacity:   frameCapacity,
		PayloadCapacity: payloadCapacity,
		ECCPercent:      cfg.ECCPercent,
		ECCBytes:        packetCodec.ParitySize(),
		PacketBytes:     packetCodec.EncodedSize(),
		FileSize:        fileSize,
		FileName:        filepath.Base(cfg.InputPath),
		CompressedSize:  sourcePayloadSize,
		TransferSize:    len(sourceData),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
		MD5:             md5Hex,
		Compression:     sourceCompression(compress),
		ColorBits:       colorBits,
		ShapeBits:       spec.ShapeBits,
		TileWidth:       tileWidth,
		TileHeight:      tileHeight,
		PacketsPerFrame: packetsPerFrame,
	}
	progress := newScreenEncoderProgress(cfg.Progress, result)
	progress.startSummary()
	defer progress.finishLine()

	source := &screenFrameSource{
		encoder:         frameEnc,
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
	_, err := DecodeScreenToPath(cfg)
	return err
}

func DecodeScreenToPath(cfg ScreenDecodeConfig) (*WriteSourceResult, error) {
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if err := validateGridScale(cfg.GridSize, cfg.Scale); err != nil {
		return nil, err
	}
	backend, err := normalizeBackend(cfg.Backend)
	if err != nil {
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

	var frameCapacity int
	var payloadCapacity int
	var imageSize int
	var cellSize int
	var decoders []frameDecoder
	if backend == BackendQR {
		qr, err := newQRFrameCodec(cfg.GridSize, cfg.Scale)
		if err != nil {
			return nil, err
		}
		frameCapacity = qr.capacity
		payloadCapacity, err = PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, packetsPerFrame)
		if err != nil {
			return nil, err
		}
		imageSize = qr.size
		cellSize = cfg.Scale
		decoders = []frameDecoder{qr}
	} else {
		frameCapacity = GridCapacityBytesWithSpec(cfg.GridSize, spec, colorBits)
		payloadCapacity, err = PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, packetsPerFrame)
		if err != nil {
			return nil, err
		}
		cellSize = CellSizeForSpec(cfg.Scale, spec)
		imageSize = cfg.GridSize * cellSize
		symRec, err := LoadSymbols(cfg.SymbolDir, spec)
		if err != nil {
			return nil, err
		}
		colorRec, err := colorRecognizerForBits(colorBits)
		if err != nil {
			return nil, err
		}
		decodeWorkers := normalizeDecodeWorkers(cfg.DecodeWorkers)
		decoders = make([]frameDecoder, decodeWorkers)
		for i := 0; i < decodeWorkers; i++ {
			dec, err := codec.NewDecoderWithColorBits(symRec, colorRec, cellSize, cfg.GridSize, colorBits)
			if err != nil {
				return nil, err
			}
			decoders[i] = dec
		}
	}
	blockSize := cfg.BlockSize
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 || blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithFrameCapacityAndPackets(frameCapacity, cfg.ECCPercent, blockSize, packetsPerFrame)
	if err != nil {
		return nil, err
	}
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1

	rect, err := ResolveDecoderRegion(cfg.Region, imageSize, imageSize)
	if err != nil {
		return nil, err
	}
	capturer, err := newScreenCapturer(rect, cfg.CaptureBackend)
	if err != nil {
		return nil, err
	}
	defer capturer.Close()
	if cfg.Progress != nil {
		fmt.Fprintf(cfg.Progress, "capture backend=%s\n", capturer.Name())
	}

	interval := time.Second / time.Duration(cfg.FPS)
	if interval <= 0 {
		interval = time.Millisecond
	}
	progress := newScreenDecoderProgress(cfg.Progress, blockSize)
	defer progress.finishLine()
	isPaused, stopPause := newPauseFlag(cfg.Pause)
	defer stopPause()

	frames := make(chan *capturedScreenFrame, len(decoders))
	decodedFrames := make(chan decodedScreenFrame, len(decoders))
	freeBuffers := make(chan []byte, len(decoders)*2+2)
	captureErr := make(chan error, 1)
	stopCapture := make(chan struct{})
	captureDone := make(chan struct{})
	go runScreenCaptureLoop(capturer, interval, progress, frames, freeBuffers, captureErr, stopCapture, isPaused, cfg.DebugCapturePath, captureDone)
	decodeDone := make(chan struct{})
	go runScreenDecodeWorkers(decoders, frames, decodedFrames, freeBuffers, progress, stopCapture, isPaused, decodeDone)
	defer func() {
		close(stopCapture)
		<-captureDone
		<-decodeDone
	}()

	var packetBuf []byte
	seenFrameIDs := make(map[uint32]struct{})
	for {
		if !waitWhilePaused(isPaused, cfg.Stop) {
			return nil, ErrStopped
		}
		var timeout <-chan time.Time
		var timer *time.Timer
		if cfg.Timeout > 0 {
			timer = time.NewTimer(cfg.Timeout)
			timeout = timer.C
		}
		var decoded decodedScreenFrame
		select {
		case decoded = <-decodedFrames:
		case err := <-captureErr:
			stopTimer(timer)
			if errors.Is(err, ErrScreenCapture) {
				return nil, fmt.Errorf("capture screen: %w", err)
			}
			return nil, err
		case <-cfg.Stop:
			stopTimer(timer)
			return nil, ErrStopped
		case <-timeout:
			continue
		}
		stopTimer(timer)

		if decoded.Err != nil {
			continue
		}
		encodedFrame := decoded.Data
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
					return nil, err
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
				return nil, fmt.Errorf("add captured frame: %w", err)
			}
			progress.noteValid(fountainDec.Rank(), added, false)
			if fountainDec.Complete() {
				progress.noteComplete()
				result, err := fountainDec.Decode()
				if err != nil {
					return nil, err
				}
				writeResult, err := WriteSourceDataToPath(result, cfg.OutputPath)
				if err != nil {
					return nil, fmt.Errorf("write decoded source: %w", err)
				}
				return writeResult, nil
			}
		}
	}

	rank := 0
	if fountainDec != nil {
		rank = fountainDec.Rank()
	}
	return nil, fmt.Errorf("stopped waiting for enough frames: rank %d of %d", rank, blockCount)
}

func newPauseFlag(updates <-chan bool) (func() bool, func()) {
	var paused atomic.Bool
	done := make(chan struct{})
	if updates == nil {
		return paused.Load, func() {}
	}
	stop := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case value, ok := <-updates:
				if !ok {
					return
				}
				paused.Store(value)
			case <-stop:
				return
			}
		}
	}()
	return paused.Load, func() {
		close(stop)
		<-done
	}
}

func waitWhilePaused(isPaused func() bool, stop <-chan struct{}) bool {
	for isPaused() {
		select {
		case <-stop:
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
	return true
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func runScreenCaptureLoop(capturer *screenCapturer, interval time.Duration, progress *screenDecoderProgress, frames chan *capturedScreenFrame, freeBuffers chan []byte, captureErr chan<- error, stop <-chan struct{}, isPaused func() bool, debugCapturePath string, done chan<- struct{}) {
	defer close(done)
	nextCapture := time.Now()
	var buf []byte
	debugCaptureCount := 0
	const maxDebugCaptures = 60
	for {
		if !waitWhilePaused(isPaused, stop) {
			return
		}
		if now := time.Now(); now.Before(nextCapture) {
			select {
			case <-stop:
				return
			case <-time.After(time.Until(nextCapture)):
			}
		} else if now.Sub(nextCapture) > interval {
			nextCapture = now
		}
		if isPaused() {
			nextCapture = time.Now()
			continue
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
		if debugCapturePath != "" && debugCaptureCount < maxDebugCaptures {
			path := debugCaptureFramePath(debugCapturePath, debugCaptureCount)
			if err := saveCapturedFramePNG(path, frame); err != nil {
				select {
				case captureErr <- fmt.Errorf("save debug capture: %w", err):
				default:
				}
				recycleCapturedFrame(frame, freeBuffers)
				return
			}
			debugCaptureCount++
			if progress.out != nil {
				fmt.Fprintf(progress.out, "debug capture saved: %s\n", path)
			}
		}

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

type decodedScreenFrame struct {
	Data []byte
	Err  error
}

func runScreenDecodeWorkers(decoders []frameDecoder, frames <-chan *capturedScreenFrame, decoded chan<- decodedScreenFrame, freeBuffers chan<- []byte, progress *screenDecoderProgress, stop <-chan struct{}, isPaused func() bool, done chan<- struct{}) {
	var wg sync.WaitGroup
	wg.Add(len(decoders))
	for _, dec := range decoders {
		go func(dec frameDecoder) {
			defer wg.Done()
			var frameBuf []byte
			for {
				if !waitWhilePaused(isPaused, stop) {
					return
				}
				select {
				case <-stop:
					return
				case captured := <-frames:
					if captured == nil {
						continue
					}
					if !waitWhilePaused(isPaused, stop) {
						recycleCapturedFrame(captured, freeBuffers)
						return
					}
					encodedFrame, err := decodeCapturedFrame(dec, captured, frameBuf)
					recycleCapturedFrame(captured, freeBuffers)
					if err != nil {
						progress.noteInvalid()
						select {
						case decoded <- decodedScreenFrame{Err: err}:
						case <-stop:
						}
						continue
					}
					progress.noteDecoded()
					frameBuf = encodedFrame
					out := append([]byte(nil), encodedFrame...)
					select {
					case decoded <- decodedScreenFrame{Data: out}:
					case <-stop:
						return
					}
				}
			}
		}(dec)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
}

func decodeCapturedFrame(dec frameDecoder, frame *capturedScreenFrame, dst []byte) ([]byte, error) {
	if frame.BGRA {
		return dec.DecodeBGRAInto(frame.Pix, frame.Width, frame.Height, frame.Stride, dst)
	}
	return dec.DecodeInto(frame.Img, dst)
}

func normalizeDecodeWorkers(workers int) int {
	if workers > 0 {
		return workers
	}
	cpus := runtime.NumCPU()
	if cpus <= 2 {
		return 1
	}
	workers = cpus / 2
	if workers > 4 {
		workers = 4
	}
	if workers < 1 {
		workers = 1
	}
	return workers
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
	encoder          frameEncoder
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

	img, err := s.encoder.Encode(frame)
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
	pixels, err := s.encoder.EncodeBGRA(frame, dst)
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
		region = "0"
	}
	return resolveScreenRegion(region, width, height, true, "encoder")
}

func ResolveDecoderRegion(region string, width int, height int) (image.Rectangle, error) {
	if region == "" {
		region = "0"
	}
	return resolveScreenRegion(region, width, height, true, "decoder")
}

func resolveScreenRegion(region string, width int, height int, allowImplicitScreen bool, label string) (image.Rectangle, error) {
	screenIndex, xToken, yToken, err := parseRegionSpec(region, allowImplicitScreen)
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("%s region must be SCREEN, X:Y or SCREEN:X:Y: %w", label, err)
	}
	if screenIndex < 0 || screenIndex >= activeDisplayCount() {
		return image.Rectangle{}, fmt.Errorf("screen index %d out of range", screenIndex)
	}
	bounds := displayBounds(screenIndex)
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
	case 1:
		screenIndex, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, "", "", fmt.Errorf("invalid screen index: %w", err)
		}
		return screenIndex, "-0", "-0", nil
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
	if strings.EqualFold(token, "c") {
		return min + (max-min-size)/2, nil
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
