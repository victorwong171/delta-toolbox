package parser

import (
	"bytes"
	"io"
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

func BenchmarkSequentialNCMParser(b *testing.B) {
	rc4Key := "test_key_12345"
	metaJSON := `{"musicId":999,"musicName":"Test Song","album":"Awesome Album","artist":[["Singer A",1001]],"format":"flac"}`
	cover := make([]byte, 50*1024)   // 50KB cover
	audio := make([]byte, 1024*1024) // 1MB audio
	for i := range audio {
		audio[i] = byte(i)
	}

	ncmBytes, err := generateMockNCMData(rc4Key, metaJSON, cover, audio)
	if err != nil {
		b.Fatalf("failed to generate mock NCM data: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := &SequentialNCMParser{}
		parsed, err := parser.Parse(bytes.NewReader(ncmBytes))
		if err != nil {
			b.Fatalf("failed to parse: %v", err)
		}
		// Also read some audio stream to include DecryptReader overhead
		stream := parsed.DecryptedStream()
		buf := make([]byte, 4096)
		for {
			_, err := stream.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatalf("failed to read stream: %v", err)
			}
		}
	}
}
