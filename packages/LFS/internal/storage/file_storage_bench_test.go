package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCalculateFileMD5(b *testing.B) {
	// Create a temp file of 10MB
	dir := b.TempDir()
	filePath := filepath.Join(dir, "bench_test_file")
	data := make([]byte, 10*1024*1024) // 10MB
	// Fill with some data
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		b.Fatalf("failed to write temp file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5(filePath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCalculateFileMD5WithProgress(b *testing.B) {
	// Create a temp file of 10MB
	dir := b.TempDir()
	filePath := filepath.Join(dir, "bench_test_file")
	data := make([]byte, 10*1024*1024) // 10MB
	// Fill with some data
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		b.Fatalf("failed to write temp file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5WithProgress(filePath, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
