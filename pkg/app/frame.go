package app

import (
	"encoding/binary"
	"fmt"
)

const FrameIDSize = 4

type Frame struct {
	FrameID uint32
	Payload []byte
}

func GridCapacityBytes(gridSize int) int {
	if gridSize <= 0 {
		return 0
	}
	return gridSize * gridSize * 6 / 8
}

func PayloadCapacityBytes(gridSize int) int {
	return GridCapacityBytes(gridSize) - FrameIDSize
}

func CellSize(scale int) int {
	return 8 * scale
}

func BuildPacket(frameID uint32, payload []byte) []byte {
	packet := make([]byte, FrameIDSize+len(payload))
	binary.BigEndian.PutUint32(packet[:FrameIDSize], frameID)
	copy(packet[FrameIDSize:], payload)
	return packet
}

func ParsePacket(data []byte, blockSize int) (*Frame, error) {
	if len(data) < FrameIDSize+blockSize {
		return nil, fmt.Errorf("packet too short: got %d, need %d", len(data), FrameIDSize+blockSize)
	}

	payload := make([]byte, blockSize)
	copy(payload, data[FrameIDSize:FrameIDSize+blockSize])
	return &Frame{
		FrameID: binary.BigEndian.Uint32(data[:FrameIDSize]),
		Payload: payload,
	}, nil
}
