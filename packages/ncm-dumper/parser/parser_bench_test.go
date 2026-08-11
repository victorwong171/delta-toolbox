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

func BenchmarkSequentialNCMParser(b *testing.B) {
	mockNCM, err := generateMockNCM()
	if err != nil {
		b.Fatalf("failed to generate mock NCM: %v", err)
	}

	p := &SequentialNCMParser{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.Parse(bytes.NewReader(mockNCM))
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}
