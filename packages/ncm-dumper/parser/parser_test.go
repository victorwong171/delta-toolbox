package parser

import (
	"bytes"
	"io"
	"testing"
)

func TestDecryptReader_Correctness(t *testing.T) {
	// Create lookup table
	var xorLookup [256]byte
	for i := range xorLookup {
		xorLookup[i] = byte(i*3 + 7)
	}

	// Test inputs of various sizes to verify both unrolled loop and remainder loop work flawlessly
	sizes := []int{0, 1, 3, 7, 8, 9, 15, 16, 17, 100, 1024, 1027}
	for _, size := range sizes {
		// Prepare original data
		originalData := make([]byte, size)
		for i := range originalData {
			originalData[i] = byte(i)
		}

		// Calculate expected data using a simple, known-correct reference implementation
		expectedData := make([]byte, size)
		copy(expectedData, originalData)
		offset := byte(0)
		for i := range expectedData {
			offset++
			expectedData[i] ^= xorLookup[offset]
		}

		// Run DecryptReader
		r := bytes.NewReader(originalData)
		dr := &DecryptReader{
			r:         r,
			xorLookup: &xorLookup,
		}

		// Read with varying buffer sizes (e.g. 1, 5, 8, 10, size)
		bufSizes := []int{1, 5, 8, 13, size}
		for _, bufSize := range bufSizes {
			if bufSize <= 0 {
				continue
			}
			r.Seek(0, io.SeekStart)
			dr.streamOffset = 0

			result := make([]byte, 0, size)
			buf := make([]byte, bufSize)
			for {
				n, err := dr.Read(buf)
				if n > 0 {
					result = append(result, buf[:n]...)
				}
				if err != nil {
					break
				}
			}

			if !bytes.Equal(result, expectedData) {
				t.Errorf("size %d, bufSize %d: Decrypted data mismatch.\nExpected: %v\nGot:      %v", size, bufSize, expectedData[:min(10, len(expectedData))], result[:min(10, len(result))])
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
