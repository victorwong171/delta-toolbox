package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// Benchmark results:
// Before optimization:
// BenchmarkCalculateFileMD5-4   	      61	  19435680 ns/op	 4194711 B/op	       8 allocs/op
//
// After optimization:
// BenchmarkCalculateFileMD5-4   	      64	  18776929 ns/op	   65932 B/op	       6 allocs/op
//
// Impact:
// - Memory allocation reduced by 98.4% (from ~4.19MB down to ~65.9KB).
// - Garbage collection (GC) pressure and overhead significantly reduced during file uploads and listings.

func BenchmarkCalculateFileMD5(b *testing.B) {
	// Create a 10MB temporary file
	tmpDir, err := os.MkdirTemp("", "md5-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.dat")
	data := make([]byte, 1024*1024) // 1MB chunk of repeating data
	for i := range data {
		data[i] = byte(i % 256)
	}

	f, err := os.Create(tmpFile)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := f.Write(data); err != nil {
			f.Close()
			b.Fatal(err)
		}
	}
	f.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5(tmpFile)
		if err != nil {
			b.Fatal(err)
		}
	}
}
