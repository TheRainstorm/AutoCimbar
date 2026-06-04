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
	colorpkg "github.com/autocambar/autocambar/pkg/color"
	"github.com/autocambar/autocambar/pkg/fountain"
)

type EncodeResult struct {
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
	CompressedSize  int
	TransferSize    int
	BlockSize       int
	BlockCount      int
	MD5             string
}

func EncodeFileToPNGFrames(inputPath string, outputDir string, gridSize int, scale int, symbolDir string, redundancyPercent int, blockSize int, eccPercent int) (*EncodeResult, error) {
	if gridSize <= 0 {
		return nil, fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("B must be > 0")
	}
	if redundancyPercent < 0 {
		return nil, fmt.Errorf("redundancy must be >= 0")
	}

	payloadCapacity, err := PayloadCapacityBytesWithECC(gridSize, eccPercent)
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
	packetCodec, err := NewFramePacketCodec(gridSize, eccPercent, blockSize)
	if err != nil {
		return nil, err
	}

	sourceData, fileSize, md5Hex, err := BuildSourceDataFromFile(inputPath)
	if err != nil {
		return nil, err
	}
	fountainEnc, err := fountain.NewEncoder(sourceData, blockSize)
	if err != nil {
		return nil, err
	}

	symRec, err := LoadLibcimbarSymbols(symbolDir)
	if err != nil {
		return nil, err
	}
	enc := codec.NewEncoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize)

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

	imageSize := gridSize * CellSize(scale)
	return &EncodeResult{
		FramePaths:      paths,
		GridSize:        gridSize,
		Scale:           scale,
		CellSize:        CellSize(scale),
		ImageSize:       imageSize,
		FrameCapacity:   GridCapacityBytes(gridSize),
		PayloadCapacity: payloadCapacity,
		ECCPercent:      eccPercent,
		ECCBytes:        packetCodec.ParitySize(),
		PacketBytes:     packetCodec.EncodedSize(),
		FileSize:        fileSize,
		CompressedSize:  len(sourceData) - SourceHeaderSize,
		TransferSize:    len(sourceData),
		BlockSize:       fountainEnc.BlockSize(),
		BlockCount:      fountainEnc.BlockCount(),
		MD5:             md5Hex,
	}, nil
}

func DecodePNGFramesToFile(inputPath string, outputPath string, gridSize int, scale int, symbolDir string, blockSize int, eccPercent int) error {
	if gridSize <= 0 {
		return fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return fmt.Errorf("B must be > 0")
	}

	paths, err := collectPNGPaths(inputPath)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no PNG frames found in %s", inputPath)
	}
	payloadCapacity, err := PayloadCapacityBytesWithECC(gridSize, eccPercent)
	if err != nil {
		return err
	}
	if blockSize == 0 {
		blockSize = payloadCapacity
	}
	if blockSize <= 0 {
		return fmt.Errorf("block size must be > 0")
	}
	if blockSize > payloadCapacity {
		return fmt.Errorf("block size %d exceeds frame payload capacity %d", blockSize, payloadCapacity)
	}
	packetCodec, err := NewFramePacketCodec(gridSize, eccPercent, blockSize)
	if err != nil {
		return err
	}
	symRec, err := LoadLibcimbarSymbols(symbolDir)
	if err != nil {
		return err
	}
	codecDec := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize)
	var fountainDec *fountain.Decoder
	blockCount := 0
	fileSize := -1
	var packetBuf []byte
	for _, path := range paths {
		frame, packet, err := decodeFrameFile(codecDec, packetCodec, path, blockSize, packetBuf)
		if err != nil {
			return err
		}
		packetBuf = packet
		if fountainDec == nil {
			fileSize = frame.FileSize
			blockCount = blockCountForFile(fileSize, blockSize)
			fountainDec, err = fountain.NewDecoder(fileSize, blockSize, blockCount)
			if err != nil {
				return err
			}
		} else if frame.FileSize != fileSize {
			return fmt.Errorf("frame %s belongs to a different file size: got %d, want %d", path, frame.FileSize, fileSize)
		}
		if _, err := fountainDec.AddFrame(frame.FrameID, frame.Payload); err != nil {
			return fmt.Errorf("add frame %s: %w", path, err)
		}
		if fountainDec.Complete() {
			break
		}
	}

	if fountainDec == nil {
		return fmt.Errorf("no decodable frames found")
	}
	if !fountainDec.Complete() {
		return fmt.Errorf("not enough independent frames: rank %d of %d", fountainDec.Rank(), blockCount)
	}

	result, err := fountainDec.Decode()
	if err != nil {
		return err
	}
	if _, err := WriteSourceDataToFile(result, outputPath); err != nil {
		return fmt.Errorf("write decoded source: %w", err)
	}
	return nil
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
