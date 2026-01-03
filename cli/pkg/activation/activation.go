// Package activation manages symlink creation/removal for font activation.
package activation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles symlink operations for font activation.
type Manager struct {
	ActivationDir string
}

// NewManager creates a new activation Manager.
func NewManager(activationDir string) *Manager {
	return &Manager{
		ActivationDir: activationDir,
	}
}

// EnsureDir creates the activation directory if it doesn't exist.
func (m *Manager) EnsureDir() error {
	return os.MkdirAll(m.ActivationDir, 0755)
}

// BuildSymlinkName creates the symlink name from package and file info.
// Format: {username}--{fontname}--{filename}
func BuildSymlinkName(username, fontname, filename string) string {
	return username + "--" + fontname + "--" + filename
}

// ParseSymlinkName extracts username, fontname, and filename from a symlink name.
func ParseSymlinkName(symlinkName string) (username, fontname, filename string, err error) {
	parts := strings.SplitN(symlinkName, "--", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid symlink name format: %s", symlinkName)
	}
	return parts[0], parts[1], parts[2], nil
}

// CreateSymlink creates a symlink for a font file.
// If a symlink already exists, it will be replaced atomically.
func (m *Manager) CreateSymlink(targetPath, symlinkName string) error {
	if err := m.EnsureDir(); err != nil {
		return err
	}

	symlinkPath := filepath.Join(m.ActivationDir, symlinkName)

	// Remove existing symlink if present
	if _, err := os.Lstat(symlinkPath); err == nil {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("failed to remove existing symlink: %w", err)
		}
	}

	// Create new symlink
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}

// RemoveSymlink removes a symlink by name.
func (m *Manager) RemoveSymlink(symlinkName string) error {
	symlinkPath := filepath.Join(m.ActivationDir, symlinkName)

	// Check if it's actually a symlink
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone, not an error
		}
		return err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("not a symlink: %s", symlinkPath)
	}

	return os.Remove(symlinkPath)
}

// RemovePackageSymlinks removes all symlinks for a given package.
func (m *Manager) RemovePackageSymlinks(username, fontname string) error {
	prefix := username + "--" + fontname + "--"

	entries, err := os.ReadDir(m.ActivationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			if err := m.RemoveSymlink(entry.Name()); err != nil {
				return err
			}
		}
	}

	return nil
}

// ListPackageSymlinks returns all symlink names for a given package.
func (m *Manager) ListPackageSymlinks(username, fontname string) ([]string, error) {
	prefix := username + "--" + fontname + "--"

	entries, err := os.ReadDir(m.ActivationDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var symlinks []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			symlinks = append(symlinks, entry.Name())
		}
	}

	return symlinks, nil
}

// IsActive checks if a package has any active symlinks.
func (m *Manager) IsActive(username, fontname string) (bool, error) {
	symlinks, err := m.ListPackageSymlinks(username, fontname)
	if err != nil {
		return false, err
	}
	return len(symlinks) > 0, nil
}
