package parser

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
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

func PKCS7Padding(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func encryptAes128EcbLocal(block cipher.Block, data []byte) []byte {
	data = PKCS7Padding(data, aes.BlockSize)
	encrypted := make([]byte, len(data))
	bs := block.BlockSize()
	for i := 0; i < len(data); i += bs {
		block.Encrypt(encrypted[i:i+bs], data[i:i+bs])
	}
	return encrypted
}

func generateMockNCMData() ([]byte, error) {
	var buf bytes.Buffer

	// 1. Header (10 bytes)
	buf.WriteString("CTENFDAM\x01\x02")

	// 2. Key Data
	rc4Key := []byte("1234567890123456")
	deKeyData := append([]byte("neteasecloudmusic"), rc4Key...)
	keyDataEnc := encryptAes128EcbLocal(aesCoreBlock, deKeyData)
	for i := range keyDataEnc {
		keyDataEnc[i] ^= 0x64
	}
	// Write length
	err := binary.Write(&buf, binary.LittleEndian, uint32(len(keyDataEnc)))
	if err != nil {
		return nil, err
	}
	buf.Write(keyDataEnc)

	// 3. Metadata (modifyData)
	metaJSON := []byte(`{"musicName":"Test Song","format":"mp3","artist":[["Singer",123]]}`)
	deModifyData := append([]byte("music:"), metaJSON...)
	modifyDataEnc := encryptAes128EcbLocal(aesModifyBlock, deModifyData)
	b64Str := base64.StdEncoding.EncodeToString(modifyDataEnc)
	modifyData := append([]byte("163 key(Don't modify):"), []byte(b64Str)...)
	for i := range modifyData {
		modifyData[i] ^= 0x63
	}
	// Write length
	err = binary.Write(&buf, binary.LittleEndian, uint32(len(modifyData)))
	if err != nil {
		return nil, err
	}
	buf.Write(modifyData)

	// 4. Gap (9 bytes)
	buf.Write(make([]byte, 9))

	// 5. Cover (optional, let's write 10 bytes of cover data)
	coverData := []byte("coverimage")
	err = binary.Write(&buf, binary.LittleEndian, uint32(len(coverData)))
	if err != nil {
		return nil, err
	}
	buf.Write(coverData)

	// 6. Audio Stream Data
	buf.Write([]byte("encrypted_audio_bytes_here"))

	return buf.Bytes(), nil
}

func TestSequentialNCMParser(t *testing.T) {
	mockData, err := generateMockNCMData()
	if err != nil {
		t.Fatalf("failed to generate mock NCM data: %v", err)
	}

	parser := &SequentialNCMParser{}
	parsed, err := parser.Parse(bytes.NewReader(mockData))
	if err != nil {
		t.Fatalf("failed to parse mock NCM data: %v", err)
	}

	if parsed.Metadata().Name != "Test Song" {
		t.Errorf("expected metadata musicName 'Test Song', got '%s'", parsed.Metadata().Name)
	}

	if parsed.AudioFormat() != "mp3" {
		t.Errorf("expected audioFormat 'mp3', got '%s'", parsed.AudioFormat())
	}

	if !bytes.Equal(parsed.Cover(), []byte("coverimage")) {
		t.Errorf("expected cover 'coverimage', got '%s'", string(parsed.Cover()))
	}
}
