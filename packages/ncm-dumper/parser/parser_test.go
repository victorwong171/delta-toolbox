package parser

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestDecryptReader_Correctness(t *testing.T) {
	// Generate random data of various sizes to test edge cases
	// (e.g. less than 8, exact multiple of 8, and not multiple of 8)
	sizes := []int{0, 1, 7, 8, 9, 15, 16, 17, 1023, 1024, 1025, 4096, 5000}

	var xorLookup [256]byte
	for i := range xorLookup {
		xorLookup[i] = byte(rand.Intn(256))
	}

	for _, size := range sizes {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(rand.Intn(256))
		}

		// Calculate expected decrypted data with a reference implementation
		expected := make([]byte, size)
		copy(expected, data)
		offset := byte(0)
		for i := range expected {
			offset++
			expected[i] ^= xorLookup[offset]
		}

		// Calculate actual decrypted data using the unrolled DecryptReader
		r := bytes.NewReader(data)
		dr := &DecryptReader{
			r:         r,
			xorLookup: &xorLookup,
		}

		// Read in small chunks to test stream state and buffer boundary handling
		chunkSizes := []int{1, 3, 8, 16, 128}
		for _, chunkSize := range chunkSizes {
			// Reset reader and DecryptReader for this chunkSize run
			_, _ = r.Seek(0, 0)
			dr.streamOffset = 0

			buf := make([]byte, size)
			nRead := 0
			for nRead < size {
				limit := chunkSize
				if nRead+limit > size {
					limit = size - nRead
				}
				n, err := dr.Read(buf[nRead : nRead+limit])
				if n > 0 {
					nRead += n
				}
				if err != nil {
					break
				}
			}

			if !bytes.Equal(buf[:nRead], expected) {
				t.Errorf("Decrypted stream mismatch for size %d using chunk size %d", size, chunkSize)
			}
		}
	}
}
