package tool

import (
	"bytes"
	"fmt"
)

// BOM bytes for common encodings
var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// StripBOM removes Byte Order Mark from file content if present.
func StripBOM(data []byte) []byte {
	if bytes.HasPrefix(data, bomUTF8) {
		return data[3:]
	}
	if bytes.HasPrefix(data, bomUTF16LE) || bytes.HasPrefix(data, bomUTF16BE) {
		return data[2:]
	}
	return data
}

// DecodeFileContent reads file bytes and returns clean UTF-8 string.
// Strips BOM, detects binary (null bytes in first 8KB), handles common encodings.
func DecodeFileContent(data []byte) (string, error) {
	if IsBinaryData(data) {
		return "", fmt.Errorf("binary file detected")
	}
	cleaned := StripBOM(data)
	return string(cleaned), nil
}

// IsBinaryData checks for null bytes in the first 8KB.
func IsBinaryData(data []byte) bool {
	check := data
	if len(check) > 8192 {
		check = check[:8192]
	}
	for _, b := range check {
		if b == 0 {
			return true
		}
	}
	return false
}
