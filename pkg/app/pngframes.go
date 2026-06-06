package app

import (
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/autocambar/autocambar/pkg/codec"
	"github.com/autocambar/autocambar/pkg/fountain"
	"github.com/autocambar/autocambar/pkg/symbol"
)

type EncodeResult struct {
	Backend         string
	FramePaths      []string
	GridSize        int
	Scale           int
	CellSize        int
	ImageSize       int
	FrameCapacity   int
	PayloadCapacity int
	ECCPercent      int
	ECCBytes        int
	PacketBytes     int
	FileSize        int
	FileName        string
	CompressedSize  int
	TransferSize    int
	BlockSize       int
	BlockCount      int
	MD5             string
	Compression     uint32
	ColorBits       int
	ShapeBits       int
	TileWidth       int
	TileHeight      int
	PacketsPerFrame int
}

func EncodeFileToPNGFrames(inputPath string, outputDir string, gridSize int, scale int, symbolDir string, redundancyPercent int, blockSize int, eccPercent int) (*EncodeResult, error) {
	return EncodeFileToPNGFramesWithOptions(inputPath, outputDir, gridSize, scale, symbolDir, redundancyPercent, blockSize, eccPercent, true, codec.ColorBits)
}

func EncodeFileToPNGFramesWithOptions(inputPath string, outputDir string, gridSize int, scale int, symbolDir string, redundancyPercent int, blockSize int, eccPercent int, compress bool, colorBits int) (*EncodeResult, error) {
	return EncodeFileToPNGFramesWithSpec(inputPath, outputDir, gridSize, scale, symbolDir, redundancyPercent, blockSize, eccPercent, compress, colorBits, symbol.DefaultSpec())
}

func EncodeFileToPNGFramesWithSpec(inputPath string, outputDir string, gridSize int, scale int, symbolDir string, redundancyPercent int, blockSize int, eccPercent int, compress bool, colorBits int, spec symbol.Spec) (*EncodeResult, error) {
	return EncodeFileToPNGFramesWithBackend(inputPath, outputDir, gridSize, scale, symbolDir, redundancyPercent, blockSize, eccPercent, compress, colorBits, spec, BackendSymbols)
}

func EncodeFileToPNGFramesWithBackend(inputPath string, outputDir string, gridSize int, scale int, symbolDir string, redundancyPercent int, blockSize int, eccPercent int, compress bool, colorBits int, spec symbol.Spec, backend string) (*EncodeResult, error) {
	backend, err := normalizeBackend(backend)
	if err != nil {
		return nil, err
	}
	if backend == BackendQR {
		return encodeFileToPNGFramesQR(inputPath, outputDir, gridSize, scale, redundancyPercent, blockSize, eccPercent, compress)
	}
	colorBits = normalizeColorBits(colorBits)
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if gridSize <= 0 {
		return nil, fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("B must be > 0")
	}
	if redundancyPercent < 0 {
		return nil, fmt.Errorf("redundancy must be >= 0")
	}

	payloadCapacity, err := PayloadCapacityBytesWithECCAndSpecAndPackets(gridSize, eccPercent, spec, colorBits, 1)
	if err != nil {
		return nil, err
	}
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	if blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithSpecAndPackets(gridSize, eccPercent, blockSize, spec, colorBits, 1)
	if err != nil {
		return nil, err
	}

	sourceData, fileSize, md5Hex, err := BuildSourceDataFromFileWithCompression(inputPath, compress)
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

	symRec, err := LoadSymbols(symbolDir, spec)
	if err != nil {
		return nil, err
	}
	colorRec, err := colorRecognizerForBits(colorBits)
	if err != nil {
		return nil, err
	}
	enc, err := codec.NewEncoderWithColorBits(symRec, colorRec, CellSizeForSpec(scale, spec), gridSize, colorBits)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	if err := removeOldFrames(outputDir); err != nil {
		return nil, err
	}

	frameCount := fountainEnc.BlockCount()
	totalFrames := frameCount + (frameCount*redundancyPercent+99)/100
	if totalFrames < frameCount {
		totalFrames = frameCount
	}

	paths := make([]string, 0, totalFrames)
	var packetBuf []byte
	var encodedPacketBuf []byte
	for i := 0; i < totalFrames; i++ {
		block := fountainEnc.Encode(uint32(i))
		packetBuf = BuildPacketInto(packetBuf, len(sourceData), block.FrameID, block.Data)
		encodedPacketBuf, err = packetCodec.EncodeInto(packetBuf, encodedPacketBuf)
		if err != nil {
			return nil, fmt.Errorf("ecc encode frame %d: %w", i, err)
		}
		img, err := enc.Encode(encodedPacketBuf)
		if err != nil {
			return nil, fmt.Errorf("encode frame %d: %w", i, err)
		}

		path := filepath.Join(outputDir, fmt.Sprintf("frame_%06d.png", i))
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("create frame %s: %w", path, err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("write frame %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("close frame %s: %w", path, err)
		}

		paths = append(paths, path)
	}

	imageSize := gridSize * CellSizeForSpec(scale, spec)
	return &EncodeResult{
		Backend:         BackendSymbols,
		FramePaths:      paths,
		GridSize:        gridSize,
		Scale:           scale,
		CellSize:        CellSizeForSpec(scale, spec),
		ImageSize:       imageSize,
		FrameCapacity:   GridCapacityBytesWithSpec(gridSize, spec, colorBits),
		PayloadCapacity: payloadCapacity,
		ECCPercent:      eccPercent,
		ECCBytes:        packetCodec.ParitySize(),
		PacketBytes:     packetCodec.EncodedSize(),
		FileSize:        fileSize,
		FileName:        filepath.Base(inputPath),
		CompressedSize:  sourcePayloadSize,
		TransferSize:    len(sourceData),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
		MD5:             md5Hex,
		Compression:     sourceCompression(compress),
		ColorBits:       colorBits,
		ShapeBits:       spec.ShapeBits,
		TileWidth:       spec.Width,
		TileHeight:      spec.Height,
		PacketsPerFrame: 1,
	}, nil
}

func DecodePNGFramesToFile(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int) error {
	return DecodePNGFramesToFileWithColorBits(inputPath, outputPath, gridSize, scale, symbolDir, blockSize, eccPercent, codec.ColorBits)
}

func DecodePNGFramesToFileWithColorBits(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int, colorBits int) error {
	return DecodePNGFramesToFileWithSpec(inputPath, outputPath, gridSize, scale, symbolDir, blockSize, eccPercent, colorBits, symbol.DefaultSpec())
}

func DecodePNGFramesToFileWithSpec(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int, colorBits int, spec symbol.Spec) error {
	return DecodePNGFramesToFileWithBackend(inputPath, outputPath, gridSize, scale, symbolDir, blockSize, eccPercent, colorBits, spec, BackendSymbols)
}

func DecodePNGFramesToFileWithBackend(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int, colorBits int, spec symbol.Spec, backend string) error {
	_, err := DecodePNGFramesToPathWithBackend(inputPath, outputPath, gridSize, scale, symbolDir, blockSize, eccPercent, colorBits, spec, backend)
	return err
}

func DecodePNGFramesToPathWithBackend(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int, colorBits int, spec symbol.Spec, backend string) (*WriteSourceResult, error) {
	backend, err := normalizeBackend(backend)
	if err != nil {
		return nil, err
	}
	if backend == BackendQR {
		return decodePNGFramesToPathQR(inputPath, outputPath, gridSize, scale, blockSize, eccPercent)
	}
	colorBits = normalizeColorBits(colorBits)
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if gridSize <= 0 {
		return nil, fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("B must be > 0")
	}

	paths, err := collectPNGPaths(inputPath)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no PNG frames found in %s", inputPath)
	}
	payloadCapacity, err := PayloadCapacityBytesWithECCAndSpecAndPackets(gridSize, eccPercent, spec, colorBits, 1)
	if err != nil {
		return nil, err
	}
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	if blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithSpecAndPackets(gridSize, eccPercent, blockSize, spec, colorBits, 1)
	if err != nil {
		return nil, err
	}
	symRec, err := LoadSymbols(symbolDir, spec)
	if err != nil {
		return nil, err
	}
	colorRec, err := colorRecognizerForBits(colorBits)
	if err != nil {
		return nil, err
	}
	codecDec, err := codec.NewDecoderWithColorBits(symRec, colorRec, CellSizeForSpec(scale, spec), gridSize, colorBits)
	if err != nil {
		return nil, err
	}
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1
	var packetBuf []byte
	for _, path := range paths {
		frame, packet, err := decodeFrameFile(codecDec, packetCodec, path, blockSize, packetBuf)
		if err != nil {
			return nil, err
		}
		packetBuf = packet
		if fountainDec == nil {
			fileSize = frame.FileSize
			blockCount = blockCountForFile(fileSize, blockSize)
			fountainDec, err = fountain.NewDecoder(fileSize, blockSize, blockCount)
			if err != nil {
				return nil, err
			}
		} else if frame.FileSize != fileSize {
			return nil, fmt.Errorf("frame %s belongs to a different file size: got %d, want %d", path, frame.FileSize, fileSize)
		}
		if _, err := fountainDec.AddFrame(frame.FrameID, frame.Payload); err != nil {
			return nil, fmt.Errorf("add frame %s: %w", path, err)
		}
		if fountainDec.Complete() {
			break
		}
	}

	if fountainDec == nil {
		return nil, fmt.Errorf("no decodable frames found")
	}
	if !fountainDec.Complete() {
		return nil, fmt.Errorf("not enough independent frames: rank %d of %d", fountainDec.Rank(), blockCount)
	}

	result, err := fountainDec.Decode()
	if err != nil {
		return nil, err
	}
	writeResult, err := WriteSourceDataToPath(result, outputPath)
	if err != nil {
		return nil, fmt.Errorf("write decoded source: %w", err)
	}
	return writeResult, nil
}

func decodeFrameFile(dec *codec.Decoder, packetCodec interface {
	DecodeInto([]byte, []byte) ([]byte, error)
}, path string, blockSize int, dst []byte) (*Frame, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, dst, fmt.Errorf("open frame %s: %w", path, err)
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		return nil, dst, fmt.Errorf("decode png %s: %w", path, err)
	}
	if closeErr != nil {
		return nil, dst, fmt.Errorf("close frame %s: %w", path, closeErr)
	}

	encodedPacket, err := dec.Decode(img)
	if err != nil {
		return nil, dst, fmt.Errorf("decode frame %s: %w", path, err)
	}
	packet, err := packetCodec.DecodeInto(encodedPacket, dst)
	if err != nil {
		return nil, dst, fmt.Errorf("ecc decode frame %s: %w", path, err)
	}

	frame, err := ParsePacket(packet, blockSize)
	if err != nil {
		return nil, dst, fmt.Errorf("parse frame %s: %w", path, err)
	}
	return frame, packet, nil
}

func collectPNGPaths(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}

	if !info.IsDir() {
		return []string{inputPath}, nil
	}

	var paths []string
	err = filepath.WalkDir(inputPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".png") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan png frames: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func removeOldFrames(outputDir string) error {
	matches, err := filepath.Glob(filepath.Join(outputDir, "frame_*.png"))
	if err != nil {
		return fmt.Errorf("scan old frames: %w", err)
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove old frame %s: %w", path, err)
		}
	}
	return nil
}

func encodeFileToPNGFramesQR(inputPath string, outputDir string, gridSize int, scale int, redundancyPercent int, blockSize int, eccPercent int, compress bool) (*EncodeResult, error) {
	if redundancyPercent < 0 {
		return nil, fmt.Errorf("redundancy must be >= 0")
	}
	qr, err := newQRFrameCodec(gridSize, scale)
	if err != nil {
		return nil, err
	}
	payloadCapacity, err := PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(qr.capacity, eccPercent, 1)
	if err != nil {
		return nil, err
	}
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	if blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithFrameCapacityAndPackets(qr.capacity, eccPercent, blockSize, 1)
	if err != nil {
		return nil, err
	}
	sourceData, fileSize, md5Hex, err := BuildSourceDataFromFileWithCompression(inputPath, compress)
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
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	if err := removeOldFrames(outputDir); err != nil {
		return nil, err
	}

	frameCount := fountainEnc.BlockCount()
	totalFrames := frameCount + (frameCount*redundancyPercent+99)/100
	if totalFrames < frameCount {
		totalFrames = frameCount
	}
	paths := make([]string, 0, totalFrames)
	var packetBuf []byte
	var encodedPacketBuf []byte
	for i := 0; i < totalFrames; i++ {
		block := fountainEnc.Encode(uint32(i))
		packetBuf = BuildPacketInto(packetBuf, len(sourceData), block.FrameID, block.Data)
		encodedPacketBuf, err = packetCodec.EncodeInto(packetBuf, encodedPacketBuf)
		if err != nil {
			return nil, fmt.Errorf("ecc encode frame %d: %w", i, err)
		}
		img, err := qr.Encode(encodedPacketBuf)
		if err != nil {
			return nil, fmt.Errorf("encode QR frame %d: %w", i, err)
		}
		path := filepath.Join(outputDir, fmt.Sprintf("frame_%06d.png", i))
		f, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("create frame %s: %w", path, err)
		}
		if err := png.Encode(f, img); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("write frame %s: %w", path, err)
		}
		if err := f.Close(); err != nil {
			return nil, fmt.Errorf("close frame %s: %w", path, err)
		}
		paths = append(paths, path)
	}

	return &EncodeResult{
		Backend:         BackendQR,
		FramePaths:      paths,
		GridSize:        gridSize,
		Scale:           scale,
		CellSize:        scale,
		ImageSize:       qr.size,
		FrameCapacity:   qr.capacity,
		PayloadCapacity: payloadCapacity,
		ECCPercent:      eccPercent,
		ECCBytes:        packetCodec.ParitySize(),
		PacketBytes:     packetCodec.EncodedSize(),
		FileSize:        fileSize,
		FileName:        filepath.Base(inputPath),
		CompressedSize:  sourcePayloadSize,
		TransferSize:    len(sourceData),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
		MD5:             md5Hex,
		Compression:     sourceCompression(compress),
		ColorBits:       0,
		ShapeBits:       0,
		TileWidth:       qr.modules,
		TileHeight:      qr.modules,
		PacketsPerFrame: 1,
	}, nil
}

func decodePNGFramesToFileQR(inputPath string, outputPath string, gridSize int, scale int, blockSize int, eccPercent int) error {
	_, err := decodePNGFramesToPathQR(inputPath, outputPath, gridSize, scale, blockSize, eccPercent)
	return err
}

func decodePNGFramesToPathQR(inputPath string, outputPath string, gridSize int, scale int, blockSize int, eccPercent int) (*WriteSourceResult, error) {
	qr, err := newQRFrameCodec(gridSize, scale)
	if err != nil {
		return nil, err
	}
	paths, err := collectPNGPaths(inputPath)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no PNG frames found in %s", inputPath)
	}
	payloadCapacity, err := PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(qr.capacity, eccPercent, 1)
	if err != nil {
		return nil, err
	}
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	if blockSize > payloadCapacity {
		return nil, fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodecWithFrameCapacityAndPackets(qr.capacity, eccPercent, blockSize, 1)
	if err != nil {
		return nil, err
	}
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1
	var packetBuf []byte
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open frame %s: %w", path, err)
		}
		img, err := png.Decode(f)
		closeErr := f.Close()
		if err != nil {
			return nil, fmt.Errorf("decode png %s: %w", path, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close frame %s: %w", path, closeErr)
		}
		encodedPacket, err := qr.DecodeInto(img, nil)
		if err != nil {
			return nil, fmt.Errorf("decode QR frame %s: %w", path, err)
		}
		packet, err := packetCodec.DecodeInto(encodedPacket, packetBuf)
		if err != nil {
			return nil, fmt.Errorf("ecc decode frame %s: %w", path, err)
		}
		packetBuf = packet
		frame, err := ParsePacket(packet, blockSize)
		if err != nil {
			return nil, fmt.Errorf("parse frame %s: %w", path, err)
		}
		if fountainDec == nil {
			fileSize = frame.FileSize
			blockCount = blockCountForFile(fileSize, blockSize)
			fountainDec, err = fountain.NewDecoder(fileSize, blockSize, blockCount)
			if err != nil {
				return nil, err
			}
		} else if frame.FileSize != fileSize {
			return nil, fmt.Errorf("frame %s belongs to a different file size: got %d, want %d", path, frame.FileSize, fileSize)
		}
		if _, err := fountainDec.AddFrame(frame.FrameID, frame.Payload); err != nil {
			return nil, fmt.Errorf("add frame %s: %w", path, err)
		}
		if fountainDec.Complete() {
			break
		}
	}
	if fountainDec == nil {
		return nil, fmt.Errorf("no decodable frames found")
	}
	if !fountainDec.Complete() {
		return nil, fmt.Errorf("not enough independent frames: rank %d of %d", fountainDec.Rank(), blockCount)
	}
	result, err := fountainDec.Decode()
	if err != nil {
		return nil, err
	}
	writeResult, err := WriteSourceDataToPath(result, outputPath)
	if err != nil {
		return nil, fmt.Errorf("write decoded source: %w", err)
	}
	return writeResult, nil
}
