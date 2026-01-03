// Package lockfile manages the fontpub.lock file.
package lockfile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	LockfileVersion = 1
	LockfileName    = "fontpub.lock"
)

// Status represents the activation status of a package.
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// FileEntry represents a single font file within a package.
type FileEntry struct {
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	SymlinkName string `json:"symlink_name"`
}

// PackageEntry represents an installed package in the lockfile.
type PackageEntry struct {
	Version string      `json:"version"`
	Status  Status      `json:"status"`
	Files   []FileEntry `json:"files"`
}

// Lockfile represents the complete fontpub.lock structure.
type Lockfile struct {
	LockfileVersion int                      `json:"lockfile_version"`
	UpdatedAt       string                   `json:"updated_at"`
	Packages        map[string]*PackageEntry `json:"packages"`
}

// New creates a new empty lockfile.
func New() *Lockfile {
	return &Lockfile{
		LockfileVersion: LockfileVersion,
		UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		Packages:        make(map[string]*PackageEntry),
	}
}

// Load reads a lockfile from the given directory.
// Returns a new empty lockfile if the file doesn't exist.
func Load(dir string) (*Lockfile, error) {
	path := filepath.Join(dir, LockfileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(), nil
		}
		return nil, err
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}

	// Initialize packages map if nil
	if lf.Packages == nil {
		lf.Packages = make(map[string]*PackageEntry)
	}

	return &lf, nil
}

// Save writes the lockfile to the given directory.
func (lf *Lockfile) Save(dir string) error {
	lf.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, LockfileName)
	return os.WriteFile(path, data, 0644)
}

// GetPackage returns a package entry by name, or nil if not found.
func (lf *Lockfile) GetPackage(name string) *PackageEntry {
	return lf.Packages[name]
}

// SetPackage adds or updates a package entry.
func (lf *Lockfile) SetPackage(name string, entry *PackageEntry) {
	lf.Packages[name] = entry
}

// RemovePackage removes a package entry.
func (lf *Lockfile) RemovePackage(name string) {
	delete(lf.Packages, name)
}

// SetStatus updates the status of a package.
func (lf *Lockfile) SetStatus(name string, status Status) error {
	pkg := lf.Packages[name]
	if pkg == nil {
		return errors.New("package not found: " + name)
	}
	pkg.Status = status
	return nil
}

// ListPackages returns all package names.
func (lf *Lockfile) ListPackages() []string {
	names := make([]string, 0, len(lf.Packages))
	for name := range lf.Packages {
		names = append(names, name)
	}
	return names
}

// ListActivePackages returns names of all active packages.
func (lf *Lockfile) ListActivePackages() []string {
	var names []string
	for name, pkg := range lf.Packages {
		if pkg.Status == StatusActive {
			names = append(names, name)
		}
	}
	return names
}
