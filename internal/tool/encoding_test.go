package tool

import (
	"testing"
)

func TestStripBOM(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "UTF-8 BOM",
			input:    append([]byte{0xEF, 0xBB, 0xBF}, []byte("hello")...),
			expected: []byte("hello"),
		},
		{
			name:     "UTF-16 LE BOM",
			input:    append([]byte{0xFF, 0xFE}, []byte("hello")...),
			expected: []byte("hello"),
		},
		{
			name:     "UTF-16 BE BOM",
			input:    append([]byte{0xFE, 0xFF}, []byte("hello")...),
			expected: []byte("hello"),
		},
		{
			name:     "no BOM",
			input:    []byte("hello"),
			expected: []byte("hello"),
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "only UTF-8 BOM",
			input:    []byte{0xEF, 0xBB, 0xBF},
			expected: []byte{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripBOM(tt.input)
			if string(result) != string(tt.expected) {
				t.Errorf("StripBOM(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsBinaryData(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{
			name:     "text file",
			input:    []byte("package main\n\nfunc main() {}\n"),
			expected: false,
		},
		{
			name:     "binary with null byte",
			input:    []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A},
			expected: true,
		},
		{
			name:     "empty data",
			input:    []byte{},
			expected: false,
		},
		{
			name: "null byte beyond 8KB not detected",
			input: func() []byte {
				b := make([]byte, 8193)
				for i := range b {
					b[i] = 'A'
				}
				b[8192] = 0x00 // null byte at position 8192, outside first 8KB
				return b
			}(),
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsBinaryData(tt.input)
			if result != tt.expected {
				t.Errorf("IsBinaryData() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDecodeFileContent(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    string
		wantErr bool
	}{
		{
			name:    "clean UTF-8 text",
			input:   []byte("func main() {}"),
			want:    "func main() {}",
			wantErr: false,
		},
		{
			name:    "UTF-8 with BOM",
			input:   append([]byte{0xEF, 0xBB, 0xBF}, []byte("func main() {}")...),
			want:    "func main() {}",
			wantErr: false,
		},
		{
			name:    "binary file",
			input:   []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0D, 0x0A, 0x1A, 0x0A},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeFileContent(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeFileContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DecodeFileContent() = %q, want %q", got, tt.want)
			}
		})
	}
}
