package parser

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"io"
	"testing"
)

// pkcs7Padding pads data to a multiple of blockSize.
func pkcs7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// aesEncryptECB encrypts data in ECB mode.
func aesEncryptECB(key, data []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	data = pkcs7Padding(data, aes.BlockSize)
	encrypted := make([]byte, len(data))
	bs := block.BlockSize()
	for i := 0; i < len(data); i += bs {
		block.Encrypt(encrypted[i:i+bs], data[i:i+bs])
	}
	return encrypted
}

// generateMockNCM generates a valid NCM byte stream for testing/benchmarking.
func generateMockNCM(rc4Key []byte, metaJSON string, coverData []byte, audioData []byte) []byte {
	buf := new(bytes.Buffer)

	// 1. Header (10 bytes: magic "CTENFDAM" + 2 bytes padding)
	buf.Write([]byte("CTENFDAM"))
	buf.Write([]byte{0x01, 0x02})

	// 2. Encrypted keyData
	// Decrypted key format: "neteasecloudmusic" + rc4Key
	keyPlain := append([]byte("neteasecloudmusic"), rc4Key...)
	keyEnc := aesEncryptECB(aesCoreKey, keyPlain)
	for i := range keyEnc {
		keyEnc[i] ^= 0x64
	}
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(keyEnc)))
	buf.Write(keyEnc)

	// 3. Encrypted metadata
	// Decrypted metadata format: "music:" + JSON
	if metaJSON != "" {
		metaPlain := append([]byte("music:"), []byte(metaJSON)...)
		metaEnc := aesEncryptECB(aesModifyKey, metaPlain)
		b64 := base64.StdEncoding.EncodeToString(metaEnc)
		modifyData := append([]byte("163 key(Don't modify):"), []byte(b64)...)
		for i := range modifyData {
			modifyData[i] ^= 0x63
		}
		_ = binary.Write(buf, binary.LittleEndian, uint32(len(modifyData)))
		buf.Write(modifyData)
	} else {
		_ = binary.Write(buf, binary.LittleEndian, uint32(0))
	}

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover data
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(coverData)))
	buf.Write(coverData)

	// 6. Audio data
	buf.Write(audioData)

	return buf.Bytes()
}

func TestParse(t *testing.T) {
	rc4Key := []byte("verysecret_rc4_key")
	metaJSON := `{"musicId":99999,"musicName":"HyperDrive","format":"flac"}`
	coverData := []byte("somefakejpegimagedata")

	// Generate raw audio data and encrypt it manually using the expected RC4 lookup table
	rawAudio := []byte("this is some ultra high fidelity audio stream data in flac format!")
	box := buildKeyBox(rc4Key)
	var xorLookup [256]byte
	for j := 0; j < 256; j++ {
		bj := byte(j)
		xorLookup[bj] = box[(box[bj]+box[(box[bj]+bj)&0xff])&0xff]
	}

	encryptedAudio := make([]byte, len(rawAudio))
	copy(encryptedAudio, rawAudio)
	for i := range encryptedAudio {
		encryptedAudio[i] ^= xorLookup[byte(i+1)]
	}

	mockNCMBytes := generateMockNCM(rc4Key, metaJSON, coverData, encryptedAudio)

	parser := &SequentialNCMParser{}
	parsed, err := parser.Parse(bytes.NewReader(mockNCMBytes))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.AudioFormat() != "flac" {
		t.Errorf("expected format flac, got %s", parsed.AudioFormat())
	}

	if !bytes.Equal(parsed.Cover(), coverData) {
		t.Errorf("cover data mismatch")
	}

	meta := parsed.Metadata()
	if meta == nil {
		t.Fatalf("metadata is nil")
	}
	if meta.Name != "HyperDrive" {
		t.Errorf("expected musicName HyperDrive, got %s", meta.Name)
	}

	// Verify decrypted stream matches raw audio
	decryptedAudio, err := io.ReadAll(parsed.DecryptedStream())
	if err != nil {
		t.Fatalf("failed to read decrypted stream: %v", err)
	}

	if !bytes.Equal(decryptedAudio, rawAudio) {
		t.Errorf("decrypted audio does not match original raw audio")
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
