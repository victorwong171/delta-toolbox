package compression

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func TestGzipReaderReset(t *testing.T) {
	// Create a valid gzip stream
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte("hello world"))
	w.Close()

	// Use an empty gzip.Reader
	var r gzip.Reader
	err := r.Reset(&buf)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	decompressed, err := io.ReadAll(&r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(decompressed) != "hello world" {
		t.Fatalf("Expected 'hello world', got '%s'", decompressed)
	}
}

func BenchmarkCompress(b *testing.B) {
	compressor := NewGzipCompressor()
	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compressor.Compress(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecompress(b *testing.B) {
	compressor := NewGzipCompressor()
	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.")
	compressed, err := compressor.Compress(data)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := compressor.Decompress(compressed)
		if err != nil {
			b.Fatal(err)
		}
	}
}
