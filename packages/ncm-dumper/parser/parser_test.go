package parser

import (
	"bytes"
	"io"
	"testing"
)

// ReferenceDecryptReader is a simple, straightforward reference implementation of DecryptReader
type ReferenceDecryptReader struct {
	r            io.Reader
	xorLookup    *[256]byte
	streamOffset int
}

func (r *ReferenceDecryptReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if n > 0 {
		offset := byte(r.streamOffset)
		for i := 0; i < n; i++ {
			offset++
			p[i] ^= r.xorLookup[offset]
		}
		r.streamOffset += n
	}
	return
}

func TestDecryptReaderCorrectness(t *testing.T) {
	var xorLookup [256]byte
	for i := range xorLookup {
		xorLookup[i] = byte(i * 3) // some dummy non-trivial lookup values
	}

	testCases := []struct {
		name string
		size int
	}{
		{"Empty", 0},
		{"Tiny (3 bytes)", 3},
		{"Exactly 8 bytes", 8},
		{"Exactly 16 bytes", 16},
		{"Medium (15 bytes)", 15},
		{"Medium (25 bytes)", 25},
		{"Large (1024 bytes)", 1024},
		{"Non-multiple of 16 (1029 bytes)", 1029},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Generate some dummy source data
			source := make([]byte, tc.size)
			for i := range source {
				source[i] = byte(i)
			}

			// Run optimized DecryptReader
			r1 := bytes.NewReader(source)
			dr1 := &DecryptReader{
				r:         r1,
				xorLookup: &xorLookup,
			}
			dest1 := make([]byte, tc.size)
			_, _ = io.ReadFull(dr1, dest1)

			// Run reference DecryptReader
			r2 := bytes.NewReader(source)
			dr2 := &ReferenceDecryptReader{
				r:         r2,
				xorLookup: &xorLookup,
			}
			dest2 := make([]byte, tc.size)
			_, _ = io.ReadFull(dr2, dest2)

			// Compare results
			if !bytes.Equal(dest1, dest2) {
				t.Errorf("optimized DecryptReader output does not match reference for size %d", tc.size)
			}
		})
	}
}

func TestSequentialNCMParser(t *testing.T) {
	mockStream := generateMockNCMStream(1024)
	parser := &SequentialNCMParser{}

	parsed, err := parser.Parse(bytes.NewReader(mockStream))
	if err != nil {
		t.Fatalf("failed to parse mock NCM stream: %v", err)
	}

	if parsed.Metadata() == nil || parsed.Metadata().Name != "Test Song" {
		t.Errorf("expected song name 'Test Song', got %v", parsed.Metadata())
	}

	if parsed.AudioFormat() != "mp3" {
		t.Errorf("expected format 'mp3', got %s", parsed.AudioFormat())
	}

	if len(parsed.Cover()) == 0 {
		t.Errorf("expected cover data, got empty")
	}

	audioBuf := make([]byte, 1024)
	n, err := io.ReadFull(parsed.DecryptedStream(), audioBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("failed to read decrypted stream: %v", err)
	}
	if n != 1024 {
		t.Errorf("expected 1024 decrypted bytes, got %d", n)
	}
}
