package app

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	HeaderSize = 68
	Magic      = "ACBAR01\x00"
)

type Frame struct {
	FileSize    uint64
	ChunkSize   uint32
	FrameIndex  uint32
	FrameCount  uint32
	PayloadSize uint32
	PayloadCRC  uint32
	FileSHA256  [32]byte
	Payload     []byte
}

func GridCapacityBytes(gridSize int) int {
	if gridSize <= 0 {
		return 0
	}
	return gridSize * gridSize * 6 / 8
}

func PayloadCapacityBytes(gridSize int) int {
	return GridCapacityBytes(gridSize) - HeaderSize
}

func CellSize(scale int) int {
	return 8 * scale
}

func BuildPacket(fileSize uint64, chunkSize uint32, frameIndex uint32, frameCount uint32, fileHash [32]byte, payload []byte) []byte {
	packet := make([]byte, HeaderSize+len(payload))
	copy(packet[0:8], Magic)
	binary.BigEndian.PutUint64(packet[8:16], fileSize)
	binary.BigEndian.PutUint32(packet[16:20], chunkSize)
	binary.BigEndian.PutUint32(packet[20:24], frameIndex)
	binary.BigEndian.PutUint32(packet[24:28], frameCount)
	binary.BigEndian.PutUint32(packet[28:32], uint32(len(payload)))
	binary.BigEndian.PutUint32(packet[32:36], crc32.ChecksumIEEE(payload))
	copy(packet[36:68], fileHash[:])
	copy(packet[HeaderSize:], payload)
	return packet
}

func ParsePacket(data []byte) (*Frame, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("packet too short: %d bytes", len(data))
	}
	if string(data[0:8]) != Magic {
		return nil, fmt.Errorf("invalid frame magic")
	}

	frame := &Frame{
		FileSize:    binary.BigEndian.Uint64(data[8:16]),
		ChunkSize:   binary.BigEndian.Uint32(data[16:20]),
		FrameIndex:  binary.BigEndian.Uint32(data[20:24]),
		FrameCount:  binary.BigEndian.Uint32(data[24:28]),
		PayloadSize: binary.BigEndian.Uint32(data[28:32]),
		PayloadCRC:  binary.BigEndian.Uint32(data[32:36]),
	}
	copy(frame.FileSHA256[:], data[36:68])

	if frame.FrameCount == 0 {
		return nil, fmt.Errorf("invalid frame count 0")
	}
	if frame.FrameIndex >= frame.FrameCount {
		return nil, fmt.Errorf("frame index %d out of range %d", frame.FrameIndex, frame.FrameCount)
	}
	if frame.PayloadSize > frame.ChunkSize {
		return nil, fmt.Errorf("payload size %d exceeds chunk size %d", frame.PayloadSize, frame.ChunkSize)
	}
	if HeaderSize+int(frame.PayloadSize) > len(data) {
		return nil, fmt.Errorf("payload size %d exceeds packet size %d", frame.PayloadSize, len(data))
	}

	frame.Payload = make([]byte, frame.PayloadSize)
	copy(frame.Payload, data[HeaderSize:HeaderSize+int(frame.PayloadSize)])
	if crc32.ChecksumIEEE(frame.Payload) != frame.PayloadCRC {
		return nil, fmt.Errorf("payload crc mismatch for frame %d", frame.FrameIndex)
	}

	return frame, nil
}

func FileHash(data []byte) [32]byte {
	return sha256.Sum256(data)
}
