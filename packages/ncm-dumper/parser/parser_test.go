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

func TestSequentialNCMParser(t *testing.T) {
	rc4Key := "test_key_12345"
	metaJSON := `{"musicId":999,"musicName":"Test Song","album":"Awesome Album","artist":[["Singer A",1001]],"format":"flac"}`
	cover := []byte("fake_image_bytes")
	audio := []byte("this is some mock encrypted audio data")

	ncmBytes, err := generateMockNCMData(rc4Key, metaJSON, cover, audio)
	if err != nil {
		t.Fatalf("failed to generate mock NCM data: %v", err)
	}

	parser := &SequentialNCMParser{}
	parsed, err := parser.Parse(bytes.NewReader(ncmBytes))
	if err != nil {
		t.Fatalf("failed to parse NCM: %v", err)
	}

	if parsed.AudioFormat() != "flac" {
		t.Errorf("expected format flac, got %s", parsed.AudioFormat())
	}

	if parsed.Metadata().Name != "Test Song" {
		t.Errorf("expected song name 'Test Song', got %s", parsed.Metadata().Name)
	}

	if len(parsed.Metadata().Artists) != 1 || parsed.Metadata().Artists[0].Name != "Singer A" {
		t.Errorf("expected artist 'Singer A', got %v", parsed.Metadata().Artists)
	}

	if !bytes.Equal(parsed.Cover(), cover) {
		t.Errorf("cover data mismatch")
	}

	decrypted, err := io.ReadAll(parsed.DecryptedStream())
	if err != nil {
		t.Fatalf("failed to read decrypted stream: %v", err)
	}

	// Verify that DecryptReader decrypted the stream correctly
	var xorLookup [256]byte
	box := buildKeyBox([]byte(rc4Key))
	for j := 0; j < 256; j++ {
		bj := byte(j)
		xorLookup[bj] = box[(box[bj]+box[(box[bj]+bj)&0xff])&0xff]
	}

	refReader := &ReferenceDecryptReader{
		r:         bytes.NewReader(audio),
		xorLookup: &xorLookup,
	}
	expectedDecrypted, err := io.ReadAll(refReader)
	if err != nil {
		t.Fatalf("failed to read from reference reader: %v", err)
	}

	if !bytes.Equal(decrypted, expectedDecrypted) {
		t.Errorf("decrypted audio mismatch")
	}
}
