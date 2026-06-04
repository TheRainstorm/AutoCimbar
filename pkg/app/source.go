package app

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

type SourceData struct {
	FileSize       int
	MD5            string
	Compression    uint32
	CompressedSize int
	Payload        []byte
}

func BuildSourceData(data []byte) []byte {
	return BuildSourceDataWithCompression(data, true)
}

func BuildSourceDataWithCompression(data []byte, compress bool) []byte {
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

	out := make([]byte, SourceHeaderSize+payload.Len())
	writeSourceHeader(out[:SourceHeaderSize], len(data), BytesMD5(data), compression)
	copy(out[SourceHeaderSize:], payload.Bytes())
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

	out := make([]byte, SourceHeaderSize+payload.Len())
	md5Bytes := hash.Sum(nil)
	writeSourceHeader(out[:SourceHeaderSize], counter.n, md5BytesToArray(md5Bytes), compression)
	copy(out[SourceHeaderSize:], payload.Bytes())
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
	payload := data[SourceHeaderSize:]
	source, err := decodeSourcePayloadToBytes(payload, header.compression, header.fileSize, header.md5)
	if err != nil {
		return nil, err
	}
	source.CompressedSize = len(payload)
	return source, nil
}

func WriteSourceDataToFile(data []byte, outputPath string) (*SourceData, error) {
	header, err := parseSourceHeader(data)
	if err != nil {
		return nil, err
	}

	tmpPath := outputPath + ".tmp"
	output, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("create temp output: %w", err)
	}
	source, err := decodeSourcePayloadToWriter(data[SourceHeaderSize:], output, header.compression, header.fileSize, header.md5)
	closeErr := output.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp output: %w", closeErr)
	}
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("remove old output file: %w", err)
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("replace output file: %w", err)
	}
	return source, nil
}

type sourceHeader struct {
	fileSize    int
	compression uint32
	md5         [16]byte
}

func writeSourceHeader(dst []byte, fileSize int, md5Sum [16]byte, compression uint32) {
	binary.BigEndian.PutUint32(dst[0:4], SourceMagic)
	binary.BigEndian.PutUint32(dst[4:8], compression)
	binary.BigEndian.PutUint64(dst[8:16], uint64(fileSize))
	copy(dst[16:32], md5Sum[:])
}

func parseSourceHeader(data []byte) (*sourceHeader, error) {
	if len(data) < SourceHeaderSize {
		return nil, fmt.Errorf("source data too short: got %d, need %d", len(data), SourceHeaderSize)
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != SourceMagic {
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
	return &sourceHeader{fileSize: int(fileSize), compression: compression, md5: md5Sum}, nil
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

func md5BytesToArray(sum []byte) [16]byte {
	var out [16]byte
	copy(out[:], sum)
	return out
}
