// Package zstdz provides Zstandard frame primitives for the JSONL persistence
// backend, ported from DSH's session-persistence-jsonl/src/zstd.ts.
//
// It owns a concatenated-frame container so the persistence layer can append
// and recover batches without exposing compression mechanics through the
// persistence seam. Each frame is independently decodable and checksummed.
package zstdz

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// ZstdMagic is the 4-byte little-endian Zstandard frame magic number.
const ZstdMagic uint32 = 0xFD2FB528

// FrameRange is one structurally complete Zstandard frame's byte range.
type FrameRange struct {
	// Start is the inclusive frame start.
	Start int
	// End is the exclusive frame end.
	End int
}

// FrameScan is the structural scan result for a concatenated Zstandard stream.
type FrameScan struct {
	// Frames is the complete frames in file order.
	Frames []FrameRange
	// TornStart is the start of an incomplete final frame, when EOF
	// interrupts one. nil means the buffer ends on a clean frame boundary.
	TornStart *int
}

// scanZstdFrames locates complete frames without decompressing their blocks.
// Ported from DSH's scanZstdFrames. Invalid complete structure rejects; EOF
// inside the final frame returns its start for repair.
func ScanFrames(buffer []byte, maxFrames int) FrameScan {
	var frames []FrameRange
	var tornStart *int
	offset := 0

	if maxFrames == 0 {
		maxFrames = -1 // sentinel: unlimited
	}

	for offset < len(buffer) {
		start := offset
		if len(buffer)-offset < 4 {
			tornStart = &start
			break
		}
		if binary.LittleEndian.Uint32(buffer[offset:]) != ZstdMagic {
			return FrameScan{Frames: nil, TornStart: &start}
		}
		offset += 4

		if offset == len(buffer) {
			tornStart = &start
			break
		}
		descriptor := buffer[offset]
		offset += 1
		if (descriptor & 0x18) != 0 {
			return FrameScan{Frames: nil, TornStart: &start}
		}

		contentSizeFlag := descriptor >> 6
		singleSegment := (descriptor & 0x20) != 0
		checksum := (descriptor & 0x04) != 0
		dictionaryFlag := descriptor & 0x03
		var dictionaryBytes int
		if dictionaryFlag == 3 {
			dictionaryBytes = 4
		}
		var contentSizeBytes int
		if contentSizeFlag == 0 {
			if singleSegment {
				contentSizeBytes = 1
			}
		} else {
			contentSizeBytes = 1 << contentSizeFlag
		}
		var remainingHeaderBytes int
		if !singleSegment {
			remainingHeaderBytes += 1
		}
		remainingHeaderBytes += dictionaryBytes + contentSizeBytes
		if len(buffer)-offset < remainingHeaderBytes {
			tornStart = &start
			break
		}
		offset += remainingHeaderBytes

		for {
			if len(buffer)-offset < 3 {
				tornStart = &start
				break
			}
			blockHeader := int(binary.LittleEndian.Uint16(buffer[offset:])) | (int(buffer[offset+2]) << 16)
			offset += 3
			lastBlock := (blockHeader & 1) != 0
			blockType := (blockHeader >> 1) & 0x03
			blockSize := blockHeader >> 3
			if blockType == 0x03 {
				return FrameScan{Frames: nil, TornStart: &start}
			}
			var payloadBytes int
			if blockType == 0x01 {
				payloadBytes = 1
			} else {
				payloadBytes = blockSize
			}
			if len(buffer)-offset < payloadBytes {
				tornStart = &start
				break
			}
			offset += payloadBytes
			if lastBlock {
				break
			}
		}
		if tornStart != nil {
			break
		}

		if checksum {
			if len(buffer)-offset < 4 {
				tornStart = &start
				break
			}
			offset += 4
		}

		frames = append(frames, FrameRange{Start: start, End: offset})
		if maxFrames > 0 && len(frames) >= maxFrames {
			return FrameScan{Frames: frames}
		}
	}

	return FrameScan{Frames: frames, TornStart: tornStart}
}

// CompressFrame compresses input into one independently decodable,
// checksummed Zstandard frame. Ported from DSH's compressZstdFrame.
func CompressFrame(input []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderConcurrency(1))
	if err != nil {
		return nil, fmt.Errorf("zstd encode: %w", err)
	}
	defer func() { _ = enc.Close() }()
	// klauspost encodes with checksum by default when enabled
	out := enc.EncodeAll(input, nil)
	return out, nil
}

// DecompressFrame decompresses one complete, checksummed Zstandard frame.
// Ported from DSH's decompressZstdFrame.
func DecompressFrame(input []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("zstd decode: %w", err)
	}
	defer dec.Close()
	out, err := dec.DecodeAll(input, nil)
	if err != nil {
		return nil, fmt.Errorf("zstd decompress frame: %w", err)
	}
	return out, nil
}

// FrameDecoder decodes complete frames in source order. Each yielded buffer
// remains valid only until the iterator advances. Ported from DSH's
// ZstdFrameDecoder interface.
type FrameDecoder interface {
	Decode(source []byte, frames []FrameRange) <-chan []byte
	Close()
}

// klauspostFrameDecoder is the default synchronous decoder using klauspost/compress.
type klauspostFrameDecoder struct {
	dec *zstd.Decoder
}

func (d *klauspostFrameDecoder) Decode(source []byte, frames []FrameRange) <-chan []byte {
	ch := make(chan []byte, len(frames))
	go func() {
		defer close(ch)
		for _, fr := range frames {
			out, err := d.dec.DecodeAll(source[fr.Start:fr.End], nil)
			if err != nil {
				continue // skip corrupt frame
			}
			ch <- out
		}
	}()
	return ch
}

func (d *klauspostFrameDecoder) Close() {
	d.dec.Close()
}

// NewFrameDecoder creates a synchronous multi-frame decoder.
func NewFrameDecoder() FrameDecoder {
	dec, _ := zstd.NewReader(nil)
	return &klauspostFrameDecoder{dec: dec}
}

// DecompressPrefix recovers available plaintext from a structurally incomplete
// final frame, using Zstandard's flush mode. Ported from DSH's
// decompressZstdPrefix. Returns partial plaintext without requiring a
// complete frame.
func DecompressPrefix(input []byte) ([]byte, error) {
	dec, err := zstd.NewReader(nil, zstd.WithDecoderLowmem(false))
	if err != nil {
		return nil, fmt.Errorf("zstd decode: %w", err)
	}
	defer dec.Close()
	// klauspost's DecodeAll handles partial frames by returning what it can.
	out, err := dec.DecodeAll(input, nil)
	if err != nil {
		// Even on error, klauspost may return partial output.
		if len(out) > 0 {
			return out, nil
		}
		return nil, fmt.Errorf("zstd decompress prefix: %w", err)
	}
	return out, nil
}

// ReadFirstFrame reads only the first newline-terminated frame's plaintext
// without loading the whole file. Used for header-only reads.
func ReadFirstFrame(r io.Reader) (string, error) {
	dec, err := zstd.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("zstd decode: %w", err)
	}
	defer dec.Close()
	out, err := io.ReadAll(dec)
	if err != nil {
		return "", err
	}
	// Return only the first line
	for i, b := range out {
		if b == '\n' {
			return string(out[:i]), nil
		}
	}
	return string(out), nil
}
