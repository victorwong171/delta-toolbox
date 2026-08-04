package compression

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"sync"

	"lfs/internal/interfaces"
)

// pooledReader groups *gzip.Reader and *bytes.Reader to avoid heap allocations
// of bytes.Reader during decompression.
type pooledReader struct {
	gr *gzip.Reader
	br *bytes.Reader
}

// GzipCompressor implements interfaces.Compressor using gzip compression.
// It uses sync.Pool to reuse gzip.Writer, gzip.Reader, and bytes.Buffer,
// drastically reducing memory allocation and garbage collection overhead.
type GzipCompressor struct {
	writerPool sync.Pool
	readerPool sync.Pool
	bufferPool sync.Pool
}

// NewGzipCompressor returns a new gzip compressor instance.
func NewGzipCompressor() interfaces.Compressor {
	return &GzipCompressor{
		writerPool: sync.Pool{
			New: func() interface{} {
				// We pass io.Discard initially so we don't have to provide a nil/dummy writer or cause panic.
				return gzip.NewWriter(io.Discard)
			},
		},
		readerPool: sync.Pool{
			New: func() interface{} {
				// We pool a bundled *gzip.Reader and *bytes.Reader to avoid
				// allocating a new bytes.Reader on every Decompress call.
				return &pooledReader{
					gr: &gzip.Reader{},
					br: &bytes.Reader{},
				}
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
	}
}

// Compress compresses byte data using pooled resources.
func (g *GzipCompressor) Compress(data []byte) ([]byte, error) {
	buf := g.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer g.bufferPool.Put(buf)

	w := g.writerPool.Get().(*gzip.Writer)
	w.Reset(buf)
	defer func() {
		w.Reset(io.Discard) // Reset to io.Discard to prevent keeping reference to the buffer
		g.writerPool.Put(w)
	}()

	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	// Copy the data out from the pooled buffer to ensure no slice sharing issues
	// once the buffer is returned to the pool and reused.
	compressed := make([]byte, buf.Len())
	copy(compressed, buf.Bytes())
	return compressed, nil
}

// CompressStream creates a streaming compression writer.
func (g *GzipCompressor) CompressStream(w io.Writer) (io.WriteCloser, error) {
	return gzip.NewWriter(w), nil
}

// Decompress decompresses byte data using pooled resources.
func (g *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	pr := g.readerPool.Get().(*pooledReader)
	defer func() {
		pr.br.Reset(nil)
		_ = pr.gr.Reset(pr.br) // Reset to release the reference to the underlying bytes
		g.readerPool.Put(pr)
	}()

	pr.br.Reset(data)
	err := pr.gr.Reset(pr.br)
	if err != nil {
		return nil, err
	}
	defer pr.gr.Close()
	return io.ReadAll(pr.gr)
}

// DecompressStream creates a streaming decompression reader.
func (g *GzipCompressor) DecompressStream(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}

// ContentEncoding returns the HTTP Content-Encoding header value.
func (g *GzipCompressor) ContentEncoding() string {
	return "gzip"
}

// Supports checks if the specified encoding format is supported.
func (g *GzipCompressor) Supports(encoding string) bool {
	return strings.Contains(encoding, "gzip")
}
