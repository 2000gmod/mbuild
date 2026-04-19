package iutils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// extractZipFS is the core function that uses fs.WalkDir on a zip.Reader (which implements fs.FS).
func ExtractZipFS(zfs fs.FS, destDir string) error {
	// Clean and absolutize the destination directory for security checks
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of destDir: %w", err)
	}

	// Create the root destination directory if it doesn't exist
	if err := os.MkdirAll(absDest, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Walk the entire zip file system
	err = fs.WalkDir(zfs, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Build the full output path
		outPath := filepath.Join(absDest, path)

		// Security: prevent Zip Slip (path traversal)
		// Ensure the resolved path is still inside absDest
		resolved, err := filepath.Abs(outPath)
		if err != nil {
			return fmt.Errorf("failed to resolve path %s: %w", outPath, err)
		}
		if !strings.HasPrefix(resolved, absDest+string(os.PathSeparator)) && resolved != absDest {
			return fmt.Errorf("invalid zip entry: %s would escape destination", path)
		}

		if d.IsDir() {
			// Create directory (including any missing parents)
			if err := os.MkdirAll(outPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", outPath, err)
			}
			return nil
		}

		// It's a regular file: we need to get its content.
		// Since we have an fs.FS, we can open the file using zfs.Open.
		file, err := zfs.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open zip entry %s: %w", path, err)
		}
		defer file.Close()

		// Ensure the parent directory exists
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent dir for %s: %w", outPath, err)
		}

		// Create the output file with the same permissions as stored in the zip.
		// To get the original file mode, we need the fs.FileInfo from the DirEntry.
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", path, err)
		}
		mode := info.Mode()

		outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", outPath, err)
		}
		defer outFile.Close()

		// Copy the content
		if _, err := io.Copy(outFile, file); err != nil {
			return fmt.Errorf("failed to copy content to %s: %w", outPath, err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking zip: %w", err)
	}
	return nil
}
