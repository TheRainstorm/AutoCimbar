package app

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
)

func BytesMD5(data []byte) [16]byte {
	return md5.Sum(data)
}

func MD5Hex(sum [16]byte) string {
	return hex.EncodeToString(sum[:])
}

func MD5HexBytes(sum []byte) string {
	return hex.EncodeToString(sum)
}

func BytesMD5Hex(data []byte) string {
	return MD5Hex(BytesMD5(data))
}

func FileMD5Hex(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file for md5: %w", err)
	}
	return BytesMD5Hex(data), nil
}
