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

func BenchmarkParse(b *testing.B) {
	rc4Key := []byte("verysecret_rc4_key")
	metaJSON := `{"musicId":99999,"musicName":"HyperDrive","format":"flac"}`
	coverData := []byte("somefakejpegimagedata")
	rawAudio := make([]byte, 1024) // 1KB audio
	mockNCMBytes := generateMockNCM(rc4Key, metaJSON, coverData, rawAudio)

	parser := &SequentialNCMParser{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(mockNCMBytes)
		_, err := parser.Parse(r)
		if err != nil {
			b.Fatalf("failed to parse NCM in benchmark: %v", err)
		}
	}
}
