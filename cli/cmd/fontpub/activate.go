package main

import (
	"fmt"

	"github.com/fontpub/cli/pkg/activation"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/spf13/cobra"
)

var activateCmd = &cobra.Command{
	Use:   "activate [username/fontname]",
	Short: "Activate an installed font package",
	Long: `Activate a font package by creating symlinks in ~/Library/Fonts/from_fontpub/.

This makes the font available to macOS applications.`,
	Args: cobra.ExactArgs(1),
	RunE: runActivate,
}

func runActivate(cmd *cobra.Command, args []string) error {
	// Parse package name
	pkg, err := pkgname.Parse(args[0])
	if err != nil {
		return err
	}

	// Initialize paths
	paths, err := storage.NewPaths()
	if err != nil {
		return fmt.Errorf("failed to get paths: %w", err)
	}

	// Load lockfile
	lf, err := lockfile.Load(paths.Root)
	if err != nil {
		return fmt.Errorf("failed to load lockfile: %w", err)
	}

	// Find package in lockfile
	pkgEntry := lf.GetPackage(pkg.String())
	if pkgEntry == nil {
		return fmt.Errorf("package %s is not installed. Run 'fontpub install %s' first", pkg.String(), pkg.String())
	}

	// Check if already active
	if pkgEntry.Status == lockfile.StatusActive {
		fmt.Printf("Package %s is already active\n", pkg.String())
		return nil
	}

	// Create activation manager
	mgr := activation.NewManager(paths.ActivationDir)

	// Ensure activation directory exists
	if err := mgr.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create activation directory: %w", err)
	}

	// Create symlinks for each file
	fmt.Printf("Activating %s@%s...\n", pkg.String(), pkgEntry.Version)
	for _, file := range pkgEntry.Files {
		targetPath := paths.PackageFilePath(pkg.Username, pkg.Fontname, pkgEntry.Version, file.Filename)
		if err := mgr.CreateSymlink(targetPath, file.SymlinkName); err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", file.Filename, err)
		}
		fmt.Printf("  Linked %s\n", file.Filename)
	}

	// Update lockfile status
	if err := lf.SetStatus(pkg.String(), lockfile.StatusActive); err != nil {
		return err
	}

	if err := lf.Save(paths.Root); err != nil {
		return fmt.Errorf("failed to save lockfile: %w", err)
	}

	fmt.Printf("Successfully activated %s\n", pkg.String())
	return nil
}
