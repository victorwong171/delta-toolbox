package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
)

// pkcs7Padding pad bytes
func pkcs7Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

// encryptAes128Ecb encrypts with AES-128-ECB
func encryptAes128Ecb(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Padding(data, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	bs := block.BlockSize()
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(encrypted[i:i+bs], padded[i:i+bs])
	}
	return encrypted, nil
}

// generateMockNCMData creates a valid NCM byte stream
func generateMockNCMData(rc4KeyStr string, metaJSON string, coverBytes []byte, audioBytes []byte) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Header (8 bytes signature + 2 bytes padding)
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0x01, 0x02})

	// 2. Key Data
	// Form plaintext: "neteasecloudmusic" + rc4KeyStr
	plaintextKey := append([]byte("neteasecloudmusic"), []byte(rc4KeyStr)...)
	encryptedKey, err := encryptAes128Ecb(aesCoreKey, plaintextKey)
	if err != nil {
		return nil, err
	}
	// XOR with 0x64
	for i := range encryptedKey {
		encryptedKey[i] ^= 0x64
	}
	// Write length (uint32) + data
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(encryptedKey))); err != nil {
		return nil, err
	}
	buf.Write(encryptedKey)

	// 3. Metadata
	// Prepend "music:" to metadata bytes
	plaintextMeta := append([]byte("music:"), []byte(metaJSON)...)
	encryptedMeta, err := encryptAes128Ecb(aesModifyKey, plaintextMeta)
	if err != nil {
		return nil, err
	}
	// Base64 encode
	base64Meta := make([]byte, base64.StdEncoding.EncodedLen(len(encryptedMeta)))
	base64.StdEncoding.Encode(base64Meta, encryptedMeta)
	// Prepend "163 key(Don't modify):"
	prefixMeta := append([]byte("163 key(Don't modify):"), base64Meta...)
	// XOR with 0x63
	for i := range prefixMeta {
		prefixMeta[i] ^= 0x63
	}
	// Write length (uint32) + data
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(prefixMeta))); err != nil {
		return nil, err
	}
	buf.Write(prefixMeta)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover Data
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(coverBytes))); err != nil {
		return nil, err
	}
	buf.Write(coverBytes)

	// 6. Audio Data
	buf.Write(audioBytes)

	return buf.Bytes(), nil
}
