package app

import (
	"encoding/binary"
	"fmt"
)

const FrameHeaderSize = 12

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
	blockCount := (fileSize + blockSize - 1) / blockSize
	if blockCount == 0 {
		blockCount = 1
	}
	return blockCount
}

func BuildPacket(fileSize int, frameID uint32, payload []byte) []byte {
	packet := make([]byte, FrameHeaderSize+len(payload))
	binary.BigEndian.PutUint64(packet[0:8], uint64(fileSize))
	binary.BigEndian.PutUint32(packet[8:12], frameID)
	copy(packet[FrameHeaderSize:], payload)
	return packet
}

func ParsePacket(data []byte, blockSize int) (*Frame, error) {
	if len(data) < FrameHeaderSize+blockSize {
		return nil, fmt.Errorf("packet too short: got %d, need %d", len(data), FrameHeaderSize+blockSize)
	}

	fileSize := binary.BigEndian.Uint64(data[0:8])
	if fileSize > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("file size too large: %d", fileSize)
	}

	payload := make([]byte, blockSize)
	copy(payload, data[FrameHeaderSize:FrameHeaderSize+blockSize])
	return &Frame{
		FileSize: int(fileSize),
		FrameID:  binary.BigEndian.Uint32(data[8:12]),
		Payload:  payload,
	}, nil
}
