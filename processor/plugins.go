package processor

import (
	"fmt"
	"os"
)

// DeleteSourceProcessor deletes the original NCM file after successful conversion.
type DeleteSourceProcessor struct{}

// Process verifies the target file is intact and then removes the original source file.
func (p *DeleteSourceProcessor) Process(src, dst string) error {
	// First check the final destination file
	info, err := os.Stat(dst)
	if err != nil {
		// If dst does not exist, check if tempDst (.tmp) exists (in case rename hasn't happened yet)
		info, err = os.Stat(dst + ".tmp")
		if err != nil {
			return fmt.Errorf("target file not found: %w", err)
		}
	}

	// Verify that the output file is not empty
	if info.Size() == 0 {
		return fmt.Errorf("target file is empty, skipping source deletion")
	}

	// Safely remove the original .ncm source file
	if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove source file %q: %w", src, err)
	}

	return nil
}

// SizeComparisonProcessor compares the size of the generated file with any pre-existing
// file of the same name and only keeps the larger one.
type SizeComparisonProcessor struct{}

// Process compares the tempDst (.tmp) size with the existing dst and retains the larger one.
func (p *SizeComparisonProcessor) Process(src, dst string) error {
	tempDst := dst + ".tmp"

	// Stat the temporary decrypted file
	tempInfo, err := os.Stat(tempDst)
	if err != nil {
		if os.IsNotExist(err) {
			// Temp file doesn't exist, likely already handled/renamed by another processor
			return nil
		}
		return fmt.Errorf("failed to stat temporary file: %w", err)
	}

	// Stat the pre-existing destination file
	destInfo, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			// Destination doesn't exist yet, simply rename temp file to final destination
			if err := os.Rename(tempDst, dst); err != nil {
				return fmt.Errorf("failed to rename temporary file to destination: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to stat destination file: %w", err)
	}

	// Both exist, compare sizes and retain the larger one
	if tempInfo.Size() > destInfo.Size() {
		// Target is larger, overwrite existing file
		// On Windows, we must remove the target first to prevent "file already exists" rename error
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("failed to remove smaller destination file: %w", err)
		}
		if err := os.Rename(tempDst, dst); err != nil {
			return fmt.Errorf("failed to replace destination with larger temporary file: %w", err)
		}
	} else {
		// Existing file is larger or equal, discard the temporary decrypted file
		if err := os.Remove(tempDst); err != nil {
			return fmt.Errorf("failed to remove smaller temporary file: %w", err)
		}
	}

	return nil
}
