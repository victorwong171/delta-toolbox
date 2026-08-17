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
		{"Medium (15 bytes)", 15},
		{"Large (1024 bytes)", 1024},
		{"Non-multiple of 8 (1029 bytes)", 1029},
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

func TestReadLenAndData(t *testing.T) {
	buf := new(bytes.Buffer)
	buf.Write([]byte{0x04, 0x00, 0x00, 0x00}) // Little endian uint32 = 4
	buf.Write([]byte("test"))

	data, err := readLenAndData(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "test" {
		t.Fatalf("expected 'test', got '%s'", string(data))
	}
}

func TestBuildKeyBox(t *testing.T) {
	key := []byte("secret_key_123")
	box := buildKeyBox(key)
	if len(box) != 256 {
		t.Fatalf("expected box length 256, got %d", len(box))
	}
}

func TestSequentialNCMParser(t *testing.T) {
	streamData := buildMockNCMStream()
	parser := &SequentialNCMParser{}
	parsed, err := parser.Parse(bytes.NewReader(streamData))
	if err != nil {
		t.Fatalf("SequentialNCMParser.Parse failed: %v", err)
	}

	meta := parsed.Metadata()
	if meta == nil || meta.Name != "Test Song" {
		t.Fatalf("expected musicName 'Test Song', got: %+v", meta)
	}

	if parsed.AudioFormat() != "mp3" {
		t.Fatalf("expected audioFormat 'mp3', got '%s'", parsed.AudioFormat())
	}

	if len(parsed.Cover()) == 0 {
		t.Fatalf("expected non-empty cover data")
	}

	audioStream := parsed.DecryptedStream()
	audioBuf := make([]byte, 1024)
	n, err := audioStream.Read(audioBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("failed reading decrypted stream: %v", err)
	}
	if n != 1024 {
		t.Fatalf("expected 1024 bytes read, got %d", n)
	}
}
