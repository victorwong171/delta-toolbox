package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCalculateFileMD5(b *testing.B) {
	// Create a temporary 10MB file for testing
	tmpDir, err := os.MkdirTemp("", "md5-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "10mb.bin")
	data := make([]byte, 10*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateFileMD5WithProgress(b *testing.B) {
	// Create a temporary 10MB file for testing
	tmpDir, err := os.MkdirTemp("", "md5-bench-progress")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "10mb.bin")
	data := make([]byte, 10*1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5WithProgress(tmpFile, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
