package parser

import (
	"bytes"
	"crypto/aes"
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

func TestBuildKeyBox(t *testing.T) {
	key := []byte("test_key_12345")
	box := buildKeyBox(key)
	if len(box) != 256 {
		t.Fatalf("expected box length 256, got %d", len(box))
	}

	// Verify permutations contain all bytes from 0 to 255
	seen := make(map[byte]bool)
	for _, b := range box {
		seen[b] = true
	}
	if len(seen) != 256 {
		t.Errorf("expected 256 unique bytes in key box, got %d", len(seen))
	}
}

func TestReadLenAndData(t *testing.T) {
	buf := new(bytes.Buffer)
	buf.Write([]byte{0x04, 0x00, 0x00, 0x00}) // length 4 (little endian)
	buf.Write([]byte("test"))

	data, err := readLenAndData(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "test" {
		t.Errorf("expected 'test', got '%s'", string(data))
	}
}

func TestDecryptAes128Ecb(t *testing.T) {
	key := aesCoreKey
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("unexpected cipher creation error: %v", err)
	}

	plain := []byte("hello_world!1234") // 12 bytes + 4 bytes padding (0x04)
	copy(plain[12:], []byte{0x04, 0x04, 0x04, 0x04})

	encrypted := make([]byte, len(plain))
	block.Encrypt(encrypted, plain)

	decrypted, err := decryptAes128Ecb(key, encrypted)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decrypted) != "hello_world!" {
		t.Fatalf("expected 'hello_world!', got '%s'", string(decrypted))
	}
}
