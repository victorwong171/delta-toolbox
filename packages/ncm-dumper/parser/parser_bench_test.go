package parser

import (
	"bytes"
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

func BenchmarkBuildKeyBox(b *testing.B) {
	key := []byte("test_key_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildKeyBox(key)
	}
}

func BenchmarkReadLenAndData(b *testing.B) {
	payload := []byte{0x04, 0x00, 0x00, 0x00, 't', 'e', 's', 't'}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(payload)
		_, _ = readLenAndData(r)
	}
}

func BenchmarkDecryptAes128Ecb(b *testing.B) {
	key := aesCoreKey
	data := make([]byte, 256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := make([]byte, len(data))
		copy(input, data)
		_, _ = decryptAes128Ecb(key, input)
	}
}
