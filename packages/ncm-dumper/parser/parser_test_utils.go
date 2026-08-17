package parser

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
)

func createPKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// buildMockNCMStream constructs a valid NCM binary stream for testing and benchmarking
func buildMockNCMStream() []byte {
	buf := new(bytes.Buffer)

	// 1. Header (10 bytes: 8 bytes magic "CTENFDAM" + 2 gap bytes)
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0x01, 0x02})

	// 2. Encrypted Key
	// Raw key payload with "neteasecloudmusic" prefix (17 bytes)
	rawKeyPayload := append([]byte("neteasecloudmusic"), []byte("0123456789abcdef")...)
	paddedKeyPayload := createPKCS7Padding(rawKeyPayload, 16)
	encKey := make([]byte, len(paddedKeyPayload))
	bs := aesCoreBlock.BlockSize()
	for i := 0; i <= len(paddedKeyPayload)-bs; i += bs {
		aesCoreBlock.Encrypt(encKey[i:i+bs], paddedKeyPayload[i:i+bs])
	}
	xorBytes(encKey, 0x64)

	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(encKey)))
	buf.Write(lenBuf[:])
	buf.Write(encKey)

	// 3. Metadata
	jsonMeta := `{"musicId":123,"musicName":"Test Song","album":"Test Album","artist":[["Test Artist",1]],"format":"mp3"}`
	metaPayload := append([]byte("music:"), []byte(jsonMeta)...)
	paddedMetaPayload := createPKCS7Padding(metaPayload, 16)
	encMeta := make([]byte, len(paddedMetaPayload))
	for i := 0; i <= len(paddedMetaPayload)-bs; i += bs {
		aesModifyBlock.Encrypt(encMeta[i:i+bs], paddedMetaPayload[i:i+bs])
	}
	b64Meta := base64.StdEncoding.EncodeToString(encMeta)
	modifyData := append([]byte("163 key(Don't modify):"), []byte(b64Meta)...)
	xorBytes(modifyData, 0x63)

	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(modifyData)))
	buf.Write(lenBuf[:])
	buf.Write(modifyData)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover Image Data
	coverData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(coverData)))
	buf.Write(lenBuf[:])
	buf.Write(coverData)

	// 6. Encrypted Audio payload
	audioData := make([]byte, 1024)
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}
	buf.Write(audioData)

	return buf.Bytes()
}
