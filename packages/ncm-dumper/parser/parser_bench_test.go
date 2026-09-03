package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/binary"
	"testing"
)

func createMockNCMData() []byte {
	var buf bytes.Buffer

	// 1. Header (10 bytes)
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0x01, 0x00})

	// 2. Key Data
	rawKey := []byte("neteasecloudmusic1234567890123456") // 33 bytes
	// AES block size is 16. Pad to 48 bytes (3 blocks of 16)
	paddedKey := append(rawKey, bytes.Repeat([]byte{15}, 15)...)
	block, _ := aes.NewCipher(aesCoreKey)
	encKey := make([]byte, len(paddedKey))
	for i := 0; i < len(paddedKey); i += 16 {
		block.Encrypt(encKey[i:i+16], paddedKey[i:i+16])
	}
	for i := range encKey {
		encKey[i] ^= 0x64
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(encKey)))
	buf.Write(lenBuf[:])
	buf.Write(encKey)

	// 3. Metadata (empty - 0 length)
	binary.LittleEndian.PutUint32(lenBuf[:], 0)
	buf.Write(lenBuf[:])

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover (empty - 0 length)
	binary.LittleEndian.PutUint32(lenBuf[:], 0)
	buf.Write(lenBuf[:])

	// 6. Audio stream data
	buf.Write(make([]byte, 1024))

	return buf.Bytes()
}

func BenchmarkSequentialNCMParser_Parse(b *testing.B) {
	mockData := createMockNCMData()
	parser := &SequentialNCMParser{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(mockData)
		_, err := parser.Parse(r)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

func BenchmarkDecryptReader(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB of data
	var xorLookup [256]byte
	for i := range xorLookup {
		xorLookup[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		dr := &DecryptReader{
			r:         r,
			xorLookup: &xorLookup,
		}
		buf := make([]byte, 4096)
		for {
			_, err := dr.Read(buf)
			if err != nil {
				break
			}
		}
	}
}
