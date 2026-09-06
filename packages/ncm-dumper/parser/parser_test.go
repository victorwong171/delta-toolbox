package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"testing"
)

func createMockNCMData() []byte {
	var buf bytes.Buffer

	// 1. Header (10 bytes)
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0x00, 0x00})

	// 2. Key data
	// Plain decrypted key: 17 bytes prefix + 16 bytes rc4 key
	plainKey := append([]byte("neteasecloudmusic"), []byte("0123456789abcdef")...)
	padLen := 48 - len(plainKey)
	paddedKey := make([]byte, 48)
	copy(paddedKey, plainKey)
	for i := len(plainKey); i < 48; i++ {
		paddedKey[i] = byte(padLen)
	}

	block, _ := aes.NewCipher(aesCoreKey)
	encryptedKey := make([]byte, 48)
	for i := 0; i < 48; i += 16 {
		block.Encrypt(encryptedKey[i:i+16], paddedKey[i:i+16])
	}
	for i := range encryptedKey {
		encryptedKey[i] ^= 0x64
	}

	var keyLenBuf [4]byte
	binary.LittleEndian.PutUint32(keyLenBuf[:], uint32(len(encryptedKey)))
	buf.Write(keyLenBuf[:])
	buf.Write(encryptedKey)

	// 3. Metadata
	jsonMeta := []byte(`{"musicName":"TestSong","artist":[["TestArtist",123]],"format":"mp3"}`)
	plainMeta := append([]byte("music:"), jsonMeta...)
	metaPadLen := 16 - (len(plainMeta) % 16)
	paddedMeta := make([]byte, len(plainMeta)+metaPadLen)
	copy(paddedMeta, plainMeta)
	for i := len(plainMeta); i < len(paddedMeta); i++ {
		paddedMeta[i] = byte(metaPadLen)
	}

	metaBlock, _ := aes.NewCipher(aesModifyKey)
	encryptedMeta := make([]byte, len(paddedMeta))
	for i := 0; i < len(paddedMeta); i += 16 {
		metaBlock.Encrypt(encryptedMeta[i:i+16], paddedMeta[i:i+16])
	}

	b64Meta := []byte(base64.StdEncoding.EncodeToString(encryptedMeta))
	prefix := []byte("163 key(Don't modify):")
	modifyData := append(prefix, b64Meta...)
	for i := range modifyData {
		modifyData[i] ^= 0x63
	}

	var metaLenBuf [4]byte
	binary.LittleEndian.PutUint32(metaLenBuf[:], uint32(len(modifyData)))
	buf.Write(metaLenBuf[:])
	buf.Write(modifyData)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover (0 bytes len)
	var coverLenBuf [4]byte
	binary.LittleEndian.PutUint32(coverLenBuf[:], 0)
	buf.Write(coverLenBuf[:])

	// 6. Audio data
	buf.Write(make([]byte, 1024))

	return buf.Bytes()
}

func TestSequentialNCMParser(t *testing.T) {
	mockData := createMockNCMData()
	parser := &SequentialNCMParser{}
	parsed, err := parser.Parse(bytes.NewReader(mockData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Metadata().Name != "TestSong" {
		t.Errorf("expected song name TestSong, got %s", parsed.Metadata().Name)
	}
	if parsed.AudioFormat() != "mp3" {
		t.Errorf("expected format mp3, got %s", parsed.AudioFormat())
	}

	stream := parsed.DecryptedStream()
	audioBuf, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("failed to read decrypted stream: %v", err)
	}
	if len(audioBuf) != 1024 {
		t.Errorf("expected 1024 audio bytes, got %d", len(audioBuf))
	}
}

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
