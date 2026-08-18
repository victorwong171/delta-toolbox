package storage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateFileMD5(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "testfile.dat")

	// Generate 10MB dummy file
	content := bytes.Repeat([]byte("1234567890ABCDEF"), 1024*640)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	expectedHasher := md5.New()
	expectedHasher.Write(content)
	expectedMD5 := hex.EncodeToString(expectedHasher.Sum(nil))

	// Test calculateFileMD5
	gotMD5, err := calculateFileMD5(filePath)
	if err != nil {
		t.Fatalf("calculateFileMD5 failed: %v", err)
	}
	if gotMD5 != expectedMD5 {
		t.Errorf("calculateFileMD5 expected %s, got %s", expectedMD5, gotMD5)
	}

	// Test calculateFileMD5WithProgress
	var progressCalled bool
	gotProgressMD5, err := calculateFileMD5WithProgress(filePath, func(p float64) {
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("calculateFileMD5WithProgress failed: %v", err)
	}
	if gotProgressMD5 != expectedMD5 {
		t.Errorf("calculateFileMD5WithProgress expected %s, got %s", expectedMD5, gotProgressMD5)
	}
	if !progressCalled {
		t.Errorf("Expected progressCallback to be called")
	}
}

func TestCopyWithCancel(t *testing.T) {
	content := bytes.Repeat([]byte("Hello, World! "), 10000)
	src := bytes.NewReader(content)
	dst := &bytes.Buffer{}

	ctx := context.Background()
	err := copyWithCancel(ctx, dst, src, int64(len(content)))
	if err != nil {
		t.Fatalf("copyWithCancel failed: %v", err)
	}

	if !bytes.Equal(dst.Bytes(), content) {
		t.Errorf("copyWithCancel copied content mismatch")
	}
}

func BenchmarkCalculateFileMD5(b *testing.B) {
	tempDir := b.TempDir()
	filePath := filepath.Join(tempDir, "benchfile.dat")

	// Generate 8MB dummy file
	content := bytes.Repeat([]byte("0123456789ABCDEF"), 1024*512)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		b.Fatalf("Failed to create bench file: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := calculateFileMD5(filePath)
		if err != nil {
			b.Fatalf("calculateFileMD5 failed: %v", err)
		}
	}
}

func BenchmarkCopyWithCancel(b *testing.B) {
	content := bytes.Repeat([]byte("0123456789ABCDEF"), 1024*512) // 8MB
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(content)
		dst := ioDiscardWriter{}
		err := copyWithCancel(ctx, dst, src, int64(len(content)))
		if err != nil {
			b.Fatalf("copyWithCancel failed: %v", err)
		}
	}
}

type ioDiscardWriter struct{}

func (ioDiscardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
