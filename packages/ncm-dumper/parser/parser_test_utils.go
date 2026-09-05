package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
)

// generateMockNCMStream creates a valid mock NCM binary stream for unit testing and benchmarking
func generateMockNCMStream(audioLen int) []byte {
	var buf bytes.Buffer

	// 1. Header (10 bytes)
	buf.WriteString("CTENFDAM")
	buf.Write([]byte{0x01, 0x01})

	// 2. Key data (length + 32 bytes encrypted key data)
	keyRaw := make([]byte, 32)
	// "neteasecloudmusic" is 17 bytes prefix
	copy(keyRaw, []byte("neteasecloudmusic"))
	for i := 17; i < 32; i++ {
		keyRaw[i] = byte(i)
	}

	// Encrypt key with aesCoreKey
	block, _ := aes.NewCipher(aesCoreKey)
	bs := block.BlockSize()
	// Pad PKCS7 to 48 bytes (3 blocks of 16)
	encKeyRaw := make([]byte, 48)
	copy(encKeyRaw, keyRaw)
	for i := 32; i < 48; i++ {
		encKeyRaw[i] = 16 // 16 bytes of PKCS7 padding
	}

	encKey := make([]byte, 48)
	for i := 0; i <= len(encKey)-bs; i += bs {
		block.Encrypt(encKey[i:i+bs], encKeyRaw[i:i+bs])
	}
	// XOR with 0x64
	for i := range encKey {
		encKey[i] ^= 0x64
	}

	keyLenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(keyLenBuf, uint32(len(encKey)))
	buf.Write(keyLenBuf)
	buf.Write(encKey)

	// 3. Metadata (length + encrypted metadata)
	metaJSON := `{"musicId":123,"musicName":"Test Song","artist":[["Test Artist",1]],"format":"mp3"}`
	metaRaw := append([]byte("music:"), []byte(metaJSON)...)

	// Encrypt metaRaw with aesModifyKey
	blockMod, _ := aes.NewCipher(aesModifyKey)
	padLen := bs - (len(metaRaw) % bs)
	if padLen == 0 {
		padLen = bs
	}
	encMetaRaw := make([]byte, len(metaRaw)+padLen)
	copy(encMetaRaw, metaRaw)
	for i := len(metaRaw); i < len(encMetaRaw); i++ {
		encMetaRaw[i] = byte(padLen)
	}

	encMeta := make([]byte, len(encMetaRaw))
	for i := 0; i <= len(encMeta)-bs; i += bs {
		blockMod.Encrypt(encMeta[i:i+bs], encMetaRaw[i:i+bs])
	}
	// Base64 encode
	b64Meta := base64.StdEncoding.EncodeToString(encMeta)
	fullMeta := append([]byte("163 key(Don't modify):"), []byte(b64Meta)...)
	// XOR with 0x63
	for i := range fullMeta {
		fullMeta[i] ^= 0x63
	}

	metaLenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(metaLenBuf, uint32(len(fullMeta)))
	buf.Write(metaLenBuf)
	buf.Write(fullMeta)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover (length + cover bytes)
	coverData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46} // Fake JPEG header
	coverLenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(coverLenBuf, uint32(len(coverData)))
	buf.Write(coverLenBuf)
	buf.Write(coverData)

	// 6. Audio stream
	audioData := make([]byte, audioLen)
	for i := range audioData {
		audioData[i] = byte(i)
	}
	buf.Write(audioData)

	return buf.Bytes()
}
