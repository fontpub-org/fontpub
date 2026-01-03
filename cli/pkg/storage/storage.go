// Package storage manages the ~/.fontpub/ directory structure.
package storage

import (
	"os"
	"path/filepath"
)

const (
	// RootDirName is the name of the fontpub storage directory.
	RootDirName = ".fontpub"
	// PackagesDirName is the subdirectory for package storage.
	PackagesDirName = "packages"
)

// Paths holds all the paths used by fontpub.
type Paths struct {
	// Root is ~/.fontpub/
	Root string
	// Packages is ~/.fontpub/packages/
	Packages string
	// ActivationDir is ~/Library/Fonts/from_fontpub/
	ActivationDir string
}

// NewPaths creates a Paths instance using the user's home directory.
func NewPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return NewPathsWithHome(home), nil
}

// NewPathsWithHome creates a Paths instance with a custom home directory.
// Useful for testing.
func NewPathsWithHome(home string) *Paths {
	root := filepath.Join(home, RootDirName)
	return &Paths{
		Root:          root,
		Packages:      filepath.Join(root, PackagesDirName),
		ActivationDir: filepath.Join(home, "Library", "Fonts", "from_fontpub"),
	}
}

// EnsureDirectories creates all necessary directories if they don't exist.
func (p *Paths) EnsureDirectories() error {
	dirs := []string{p.Root, p.Packages}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureActivationDir creates the activation directory if it doesn't exist.
func (p *Paths) EnsureActivationDir() error {
	return os.MkdirAll(p.ActivationDir, 0755)
}

// PackagePath returns the path for a specific package version.
// Example: ~/.fontpub/packages/username/fontname/1.0.0/
func (p *Paths) PackagePath(username, fontname, version string) string {
	return filepath.Join(p.Packages, username, fontname, version)
}

// PackageFilePath returns the full path to a font file within a package.
func (p *Paths) PackageFilePath(username, fontname, version, filename string) string {
	return filepath.Join(p.PackagePath(username, fontname, version), filename)
}

// SymlinkPath returns the path for a symlink in the activation directory.
// Format: ~/Library/Fonts/from_fontpub/{username}--{fontname}--{filename}
func (p *Paths) SymlinkPath(username, fontname, filename string) string {
	symlinkName := username + "--" + fontname + "--" + filename
	return filepath.Join(p.ActivationDir, symlinkName)
}

// EnsurePackageDir creates the directory for a specific package version.
func (p *Paths) EnsurePackageDir(username, fontname, version string) error {
	dir := p.PackagePath(username, fontname, version)
	return os.MkdirAll(dir, 0755)
}

// RemovePackage removes all versions of a package.
func (p *Paths) RemovePackage(username, fontname string) error {
	dir := filepath.Join(p.Packages, username, fontname)
	return os.RemoveAll(dir)
}

// PackageExists checks if a specific package version exists.
func (p *Paths) PackageExists(username, fontname, version string) bool {
	dir := p.PackagePath(username, fontname, version)
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
