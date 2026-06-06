package app

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

type SourceData struct {
	FileSize       int
	FileName       string
	MD5            string
	Compression    uint32
	CompressedSize int
	Payload        []byte
}

type WriteSourceResult struct {
	Source     *SourceData
	OutputPath string
}

func BuildSourceData(data []byte) []byte {
	return BuildSourceDataWithCompression(data, true)
}

func BuildSourceDataWithCompression(data []byte, compress bool) []byte {
	return BuildSourceDataWithCompressionAndName(data, "", compress)
}

func BuildSourceDataWithCompressionAndName(data []byte, fileName string, compress bool) []byte {
	var payload bytes.Buffer
	compression := SourceCompressionNone
	if compress {
		compression = SourceCompressionZstd
		writer, err := zstd.NewWriter(&payload)
		if err != nil {
			panic(err)
		}
		if _, err := writer.Write(data); err != nil {
			panic(err)
		}
		if err := writer.Close(); err != nil {
			panic(err)
		}
	} else {
		if _, err := payload.Write(data); err != nil {
			panic(err)
		}
	}

	fileName = cleanTransferFileName(fileName)
	headerSize := sourceHeaderSize(fileName)
	out := make([]byte, headerSize+payload.Len())
	writeSourceHeader(out[:headerSize], len(data), BytesMD5(data), compression, fileName)
	copy(out[headerSize:], payload.Bytes())
	return out
}

func BuildSourceDataFromFile(path string) ([]byte, int, string, error) {
	return BuildSourceDataFromFileWithCompression(path, true)
}

func BuildSourceDataFromFileWithCompression(path string, compress bool) ([]byte, int, string, error) {
	input, err := os.Open(path)
	if err != nil {
		return nil, 0, "", fmt.Errorf("open input file: %w", err)
	}
	defer input.Close()

	var payload bytes.Buffer
	hash := md5.New()
	counter := &countingWriter{}
	compression := SourceCompressionNone
	if compress {
		compression = SourceCompressionZstd
		writer, err := zstd.NewWriter(&payload)
		if err != nil {
			return nil, 0, "", fmt.Errorf("create zstd writer: %w", err)
		}
		if _, err := io.Copy(writer, io.TeeReader(input, io.MultiWriter(hash, counter))); err != nil {
			_ = writer.Close()
			return nil, 0, "", fmt.Errorf("compress input file: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, 0, "", fmt.Errorf("finish zstd stream: %w", err)
		}
	} else if _, err := io.Copy(&payload, io.TeeReader(input, io.MultiWriter(hash, counter))); err != nil {
		return nil, 0, "", fmt.Errorf("read input file: %w", err)
	}

	fileName := cleanTransferFileName(filepath.Base(path))
	headerSize := sourceHeaderSize(fileName)
	out := make([]byte, headerSize+payload.Len())
	md5Bytes := hash.Sum(nil)
	writeSourceHeader(out[:headerSize], counter.n, md5BytesToArray(md5Bytes), compression, fileName)
	copy(out[headerSize:], payload.Bytes())
	return out, counter.n, MD5HexBytes(md5Bytes), nil
}

func sourceCompression(compress bool) uint32 {
	if compress {
		return SourceCompressionZstd
	}
	return SourceCompressionNone
}

func SourceCompressionName(compression uint32) string {
	switch compression {
	case SourceCompressionNone:
		return "none"
	case SourceCompressionZstd:
		return "zstd"
	default:
		return fmt.Sprintf("unknown(%d)", compression)
	}
}

func ParseSourceData(data []byte) (*SourceData, error) {
	header, err := parseSourceHeader(data)
	if err != nil {
		return nil, err
	}
	payload := data[header.headerSize:]
	source, err := decodeSourcePayloadToBytes(payload, header.compression, header.fileSize, header.md5)
	if err != nil {
		return nil, err
	}
	source.FileName = header.fileName
	source.CompressedSize = len(payload)
	return source, nil
}

func SourcePayloadSize(data []byte) (int, error) {
	header, err := parseSourceHeader(data)
	if err != nil {
		return 0, err
	}
	return len(data) - header.headerSize, nil
}

func WriteSourceDataToFile(data []byte, outputPath string) (*SourceData, error) {
	result, err := WriteSourceDataToPath(data, outputPath)
	if err != nil {
		return nil, err
	}
	return result.Source, nil
}

func WriteSourceDataToPath(data []byte, outputPath string) (*WriteSourceResult, error) {
	header, err := parseSourceHeader(data)
	if err != nil {
		return nil, err
	}

	finalPath, err := resolveOutputPath(outputPath, header.fileName)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	tmpPath := finalPath + ".tmp"
	output, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	source, err := decodeSourcePayloadToWriter(data[header.headerSize:], output, header.compression, header.fileSize, header.md5)
	closeErr := output.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp output: %w", closeErr)
	}
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("remove old output file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("replace output file: %w", err)
	}
	source.FileName = header.fileName
	return &WriteSourceResult{Source: source, OutputPath: finalPath}, nil
}

type sourceHeader struct {
	fileSize    int
	compression uint32
	md5         [16]byte
	fileName    string
	headerSize  int
}

func writeSourceHeader(dst []byte, fileSize int, md5Sum [16]byte, compression uint32, fileName string) {
	fileName = cleanTransferFileName(fileName)
	magic := SourceMagic
	if fileName != "" {
		magic = SourceMagicV2
	}
	binary.BigEndian.PutUint32(dst[0:4], magic)
	binary.BigEndian.PutUint32(dst[4:8], compression)
	binary.BigEndian.PutUint64(dst[8:16], uint64(fileSize))
	copy(dst[16:32], md5Sum[:])
	if magic == SourceMagicV2 {
		nameBytes := []byte(fileName)
		binary.BigEndian.PutUint32(dst[32:36], uint32(len(nameBytes)))
		copy(dst[36:], nameBytes)
	}
}

func parseSourceHeader(data []byte) (*sourceHeader, error) {
	if len(data) < SourceHeaderSize {
		return nil, fmt.Errorf("source data too short: got %d, need %d", len(data), SourceHeaderSize)
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != SourceMagic && magic != SourceMagicV2 {
		return nil, fmt.Errorf("invalid source magic: got 0x%08x", magic)
	}
	compression := binary.BigEndian.Uint32(data[4:8])
	if compression != SourceCompressionNone && compression != SourceCompressionZstd {
		return nil, fmt.Errorf("unsupported source compression: %d", compression)
	}
	fileSize := binary.BigEndian.Uint64(data[8:16])
	if fileSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("source file size too large: %d", fileSize)
	}
	var md5Sum [16]byte
	copy(md5Sum[:], data[16:32])
	header := &sourceHeader{fileSize: int(fileSize), compression: compression, md5: md5Sum, headerSize: SourceHeaderSize}
	if magic == SourceMagicV2 {
		if len(data) < SourceHeaderV2Size {
			return nil, fmt.Errorf("source data too short: got %d, need %d", len(data), SourceHeaderV2Size)
		}
		nameLen := binary.BigEndian.Uint32(data[32:36])
		if nameLen > 4096 {
			return nil, fmt.Errorf("source file name too long: %d", nameLen)
		}
		header.headerSize = SourceHeaderV2Size + int(nameLen)
		if len(data) < header.headerSize {
			return nil, fmt.Errorf("source data too short for file name: got %d, need %d", len(data), header.headerSize)
		}
		header.fileName = cleanTransferFileName(string(data[36:header.headerSize]))
	}
	return header, nil
}

func decodeSourcePayloadToBytes(payload []byte, compression uint32, fileSize int, wantMD5 [16]byte) (*SourceData, error) {
	var output bytes.Buffer
	source, err := decodeSourcePayloadToWriter(payload, &output, compression, fileSize, wantMD5)
	if err != nil {
		return nil, err
	}
	source.Payload = output.Bytes()
	return source, nil
}

func decodeSourcePayloadToWriter(payload []byte, output io.Writer, compression uint32, fileSize int, wantMD5 [16]byte) (*SourceData, error) {
	if compression == SourceCompressionNone {
		return copyPlainSourceToWriter(payload, output, fileSize, wantMD5)
	}
	return decompressZstdToWriter(payload, output, fileSize, wantMD5)
}

func decompressZstdToWriter(payload []byte, output io.Writer, fileSize int, wantMD5 [16]byte) (*SourceData, error) {
	reader, err := zstd.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create zstd reader: %w", err)
	}
	defer reader.Close()

	hash := md5.New()
	counter := &countingWriter{}
	if _, err := io.Copy(io.MultiWriter(output, hash, counter), reader); err != nil {
		return nil, fmt.Errorf("decompress zstd stream: %w", err)
	}
	gotMD5 := md5BytesToArray(hash.Sum(nil))
	if counter.n != fileSize {
		return nil, fmt.Errorf("source size mismatch: got %d, want %d", counter.n, fileSize)
	}
	if !bytes.Equal(gotMD5[:], wantMD5[:]) {
		return nil, fmt.Errorf("source md5 mismatch: got %s, want %s", MD5Hex(gotMD5), MD5Hex(wantMD5))
	}
	return &SourceData{
		FileSize:       fileSize,
		MD5:            MD5Hex(gotMD5),
		Compression:    SourceCompressionZstd,
		CompressedSize: len(payload),
	}, nil
}

func copyPlainSourceToWriter(payload []byte, output io.Writer, fileSize int, wantMD5 [16]byte) (*SourceData, error) {
	hash := md5.New()
	counter := &countingWriter{}
	if _, err := io.Copy(io.MultiWriter(output, hash, counter), bytes.NewReader(payload)); err != nil {
		return nil, fmt.Errorf("copy plain source: %w", err)
	}
	gotMD5 := md5BytesToArray(hash.Sum(nil))
	if counter.n != fileSize {
		return nil, fmt.Errorf("source size mismatch: got %d, want %d", counter.n, fileSize)
	}
	if !bytes.Equal(gotMD5[:], wantMD5[:]) {
		return nil, fmt.Errorf("source md5 mismatch: got %s, want %s", MD5Hex(gotMD5), MD5Hex(wantMD5))
	}
	return &SourceData{
		FileSize:       fileSize,
		MD5:            MD5Hex(gotMD5),
		Compression:    SourceCompressionNone,
		CompressedSize: len(payload),
	}, nil
}

type countingWriter struct {
	n int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}

func sourceHeaderSize(fileName string) int {
	if fileName == "" {
		return SourceHeaderSize
	}
	return SourceHeaderV2Size + len([]byte(fileName))
}

func cleanTransferFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = filepath.Base(name)
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		switch r {
		case 0, '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return -1
		default:
			return r
		}
	}, name)
	name = strings.TrimSpace(name)
	name = strings.Trim(name, ".")
	if len([]byte(name)) > 4096 {
		return string([]byte(name)[:4096])
	}
	return name
}

func resolveOutputPath(outputPath string, fileName string) (string, error) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		outputPath = "."
	}
	info, err := os.Stat(outputPath)
	if err == nil && info.IsDir() {
		name := fileName
		if name == "" {
			name = "decoded.out"
		}
		return filepath.Join(outputPath, name), nil
	}
	if err != nil && os.IsNotExist(err) {
		if strings.HasSuffix(outputPath, string(os.PathSeparator)) || strings.HasSuffix(outputPath, "/") || strings.HasSuffix(outputPath, "\\") {
			name := fileName
			if name == "" {
				name = "decoded.out"
			}
			return filepath.Join(outputPath, name), nil
		}
		return outputPath, nil
	}
	if err != nil {
		return "", fmt.Errorf("stat output path: %w", err)
	}
	return outputPath, nil
}

func md5BytesToArray(sum []byte) [16]byte {
	var out [16]byte
	copy(out[:], sum)
	return out
}
