package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	lf := New()

	if lf.LockfileVersion != LockfileVersion {
		t.Errorf("LockfileVersion = %d, want %d", lf.LockfileVersion, LockfileVersion)
	}
	if lf.Packages == nil {
		t.Error("Packages should not be nil")
	}
	if len(lf.Packages) != 0 {
		t.Errorf("Packages should be empty, got %d", len(lf.Packages))
	}
}

func TestLoadNonExistent(t *testing.T) {
	dir := t.TempDir()

	lf, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if lf.LockfileVersion != LockfileVersion {
		t.Errorf("LockfileVersion = %d, want %d", lf.LockfileVersion, LockfileVersion)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	// Create and save
	lf := New()
	lf.SetPackage("user/fontname", &PackageEntry{
		Version: "1.0.0",
		Status:  StatusActive,
		Files: []FileEntry{
			{
				Filename:    "Font-Regular.otf",
				SHA256:      "abc123",
				SymlinkName: "user--fontname--Font-Regular.otf",
			},
		},
	})

	if err := lf.Save(dir); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, LockfileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Lockfile was not created")
	}

	// Load and verify
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	pkg := loaded.GetPackage("user/fontname")
	if pkg == nil {
		t.Fatal("Package not found after load")
	}
	if pkg.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", pkg.Version, "1.0.0")
	}
	if pkg.Status != StatusActive {
		t.Errorf("Status = %q, want %q", pkg.Status, StatusActive)
	}
	if len(pkg.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(pkg.Files))
	}
	if pkg.Files[0].Filename != "Font-Regular.otf" {
		t.Errorf("Filename = %q, want %q", pkg.Files[0].Filename, "Font-Regular.otf")
	}
}

func TestSetStatus(t *testing.T) {
	lf := New()
	lf.SetPackage("user/font", &PackageEntry{
		Version: "1.0",
		Status:  StatusInactive,
	})

	if err := lf.SetStatus("user/font", StatusActive); err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}

	pkg := lf.GetPackage("user/font")
	if pkg.Status != StatusActive {
		t.Errorf("Status = %q, want %q", pkg.Status, StatusActive)
	}
}

func TestSetStatusNotFound(t *testing.T) {
	lf := New()

	err := lf.SetStatus("nonexistent", StatusActive)
	if err == nil {
		t.Error("SetStatus() should return error for nonexistent package")
	}
}

func TestRemovePackage(t *testing.T) {
	lf := New()
	lf.SetPackage("user/font", &PackageEntry{Version: "1.0"})

	lf.RemovePackage("user/font")

	if pkg := lf.GetPackage("user/font"); pkg != nil {
		t.Error("Package should be removed")
	}
}

func TestListPackages(t *testing.T) {
	lf := New()
	lf.SetPackage("user/font1", &PackageEntry{Version: "1.0", Status: StatusActive})
	lf.SetPackage("user/font2", &PackageEntry{Version: "2.0", Status: StatusInactive})

	names := lf.ListPackages()
	if len(names) != 2 {
		t.Errorf("ListPackages() count = %d, want 2", len(names))
	}
}

func TestListActivePackages(t *testing.T) {
	lf := New()
	lf.SetPackage("user/font1", &PackageEntry{Version: "1.0", Status: StatusActive})
	lf.SetPackage("user/font2", &PackageEntry{Version: "2.0", Status: StatusInactive})
	lf.SetPackage("user/font3", &PackageEntry{Version: "3.0", Status: StatusActive})

	active := lf.ListActivePackages()
	if len(active) != 2 {
		t.Errorf("ListActivePackages() count = %d, want 2", len(active))
	}
}
