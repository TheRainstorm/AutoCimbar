package app

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FrameMagic      = uint32(0x41434231) // "ACB1"
	FrameHeaderSize = 16
)

var ErrInvalidFrameMagic = errors.New("invalid frame magic")

type Frame struct {
	FileSize int
	FrameID  uint32
	Payload  []byte
}

func GridCapacityBytes(gridSize int) int {
	if gridSize <= 0 {
		return 0
	}
	return gridSize * gridSize * 6 / 8
}

func PayloadCapacityBytes(gridSize int) int {
	return GridCapacityBytes(gridSize) - FrameHeaderSize
}

func CellSize(scale int) int {
	return 8 * scale
}

func blockCountForFile(fileSize int, blockSize int) int {
	if fileSize <= 0 {
		return 1
	}
	blockCount := fileSize / blockSize
	if fileSize%blockSize != 0 {
		blockCount++
	}
	return blockCount
}

func BuildPacket(fileSize int, frameID uint32, payload []byte) []byte {
	packet := make([]byte, FrameHeaderSize+len(payload))
	binary.BigEndian.PutUint32(packet[0:4], FrameMagic)
	binary.BigEndian.PutUint64(packet[4:12], uint64(fileSize))
	binary.BigEndian.PutUint32(packet[12:16], frameID)
	copy(packet[FrameHeaderSize:], payload)
	return packet
}

func ParsePacket(data []byte, blockSize int) (*Frame, error) {
	if len(data) < FrameHeaderSize+blockSize {
		return nil, fmt.Errorf("packet too short: got %d, need %d", len(data), FrameHeaderSize+blockSize)
	}

	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != FrameMagic {
		return nil, fmt.Errorf("%w: got 0x%08x", ErrInvalidFrameMagic, magic)
	}

	fileSize := binary.BigEndian.Uint64(data[4:12])
	if fileSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("file size too large: %d", fileSize)
	}

	payload := make([]byte, blockSize)
	copy(payload, data[FrameHeaderSize:FrameHeaderSize+blockSize])
	return &Frame{
		FileSize: int(fileSize),
		FrameID:  binary.BigEndian.Uint32(data[12:16]),
		Payload:  payload,
	}, nil
}
