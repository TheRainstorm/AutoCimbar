package app

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"

	"github.com/autocambar/autocambar/pkg/ecc"
	"github.com/autocambar/autocambar/pkg/symbol"
)

const (
	FrameMagic            = uint32(0x41434231) // "ACB1"
	FrameHeaderSize       = 20
	SourceMagic           = uint32(0x41435331) // "ACS1"
	SourceMagicV2         = uint32(0x41435332) // "ACS2"
	SourceHeaderSize      = 32
	SourceHeaderV2Size    = 36
	SourceCompressionNone = uint32(0)
	SourceCompressionZstd = uint32(1)
)

var ErrInvalidFrameMagic = errors.New("invalid frame magic")
var ErrInvalidFrameCRC = errors.New("invalid frame crc")

type Frame struct {
	FileSize int
	FrameID  uint32
	Payload  []byte
}

func GridCapacityBytes(gridSize int) int {
	return GridCapacityBytesWithSpec(gridSize, symbol.DefaultSpec(), 2)
}

func GridCapacityBytesWithColorBits(gridSize int, colorBits int) int {
	return GridCapacityBytesWithSpec(gridSize, symbol.DefaultSpec(), colorBits)
}

func GridCapacityBytesWithSpec(gridSize int, spec symbol.Spec, colorBits int) int {
	if gridSize <= 0 {
		return 0
	}
	cellBits, err := CellBitsForSpec(spec, colorBits)
	if err != nil {
		return 0
	}
	return gridSize * gridSize * cellBits / 8
}

func PayloadCapacityBytes(gridSize int) int {
	return GridCapacityBytes(gridSize) - FrameHeaderSize
}

func PayloadCapacityBytesWithColorBits(gridSize int, colorBits int) int {
	return GridCapacityBytesWithColorBits(gridSize, colorBits) - FrameHeaderSize
}

func PayloadCapacityBytesWithECC(gridSize int, eccPercent int) (int, error) {
	return PayloadCapacityBytesWithECCAndColorBits(gridSize, eccPercent, 2)
}

func PayloadCapacityBytesWithECCAndColorBits(gridSize int, eccPercent int, colorBits int) (int, error) {
	return PayloadCapacityBytesWithECCAndColorBitsAndPackets(gridSize, eccPercent, colorBits, 1)
}

func PayloadCapacityBytesWithECCAndColorBitsAndPackets(gridSize int, eccPercent int, colorBits int, packetsPerFrame int) (int, error) {
	return PayloadCapacityBytesWithECCAndSpecAndPackets(gridSize, eccPercent, symbol.DefaultSpec(), colorBits, packetsPerFrame)
}

func PayloadCapacityBytesWithECCAndSpecAndPackets(gridSize int, eccPercent int, spec symbol.Spec, colorBits int, packetsPerFrame int) (int, error) {
	frameCapacity := GridCapacityBytesWithSpec(gridSize, spec, colorBits)
	return PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, eccPercent, packetsPerFrame)
}

func PayloadCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity int, eccPercent int, packetsPerFrame int) (int, error) {
	packetCapacity, err := PacketDataCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, eccPercent, packetsPerFrame)
	if err != nil {
		return 0, err
	}
	payloadCapacity := packetCapacity - FrameHeaderSize
	if payloadCapacity <= 0 {
		return 0, fmt.Errorf("frame capacity is too small: packet data capacity %d, frame header %d", packetCapacity, FrameHeaderSize)
	}
	return payloadCapacity, nil
}

func PacketDataCapacityBytesWithECC(gridSize int, eccPercent int) (int, error) {
	return PacketDataCapacityBytesWithECCAndColorBits(gridSize, eccPercent, 2)
}

func PacketDataCapacityBytesWithECCAndColorBits(gridSize int, eccPercent int, colorBits int) (int, error) {
	return PacketDataCapacityBytesWithECCAndColorBitsAndPackets(gridSize, eccPercent, colorBits, 1)
}

func PacketDataCapacityBytesWithECCAndColorBitsAndPackets(gridSize int, eccPercent int, colorBits int, packetsPerFrame int) (int, error) {
	return PacketDataCapacityBytesWithECCAndSpecAndPackets(gridSize, eccPercent, symbol.DefaultSpec(), colorBits, packetsPerFrame)
}

func PacketDataCapacityBytesWithECCAndSpecAndPackets(gridSize int, eccPercent int, spec symbol.Spec, colorBits int, packetsPerFrame int) (int, error) {
	frameCapacity := GridCapacityBytesWithSpec(gridSize, spec, colorBits)
	return PacketDataCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity, eccPercent, packetsPerFrame)
}

func PacketDataCapacityBytesWithECCAndFrameCapacityAndPackets(frameCapacity int, eccPercent int, packetsPerFrame int) (int, error) {
	if packetsPerFrame <= 0 {
		return 0, fmt.Errorf("packets per frame must be > 0")
	}
	if frameCapacity <= 0 {
		return 0, fmt.Errorf("frame capacity is too small: %d", frameCapacity)
	}
	packetCapacity := frameCapacity / packetsPerFrame
	if packetCapacity <= 0 {
		return 0, fmt.Errorf("frame capacity %d is too small for %d packets per frame", frameCapacity, packetsPerFrame)
	}
	return ecc.MaxPacketDataSize(packetCapacity, eccPercent)
}

func NewFramePacketCodec(gridSize int, eccPercent int, blockSize int) (*ecc.PacketCodec, error) {
	return NewFramePacketCodecWithColorBits(gridSize, eccPercent, blockSize, 2)
}

func NewFramePacketCodecWithColorBits(gridSize int, eccPercent int, blockSize int, colorBits int) (*ecc.PacketCodec, error) {
	return NewFramePacketCodecWithColorBitsAndPackets(gridSize, eccPercent, blockSize, colorBits, 1)
}

func NewFramePacketCodecWithColorBitsAndPackets(gridSize int, eccPercent int, blockSize int, colorBits int, packetsPerFrame int) (*ecc.PacketCodec, error) {
	return NewFramePacketCodecWithSpecAndPackets(gridSize, eccPercent, blockSize, symbol.DefaultSpec(), colorBits, packetsPerFrame)
}

func NewFramePacketCodecWithSpecAndPackets(gridSize int, eccPercent int, blockSize int, spec symbol.Spec, colorBits int, packetsPerFrame int) (*ecc.PacketCodec, error) {
	return NewFramePacketCodecWithFrameCapacityAndPackets(GridCapacityBytesWithSpec(gridSize, spec, colorBits), eccPercent, blockSize, packetsPerFrame)
}

func NewFramePacketCodecWithFrameCapacityAndPackets(frameCapacity int, eccPercent int, blockSize int, packetsPerFrame int) (*ecc.PacketCodec, error) {
	if blockSize <= 0 {
		return nil, fmt.Errorf("block size must be > 0")
	}
	packetDataSize := FrameHeaderSize + blockSize
	packetCodec, err := ecc.NewPacketCodec(eccPercent, packetDataSize)
	if err != nil {
		return nil, err
	}
	if packetsPerFrame <= 0 {
		return nil, fmt.Errorf("packets per frame must be > 0")
	}
	if packetCodec.EncodedSize()*packetsPerFrame > frameCapacity {
		return nil, fmt.Errorf("encoded packet size %d * packets %d exceeds frame capacity %d", packetCodec.EncodedSize(), packetsPerFrame, frameCapacity)
	}
	return packetCodec, nil
}

func CellBitsForColorBits(colorBits int) (int, error) {
	return CellBitsForSpec(symbol.DefaultSpec(), colorBits)
}

func CellBitsForSpec(spec symbol.Spec, colorBits int) (int, error) {
	if err := spec.Validate(); err != nil {
		return 0, err
	}
	if colorBits < 0 || colorBits > 8 {
		return 0, fmt.Errorf("color bits must be 0..8, got %d", colorBits)
	}
	return spec.ShapeBits + colorBits, nil
}

func normalizePacketsPerFrame(packetsPerFrame int) int {
	if packetsPerFrame <= 0 {
		return 1
	}
	return packetsPerFrame
}

func CellSize(scale int) int {
	return CellSizeForSpec(scale, symbol.DefaultSpec())
}

func CellSizeForSpec(scale int, spec symbol.Spec) int {
	return spec.Width * scale
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
	binary.BigEndian.PutUint32(packet[16:20], 0)
	copy(packet[FrameHeaderSize:], payload)
	binary.BigEndian.PutUint32(packet[16:20], packetCRC(packet))
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
	wantCRC := binary.BigEndian.Uint32(data[16:20])
	if gotCRC := packetCRC(data[:FrameHeaderSize+blockSize]); gotCRC != wantCRC {
		return nil, fmt.Errorf("%w: got 0x%08x, want 0x%08x", ErrInvalidFrameCRC, gotCRC, wantCRC)
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

func packetCRC(packet []byte) uint32 {
	if len(packet) < FrameHeaderSize {
		return 0
	}
	crc := crc32.NewIEEE()
	_, _ = crc.Write(packet[4:16])
	_, _ = crc.Write(packet[FrameHeaderSize:])
	return crc.Sum32()
}
