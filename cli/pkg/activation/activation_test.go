package activation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSymlinkName(t *testing.T) {
	got := BuildSymlinkName("alice", "myfont", "Regular.otf")
	want := "alice--myfont--Regular.otf"
	if got != want {
		t.Errorf("BuildSymlinkName() = %q, want %q", got, want)
	}
}

func TestParseSymlinkName(t *testing.T) {
	username, fontname, filename, err := ParseSymlinkName("alice--myfont--Regular.otf")
	if err != nil {
		t.Fatalf("ParseSymlinkName() error = %v", err)
	}
	if username != "alice" {
		t.Errorf("username = %q, want %q", username, "alice")
	}
	if fontname != "myfont" {
		t.Errorf("fontname = %q, want %q", fontname, "myfont")
	}
	if filename != "Regular.otf" {
		t.Errorf("filename = %q, want %q", filename, "Regular.otf")
	}
}

func TestParseSymlinkNameInvalid(t *testing.T) {
	_, _, _, err := ParseSymlinkName("invalid-name")
	if err == nil {
		t.Error("ParseSymlinkName() should return error for invalid format")
	}
}

func TestCreateSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	activationDir := filepath.Join(tmpDir, "activation")
	targetDir := filepath.Join(tmpDir, "target")

	// Create target file
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(targetDir, "Font.otf")
	if err := os.WriteFile(targetPath, []byte("font data"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(activationDir)
	symlinkName := BuildSymlinkName("alice", "myfont", "Font.otf")

	if err := m.CreateSymlink(targetPath, symlinkName); err != nil {
		t.Fatalf("CreateSymlink() error = %v", err)
	}

	// Verify symlink exists and points to correct target
	symlinkPath := filepath.Join(activationDir, symlinkName)
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		t.Fatalf("Symlink not created: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Created file is not a symlink")
	}

	// Verify symlink target
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != targetPath {
		t.Errorf("Symlink target = %q, want %q", target, targetPath)
	}
}

func TestCreateSymlinkOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	activationDir := filepath.Join(tmpDir, "activation")
	targetDir := filepath.Join(tmpDir, "target")

	// Create two target files
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	target1 := filepath.Join(targetDir, "v1", "Font.otf")
	target2 := filepath.Join(targetDir, "v2", "Font.otf")
	for _, target := range []string{target1, target2} {
		dir := filepath.Dir(target)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("font"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := NewManager(activationDir)
	symlinkName := "alice--myfont--Font.otf"

	// Create first symlink
	if err := m.CreateSymlink(target1, symlinkName); err != nil {
		t.Fatalf("First CreateSymlink() error = %v", err)
	}

	// Overwrite with second symlink
	if err := m.CreateSymlink(target2, symlinkName); err != nil {
		t.Fatalf("Second CreateSymlink() error = %v", err)
	}

	// Verify symlink points to new target
	symlinkPath := filepath.Join(activationDir, symlinkName)
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != target2 {
		t.Errorf("Symlink target = %q, want %q", target, target2)
	}
}

func TestRemoveSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	activationDir := filepath.Join(tmpDir, "activation")

	if err := os.MkdirAll(activationDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a symlink
	symlinkPath := filepath.Join(activationDir, "test-symlink")
	targetPath := filepath.Join(tmpDir, "target")
	if err := os.WriteFile(targetPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	m := NewManager(activationDir)
	if err := m.RemoveSymlink("test-symlink"); err != nil {
		t.Fatalf("RemoveSymlink() error = %v", err)
	}

	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Error("Symlink should be removed")
	}
}

func TestRemoveSymlinkNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Should not error for non-existent symlink
	err := m.RemoveSymlink("nonexistent")
	if err != nil {
		t.Errorf("RemoveSymlink() error = %v, want nil", err)
	}
}

func TestRemovePackageSymlinks(t *testing.T) {
	tmpDir := t.TempDir()
	activationDir := filepath.Join(tmpDir, "activation")
	targetPath := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(activationDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlinks for two packages
	symlinks := []string{
		"alice--font1--Regular.otf",
		"alice--font1--Bold.otf",
		"alice--font2--Regular.otf",
	}
	for _, name := range symlinks {
		path := filepath.Join(activationDir, name)
		if err := os.Symlink(targetPath, path); err != nil {
			t.Fatal(err)
		}
	}

	m := NewManager(activationDir)
	if err := m.RemovePackageSymlinks("alice", "font1"); err != nil {
		t.Fatalf("RemovePackageSymlinks() error = %v", err)
	}

	// Verify font1 symlinks are removed
	remaining, _ := m.ListPackageSymlinks("alice", "font1")
	if len(remaining) != 0 {
		t.Errorf("font1 symlinks remaining: %v", remaining)
	}

	// Verify font2 symlinks remain
	remaining, _ = m.ListPackageSymlinks("alice", "font2")
	if len(remaining) != 1 {
		t.Errorf("font2 symlinks count = %d, want 1", len(remaining))
	}
}

func TestIsActive(t *testing.T) {
	tmpDir := t.TempDir()
	activationDir := filepath.Join(tmpDir, "activation")
	targetPath := filepath.Join(tmpDir, "target")

	if err := os.MkdirAll(activationDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(activationDir)

	// Should not be active initially
	active, err := m.IsActive("alice", "font1")
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}
	if active {
		t.Error("Should not be active initially")
	}

	// Create symlink and check again
	symlinkPath := filepath.Join(activationDir, "alice--font1--Regular.otf")
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	active, err = m.IsActive("alice", "font1")
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}
	if !active {
		t.Error("Should be active after creating symlink")
	}
}
