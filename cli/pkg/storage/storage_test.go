package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPathsWithHome(t *testing.T) {
	paths := NewPathsWithHome("/home/user")

	if paths.Root != "/home/user/.fontpub" {
		t.Errorf("Root = %q, want %q", paths.Root, "/home/user/.fontpub")
	}
	if paths.Packages != "/home/user/.fontpub/packages" {
		t.Errorf("Packages = %q, want %q", paths.Packages, "/home/user/.fontpub/packages")
	}
	if paths.ActivationDir != "/home/user/Library/Fonts/from_fontpub" {
		t.Errorf("ActivationDir = %q, want %q", paths.ActivationDir, "/home/user/Library/Fonts/from_fontpub")
	}
}

func TestPackagePath(t *testing.T) {
	paths := NewPathsWithHome("/home/user")

	got := paths.PackagePath("alice", "myfont", "1.0.0")
	want := "/home/user/.fontpub/packages/alice/myfont/1.0.0"
	if got != want {
		t.Errorf("PackagePath() = %q, want %q", got, want)
	}
}

func TestPackageFilePath(t *testing.T) {
	paths := NewPathsWithHome("/home/user")

	got := paths.PackageFilePath("alice", "myfont", "1.0.0", "Regular.otf")
	want := "/home/user/.fontpub/packages/alice/myfont/1.0.0/Regular.otf"
	if got != want {
		t.Errorf("PackageFilePath() = %q, want %q", got, want)
	}
}

func TestSymlinkPath(t *testing.T) {
	paths := NewPathsWithHome("/home/user")

	got := paths.SymlinkPath("alice", "myfont", "Regular.otf")
	want := "/home/user/Library/Fonts/from_fontpub/alice--myfont--Regular.otf"
	if got != want {
		t.Errorf("SymlinkPath() = %q, want %q", got, want)
	}
}

func TestEnsureDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewPathsWithHome(tmpDir)

	if err := paths.EnsureDirectories(); err != nil {
		t.Fatalf("EnsureDirectories() error = %v", err)
	}

	// Check directories exist
	if _, err := os.Stat(paths.Root); os.IsNotExist(err) {
		t.Error("Root directory was not created")
	}
	if _, err := os.Stat(paths.Packages); os.IsNotExist(err) {
		t.Error("Packages directory was not created")
	}
}

func TestEnsurePackageDir(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewPathsWithHome(tmpDir)

	if err := paths.EnsurePackageDir("alice", "myfont", "1.0.0"); err != nil {
		t.Fatalf("EnsurePackageDir() error = %v", err)
	}

	dir := paths.PackagePath("alice", "myfont", "1.0.0")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Package directory was not created")
	}
}

func TestPackageExists(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewPathsWithHome(tmpDir)

	// Should not exist initially
	if paths.PackageExists("alice", "myfont", "1.0.0") {
		t.Error("PackageExists() should return false for non-existent package")
	}

	// Create the directory
	dir := paths.PackagePath("alice", "myfont", "1.0.0")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Should exist now
	if !paths.PackageExists("alice", "myfont", "1.0.0") {
		t.Error("PackageExists() should return true for existing package")
	}
}

func TestRemovePackage(t *testing.T) {
	tmpDir := t.TempDir()
	paths := NewPathsWithHome(tmpDir)

	// Create package directories with multiple versions
	for _, version := range []string{"1.0.0", "1.1.0"} {
		dir := paths.PackagePath("alice", "myfont", version)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		// Create a dummy file
		file := filepath.Join(dir, "Font.otf")
		if err := os.WriteFile(file, []byte("dummy"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Remove package
	if err := paths.RemovePackage("alice", "myfont"); err != nil {
		t.Fatalf("RemovePackage() error = %v", err)
	}

	// Check all versions are removed
	for _, version := range []string{"1.0.0", "1.1.0"} {
		if paths.PackageExists("alice", "myfont", version) {
			t.Errorf("Package version %s should be removed", version)
		}
	}
}
