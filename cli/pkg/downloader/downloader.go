// Package downloader handles file downloads with SHA256 verification.
package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Downloader handles file downloads with hash verification.
type Downloader struct {
	HTTPClient *http.Client
}

// New creates a new Downloader with default settings.
func New() *Downloader {
	return &Downloader{
		HTTPClient: http.DefaultClient,
	}
}

// Download fetches a file from the given URL and saves it to destPath.
// It verifies the SHA256 hash if expectedHash is provided.
func (d *Downloader) Download(url, destPath, expectedHash string) error {
	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download to temporary file
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath) // Clean up on any error

	resp, err := d.HTTPClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Calculate hash while downloading
	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write file: %w", err)
	}
	tmpFile.Close()

	// Verify hash if provided
	if expectedHash != "" {
		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if actualHash != expectedHash {
			return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
		}
	}

	// Move temp file to final destination (atomic on same filesystem)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	return nil
}

// CalculateHash computes the SHA256 hash of a file.
func CalculateHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
