package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fontpub/cli/pkg/downloader"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/spf13/cobra"
)

var statusJSONOutput bool

var statusCmd = &cobra.Command{
	Use:   "status [username/fontname]",
	Short: "Show system or package status",
	Long: `Show diagnostic information about the Fontpub system or a specific package.

Without arguments: Shows system-wide health check including storage paths,
lockfile status, and permissions.

With a package name: Shows detailed information about the specified package
including file integrity and symlink verification.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSONOutput, "json", false, "Output in JSON format")
}

// SystemStatus represents the system-wide diagnostic info.
type SystemStatus struct {
	StorageRoot    PathStatus `json:"storage_root"`
	PackagesDir    PathStatus `json:"packages_dir"`
	ActivationDir  PathStatus `json:"activation_dir"`
	Lockfile       FileStatus `json:"lockfile"`
	InstalledCount int        `json:"installed_count"`
	ActiveCount    int        `json:"active_count"`
}

// PathStatus represents the status of a directory path.
type PathStatus struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	Writable  bool   `json:"writable"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

// FileStatus represents the status of a file.
type FileStatus struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

// PackageStatus represents the diagnostic info for a single package.
type PackageStatus struct {
	Name    string      `json:"name"`
	Version string      `json:"version"`
	Status  string      `json:"status"`
	Files   []FileCheck `json:"files"`
	Healthy bool        `json:"healthy"`
}

// FileCheck represents the integrity check for a single file.
type FileCheck struct {
	Filename      string `json:"filename"`
	FilePath      string `json:"file_path"`
	FileExists    bool   `json:"file_exists"`
	HashValid     bool   `json:"hash_valid"`
	ExpectedHash  string `json:"expected_hash"`
	ActualHash    string `json:"actual_hash,omitempty"`
	SymlinkName   string `json:"symlink_name"`
	SymlinkPath   string `json:"symlink_path"`
	SymlinkExists bool   `json:"symlink_exists"`
	SymlinkValid  bool   `json:"symlink_valid"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return runSystemStatus()
	}
	return runPackageStatus(args[0])
}

func runSystemStatus() error {
	paths, err := storage.NewPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	status := SystemStatus{
		StorageRoot:   checkPath(paths.Root),
		PackagesDir:   checkPath(paths.Packages),
		ActivationDir: checkPath(paths.ActivationDir),
		Lockfile:      checkLockfile(paths.Root),
	}

	// Count packages
	lf, err := lockfile.Load(paths.Root)
	if err == nil {
		status.InstalledCount = len(lf.ListPackages())
		status.ActiveCount = len(lf.ListActivePackages())
	}

	if statusJSONOutput {
		return outputJSON(status)
	}
	return outputSystemStatusText(status)
}

func runPackageStatus(packageName string) error {
	pkg, err := pkgname.Parse(packageName)
	if err != nil {
		return err
	}

	paths, err := storage.NewPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	lf, err := lockfile.Load(paths.Root)
	if err != nil {
		return fmt.Errorf("failed to load lockfile: %w", err)
	}

	pkgEntry := lf.GetPackage(pkg.String())
	if pkgEntry == nil {
		return fmt.Errorf("package %s is not installed", pkg.String())
	}

	status := PackageStatus{
		Name:    pkg.String(),
		Version: pkgEntry.Version,
		Status:  string(pkgEntry.Status),
		Healthy: true,
	}

	// Check each file
	for _, file := range pkgEntry.Files {
		filePath := paths.PackageFilePath(pkg.Username, pkg.Fontname, pkgEntry.Version, file.Filename)
		symlinkPath := filepath.Join(paths.ActivationDir, file.SymlinkName)

		check := FileCheck{
			Filename:     file.Filename,
			FilePath:     filePath,
			ExpectedHash: file.SHA256,
			SymlinkName:  file.SymlinkName,
			SymlinkPath:  symlinkPath,
		}

		// Check file exists
		if _, err := os.Stat(filePath); err == nil {
			check.FileExists = true

			// Verify hash
			actualHash, err := downloader.CalculateHash(filePath)
			if err == nil {
				check.ActualHash = actualHash
				check.HashValid = actualHash == file.SHA256
			}
		}

		// Check symlink (only if package is active)
		if pkgEntry.Status == lockfile.StatusActive {
			if info, err := os.Lstat(symlinkPath); err == nil {
				check.SymlinkExists = true
				if info.Mode()&os.ModeSymlink != 0 {
					// Verify symlink target
					target, err := os.Readlink(symlinkPath)
					if err == nil {
						check.SymlinkValid = target == filePath
					}
				}
			}
		} else {
			// For inactive packages, symlink should not exist
			if _, err := os.Lstat(symlinkPath); os.IsNotExist(err) {
				check.SymlinkValid = true // correct: no symlink for inactive package
			}
		}

		// Update healthy status
		if !check.FileExists || !check.HashValid {
			status.Healthy = false
		}
		if pkgEntry.Status == lockfile.StatusActive && (!check.SymlinkExists || !check.SymlinkValid) {
			status.Healthy = false
		}

		status.Files = append(status.Files, check)
	}

	if statusJSONOutput {
		return outputJSON(status)
	}
	return outputPackageStatusText(status)
}

func checkPath(path string) PathStatus {
	status := PathStatus{Path: path}

	info, err := os.Stat(path)
	if err != nil {
		return status
	}

	status.Exists = true

	// Check writable by trying to create a temp file
	testFile := filepath.Join(path, ".fontpub_write_test")
	if f, err := os.Create(testFile); err == nil {
		f.Close()
		os.Remove(testFile)
		status.Writable = true
	}

	// Calculate size (only for directories)
	if info.IsDir() {
		var size int64
		filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		status.SizeBytes = size
	}

	return status
}

func checkLockfile(rootDir string) FileStatus {
	path := filepath.Join(rootDir, lockfile.LockfileName)
	status := FileStatus{Path: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		status.Valid = true // Non-existent lockfile is valid (will be created)
		return status
	}

	status.Exists = true

	// Try to read and parse
	lf, err := lockfile.Load(rootDir)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.Readable = true
	status.Valid = lf.LockfileVersion == lockfile.LockfileVersion

	return status
}

func outputJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputSystemStatusText(s SystemStatus) error {
	fmt.Println("Fontpub System Status")
	fmt.Println("=====================")
	fmt.Println()

	fmt.Println("Storage:")
	printPathStatus("  Root", s.StorageRoot)
	printPathStatus("  Packages", s.PackagesDir)
	printPathStatus("  Activation", s.ActivationDir)
	fmt.Println()

	fmt.Println("Lockfile:")
	if s.Lockfile.Exists {
		fmt.Printf("  Path: %s\n", s.Lockfile.Path)
		if s.Lockfile.Valid {
			fmt.Println("  Status: OK")
		} else {
			fmt.Printf("  Status: Invalid (%s)\n", s.Lockfile.Error)
		}
	} else {
		fmt.Println("  Status: Not created yet (will be created on first install)")
	}
	fmt.Println()

	fmt.Println("Packages:")
	fmt.Printf("  Installed: %d\n", s.InstalledCount)
	fmt.Printf("  Active: %d\n", s.ActiveCount)

	return nil
}

func printPathStatus(label string, s PathStatus) {
	if s.Exists {
		writable := "writable"
		if !s.Writable {
			writable = "read-only"
		}
		if s.SizeBytes > 0 {
			fmt.Printf("%s: %s (%s, %s)\n", label, s.Path, writable, formatBytes(s.SizeBytes))
		} else {
			fmt.Printf("%s: %s (%s)\n", label, s.Path, writable)
		}
	} else {
		fmt.Printf("%s: %s (not created)\n", label, s.Path)
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func outputPackageStatusText(s PackageStatus) error {
	fmt.Printf("Package: %s\n", s.Name)
	fmt.Printf("Version: %s\n", s.Version)
	fmt.Printf("Status: %s\n", s.Status)
	fmt.Println()

	healthIcon := "✓"
	if !s.Healthy {
		healthIcon = "✗"
	}
	fmt.Printf("Health: %s %s\n", healthIcon, map[bool]string{true: "Healthy", false: "Issues detected"}[s.Healthy])
	fmt.Println()

	fmt.Println("Files:")
	for _, f := range s.Files {
		fmt.Printf("  %s\n", f.Filename)

		// File status
		if f.FileExists {
			if f.HashValid {
				fmt.Printf("    File: ✓ %s\n", f.FilePath)
			} else {
				fmt.Printf("    File: ✗ %s (hash mismatch)\n", f.FilePath)
				fmt.Printf("           Expected: %s\n", f.ExpectedHash)
				fmt.Printf("           Actual:   %s\n", f.ActualHash)
			}
		} else {
			fmt.Printf("    File: ✗ %s (missing)\n", f.FilePath)
		}

		// Symlink status (only for active packages)
		if s.Status == "active" {
			if f.SymlinkExists && f.SymlinkValid {
				fmt.Printf("    Link: ✓ %s\n", f.SymlinkPath)
			} else if f.SymlinkExists && !f.SymlinkValid {
				fmt.Printf("    Link: ✗ %s (points to wrong target)\n", f.SymlinkPath)
			} else {
				fmt.Printf("    Link: ✗ %s (missing)\n", f.SymlinkPath)
			}
		}
	}

	return nil
}
