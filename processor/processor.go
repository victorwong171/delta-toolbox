package processor

// FileProcessor defines the unified interface for all post-processing plugins.
type FileProcessor interface {
	// Process executes the post-processing logic on the input source file (src)
	// and the generated destination file (dst).
	Process(src, dst string) error
}
