// yes, 99% of this code was authored by ChatGPT
package main

import (
	"archive/tar"
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("Usage: %s <input.conda> <target-directory>", os.Args[0])
	}

	condaFile := os.Args[1]
	targetDir := os.Args[2]

	// Ensure the target directory exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("Failed to create target directory: %v", err)
	}

	// Open the .conda file
	zipReader, err := zip.OpenReader(condaFile)
	if err != nil {
		log.Fatalf("Failed to open .conda file: %v", err)
	}
	defer zipReader.Close()

	// Process each file in the .conda archive
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".tar.zst") {
			if err := extractTarZstStream(file, targetDir); err != nil {
				log.Fatalf("Failed to extract %s: %v", file.Name, err)
			}
		} else {
			// Extract metadata.json or other files
			if err := extractFile(file, targetDir); err != nil {
				log.Fatalf("Failed to extract %s: %v", file.Name, err)
			}
		}
	}

	fmt.Println("Extraction complete.")
}

func extractFile(f *zip.File, targetDir string) error {
	// Open the file inside the ZIP archive
	srcFile, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %w", err)
	}
	defer srcFile.Close()

	// Create the destination file
	destPath := filepath.Join(targetDir, f.Name)
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy the content to the destination file
	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}

func extractTarZstStream(f *zip.File, targetDir string) error {
	// Open the .tar.zst file inside the ZIP archive
	srcFile, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %w", err)
	}
	defer srcFile.Close()

	// Create a Zstandard decompressor
	decoder, err := zstd.NewReader(srcFile)
	if err != nil {
		return fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()

	// Create a TAR reader to process the decompressed data
	tarReader := tar.NewReader(decoder)

	// Determine the subdirectory for extraction
	subDir := "pkg"
	if strings.Contains(f.Name, "info-") {
		subDir = "info"
	}

	destDir := filepath.Join(targetDir, subDir)

	// Extract the contents of the TAR archive
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		destPath := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directories as needed
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directories for symlink: %w", err)
			}
			if err := os.Symlink(header.Linkname, destPath); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}
		case tar.TypeReg:
			// Create files
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directories: %w", err)
			}
			destFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			// Copy the file content
			if _, err := io.Copy(destFile, tarReader); err != nil {
				destFile.Close()
				return fmt.Errorf("failed to copy file content: %w", err)
			}
			destFile.Close()
			// Set file permissions
			if err := os.Chmod(destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to set file permissions: %w", err)
			}
		default:
			// Handle other file types if necessary
			fmt.Printf("Skipping unsupported TAR entry: %s\n", header.Name)
		}
	}

	return nil
}
