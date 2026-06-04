package app

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/autocambar/autocambar/pkg/codec"
	colorpkg "github.com/autocambar/autocambar/pkg/color"
)

type EncodeResult struct {
	FramePaths      []string
	GridSize        int
	Scale           int
	CellSize        int
	ImageSize       int
	FrameCapacity   int
	PayloadCapacity int
	FileSize        int
}

func EncodeFileToPNGFrames(inputPath string, outputDir string, gridSize int, scale int, symbolDir string) (*EncodeResult, error) {
	if gridSize <= 0 {
		return nil, fmt.Errorf("Q must be > 0")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("B must be > 0")
	}

	payloadCapacity := PayloadCapacityBytes(gridSize)
	if payloadCapacity <= 0 {
		return nil, fmt.Errorf("grid Q=%d capacity is too small: frame capacity %d, header %d", gridSize, GridCapacityBytes(gridSize), HeaderSize)
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("read input file: %w", err)
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

	fileHash := FileHash(data)
	frameCount := (len(data) + payloadCapacity - 1) / payloadCapacity
	if frameCount == 0 {
		frameCount = 1
	}

	paths := make([]string, 0, frameCount)
	for i := 0; i < frameCount; i++ {
		start := i * payloadCapacity
		end := start + payloadCapacity
		if end > len(data) {
			end = len(data)
		}
		if start > len(data) {
			start = len(data)
		}

		packet := BuildPacket(uint64(len(data)), uint32(payloadCapacity), uint32(i), uint32(frameCount), fileHash, data[start:end])
		img, err := enc.Encode(packet)
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
		FileSize:        len(data),
	}, nil
}

func DecodePNGFramesToFile(inputPath string, outputPath string, gridSize int, scale int, symbolDir string) error {
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

	symRec, err := LoadLibcimbarSymbols(symbolDir)
	if err != nil {
		return err
	}
	dec := codec.NewDecoder(symRec, colorpkg.NewRecognizer4Color(), CellSize(scale), gridSize)

	var expected *Frame
	chunks := make(map[uint32][]byte)
	for _, path := range paths {
		frame, err := decodeFrameFile(dec, path)
		if err != nil {
			return err
		}

		if expected == nil {
			expected = frame
		} else if frame.FileSize != expected.FileSize ||
			frame.ChunkSize != expected.ChunkSize ||
			frame.FrameCount != expected.FrameCount ||
			frame.FileSHA256 != expected.FileSHA256 {
			return fmt.Errorf("frame %s belongs to a different transfer", path)
		}

		if _, exists := chunks[frame.FrameIndex]; !exists {
			chunks[frame.FrameIndex] = frame.Payload
		}
	}

	if expected == nil {
		return fmt.Errorf("no decodable frames found")
	}
	for i := uint32(0); i < expected.FrameCount; i++ {
		if _, ok := chunks[i]; !ok {
			return fmt.Errorf("missing frame %d of %d", i, expected.FrameCount)
		}
	}

	var out bytes.Buffer
	for i := uint32(0); i < expected.FrameCount; i++ {
		out.Write(chunks[i])
	}

	result := out.Bytes()
	if uint64(len(result)) < expected.FileSize {
		return fmt.Errorf("decoded data too short: got %d, need %d", len(result), expected.FileSize)
	}
	result = result[:expected.FileSize]

	actualHash := sha256.Sum256(result)
	if actualHash != expected.FileSHA256 {
		return fmt.Errorf("file sha256 mismatch")
	}

	if err := os.WriteFile(outputPath, result, 0644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func decodeFrameFile(dec *codec.Decoder, path string) (*Frame, error) {
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

	packet, err := dec.Decode(img)
	if err != nil {
		return nil, fmt.Errorf("decode frame %s: %w", path, err)
	}

	frame, err := ParsePacket(packet)
	if err != nil {
		return nil, fmt.Errorf("parse frame %s: %w", path, err)
	}
	return frame, nil
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
