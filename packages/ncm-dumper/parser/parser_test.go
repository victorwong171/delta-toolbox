package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
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

// Helper functions to generate a valid mock NCM stream programmatically for testing/benchmarking.

func pkcs7Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func encryptAes128Ecb(key, data []byte) ([]byte, error) {
	padded := pkcs7Padding(data, aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	encrypted := make([]byte, len(padded))
	bs := block.BlockSize()
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(encrypted[i:i+bs], padded[i:i+bs])
	}
	return encrypted, nil
}

func xorBytesTest(data []byte, val uint8) {
	for i := range data {
		data[i] ^= val
	}
}

func generateMockMetadata() ([]byte, error) {
	rawJSON := []byte(`{"musicId": 12345, "musicName": "Test Song", "format": "flac"}`)
	prefixed := append([]byte("music:"), rawJSON...)
	encrypted, err := encryptAes128Ecb(aesModifyKey, prefixed)
	if err != nil {
		return nil, err
	}
	b64Bytes := []byte(base64.StdEncoding.EncodeToString(encrypted))
	fullModifyData := append([]byte("163 key(Don't modify):"), b64Bytes...)
	xorBytesTest(fullModifyData, 0x63)
	return fullModifyData, nil
}

func generateMockNCM() ([]byte, error) {
	var buf bytes.Buffer

	// 1. Header (10 bytes)
	buf.Write([]byte("CTENFDAM\x01\x02"))

	// 2. Key block (length + encrypted/XORed key data)
	deKeyData := []byte("neteasecloudmusicMySecretRC4Key!!")
	encryptedKey, err := encryptAes128Ecb(aesCoreKey, deKeyData)
	if err != nil {
		return nil, err
	}
	xorBytesTest(encryptedKey, 0x64)

	keyLen := uint32(len(encryptedKey))
	binary.Write(&buf, binary.LittleEndian, keyLen)
	buf.Write(encryptedKey)

	// 3. Metadata block (length + encrypted/XORed metadata JSON)
	metaData, err := generateMockMetadata()
	if err != nil {
		return nil, err
	}
	metaLen := uint32(len(metaData))
	binary.Write(&buf, binary.LittleEndian, metaLen)
	buf.Write(metaData)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover (optional, let's write 4 bytes of dummy cover data)
	coverBytes := []byte("jpeg")
	coverLen := uint32(len(coverBytes))
	binary.Write(&buf, binary.LittleEndian, coverLen)
	buf.Write(coverBytes)

	// 6. Audio stream
	buf.Write([]byte("this is some dummy encrypted audio data"))

	return buf.Bytes(), nil
}

func TestSequentialNCMParser(t *testing.T) {
	mockNCM, err := generateMockNCM()
	if err != nil {
		t.Fatalf("failed to generate mock NCM: %v", err)
	}

	p := &SequentialNCMParser{}
	parsed, err := p.Parse(bytes.NewReader(mockNCM))
	if err != nil {
		t.Fatalf("failed to parse mock NCM: %v", err)
	}

	if parsed.AudioFormat() != "flac" {
		t.Errorf("expected format flac, got %s", parsed.AudioFormat())
	}

	if parsed.Metadata().Name != "Test Song" {
		t.Errorf("expected song name 'Test Song', got %s", parsed.Metadata().Name)
	}

	if string(parsed.Cover()) != "jpeg" {
		t.Errorf("expected cover 'jpeg', got %s", string(parsed.Cover()))
	}

	decrypted, err := io.ReadAll(parsed.DecryptedStream())
	if err != nil {
		t.Fatalf("failed to read decrypted stream: %v", err)
	}

	if len(decrypted) != len("this is some dummy encrypted audio data") {
		t.Errorf("expected decrypted size %d, got %d", len("this is some dummy encrypted audio data"), len(decrypted))
	}
}
