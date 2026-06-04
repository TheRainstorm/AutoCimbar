package app

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/autocambar/autocambar/pkg/ecc"
)

const (
	FrameMagic            = uint32(0x41434231) // "ACB1"
	FrameHeaderSize       = 16
	SourceMagic           = uint32(0x41435331) // "ACS1"
	SourceHeaderSize      = 32
	SourceCompressionZstd = uint32(1)
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

func PayloadCapacityBytesWithECC(gridSize int, eccPercent int) (int, error) {
	packetCapacity, err := PacketDataCapacityBytesWithECC(gridSize, eccPercent)
	if err != nil {
		return 0, err
	}
	payloadCapacity := packetCapacity - FrameHeaderSize
	if payloadCapacity <= 0 {
		return 0, fmt.Errorf("grid Q=%d capacity is too small: packet data capacity %d, frame header %d", gridSize, packetCapacity, FrameHeaderSize)
	}
	return payloadCapacity, nil
}

func PacketDataCapacityBytesWithECC(gridSize int, eccPercent int) (int, error) {
	frameCapacity := GridCapacityBytes(gridSize)
	if frameCapacity <= 0 {
		return 0, fmt.Errorf("grid Q=%d capacity is too small", gridSize)
	}
	return ecc.MaxPacketDataSize(frameCapacity, eccPercent)
}

func NewFramePacketCodec(gridSize int, eccPercent int, blockSize int) (*ecc.PacketCodec, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	packetDataSize := FrameHeaderSize + blockSize
	packetCodec, err := ecc.NewPacketCodec(eccPercent, packetDataSize)
	if err != nil {
		return nil, err
	}
	frameCapacity := GridCapacityBytes(gridSize)
	if packetCodec.EncodedSize() > frameCapacity {
		return nil, fmt.Errorf("encoded packet size %d exceeds frame capacity %d", packetCodec.EncodedSize(), frameCapacity)
	}
	return packetCodec, nil
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
	BuildPacketInto(packet, fileSize, frameID, payload)
	return packet
}

func BuildPacketInto(packet []byte, fileSize int, frameID uint32, payload []byte) []byte {
	need := FrameHeaderSize + len(payload)
	if cap(packet) < need {
		packet = make([]byte, need)
	} else {
		packet = packet[:need]
	}
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
