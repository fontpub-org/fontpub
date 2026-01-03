package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownload(t *testing.T) {
	content := []byte("test file content")
	hash := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.txt")

	d := New()
	err := d.Download(server.URL, destPath, expectedHash)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	// Verify file exists and content matches
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Content = %q, want %q", string(got), string(content))
	}
}

func TestDownloadHashMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("some content"))
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.txt")

	d := New()
	err := d.Download(server.URL, destPath, "wronghash")
	if err == nil {
		t.Error("Download() should return error for hash mismatch")
	}

	// File should not exist after hash mismatch
	if _, err := os.Stat(destPath); !os.IsNotExist(err) {
		t.Error("File should not exist after hash mismatch")
	}
}

func TestDownloadNoHashVerification(t *testing.T) {
	content := []byte("test content without hash check")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.txt")

	d := New()
	// Empty hash means no verification
	err := d.Download(server.URL, destPath, "")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Content = %q, want %q", string(got), string(content))
	}
}

func TestDownloadHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "downloaded.txt")

	d := New()
	err := d.Download(server.URL, destPath, "")
	if err == nil {
		t.Error("Download() should return error for HTTP 404")
	}
}

func TestDownloadCreatesDirectory(t *testing.T) {
	content := []byte("test")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "nested", "dir", "file.txt")

	d := New()
	err := d.Download(server.URL, destPath, "")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("File should exist in nested directory")
	}
}

func TestCalculateHash(t *testing.T) {
	content := []byte("hello world")
	expectedHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	hash, err := CalculateHash(filePath)
	if err != nil {
		t.Fatalf("CalculateHash() error = %v", err)
	}

	if hash != expectedHash {
		t.Errorf("CalculateHash() = %q, want %q", hash, expectedHash)
	}
}
