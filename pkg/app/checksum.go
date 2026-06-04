package app

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
)

func BytesMD5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func FileMD5Hex(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file for md5: %w", err)
	}
	return BytesMD5Hex(data), nil
}
