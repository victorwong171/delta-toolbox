package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

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

func createMockNCMData() []byte {
	var buf bytes.Buffer
	// 1. Magic
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0, 0})

	// 2. AES Key. Let's make an encrypted key
	rawKey := []byte("neteasecloudmusic123456")
	padLen := 32 - len(rawKey)
	paddedKey := make([]byte, 32)
	copy(paddedKey, rawKey)
	for i := len(rawKey); i < 32; i++ {
		paddedKey[i] = byte(padLen)
	}

	aesCoreBlock, _ := aes.NewCipher(aesCoreKey)
	encryptedKey := make([]byte, 32)
	aesCoreBlock.Encrypt(encryptedKey[:16], paddedKey[:16])
	aesCoreBlock.Encrypt(encryptedKey[16:], paddedKey[16:])

	for i := range encryptedKey {
		encryptedKey[i] ^= 0x64
	}

	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(encryptedKey)))
	buf.Write(lenBuf[:])
	buf.Write(encryptedKey)

	// 3. Meta data.
	metaRaw := []byte("music:{\"musicId\":1234,\"musicName\":\"Test Song\",\"format\":\"mp3\"}")
	paddedMeta := make([]byte, 64)
	copy(paddedMeta, metaRaw)
	for i := len(metaRaw); i < 64; i++ {
		paddedMeta[i] = byte(64 - len(metaRaw))
	}

	aesModifyBlock, _ := aes.NewCipher(aesModifyKey)
	encryptedMeta := make([]byte, 64)
	aesModifyBlock.Encrypt(encryptedMeta[:16], paddedMeta[:16])
	aesModifyBlock.Encrypt(encryptedMeta[16:32], paddedMeta[16:32])
	aesModifyBlock.Encrypt(encryptedMeta[32:48], paddedMeta[32:48])
	aesModifyBlock.Encrypt(encryptedMeta[48:], paddedMeta[48:])

	b64Meta := make([]byte, base64.StdEncoding.EncodedLen(len(encryptedMeta)))
	base64.StdEncoding.Encode(b64Meta, encryptedMeta)

	prefix := []byte("163 key(Don't modify):")
	metaFull := append(prefix, b64Meta...)

	for i := range metaFull {
		metaFull[i] ^= 0x63
	}

	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(metaFull)))
	buf.Write(lenBuf[:])
	buf.Write(metaFull)

	// 4. Gap
	buf.Write(make([]byte, 9))

	// 5. Cover data
	binary.LittleEndian.PutUint32(lenBuf[:], 0)
	buf.Write(lenBuf[:])

	// 6. Audio stream
	buf.Write([]byte("some encrypted audio stream bytes..."))

	return buf.Bytes()
}

func BenchmarkSequentialParserParse(b *testing.B) {
	ncmData := createMockNCMData()
	parser := &SequentialNCMParser{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(ncmData)
		_, err := parser.Parse(r)
		if err != nil {
			b.Fatalf("failed to parse: %v", err)
		}
	}
}
