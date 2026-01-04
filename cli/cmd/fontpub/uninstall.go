package main

import (
	"fmt"

	"github.com/fontpub/cli/pkg/activation"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [username/fontname]",
	Short: "Uninstall a font package",
	Long: `Completely remove a font package from the system.

This will:
1. Deactivate the package if it's active
2. Delete the font files from ~/.fontpub/packages/
3. Remove the entry from fontpub.lock`,
	Args: cobra.ExactArgs(1),
	RunE: runUninstall,
}

func runUninstall(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("package %s is not installed", pkg.String())
	}

	fmt.Printf("Uninstalling %s@%s...\n", pkg.String(), pkgEntry.Version)

	// Deactivate if active
	if pkgEntry.Status == lockfile.StatusActive {
		fmt.Println("  Deactivating...")
		mgr := activation.NewManager(paths.ActivationDir)
		if err := mgr.RemovePackageSymlinks(pkg.Username, pkg.Fontname); err != nil {
			return fmt.Errorf("failed to deactivate: %w", err)
		}
	}

	// Remove package files
	fmt.Println("  Removing files...")
	if err := paths.RemovePackage(pkg.Username, pkg.Fontname); err != nil {
		return fmt.Errorf("failed to remove package files: %w", err)
	}

	// Remove from lockfile
	lf.RemovePackage(pkg.String())

	if err := lf.Save(paths.Root); err != nil {
		return fmt.Errorf("failed to save lockfile: %w", err)
	}

	fmt.Printf("Successfully uninstalled %s\n", pkg.String())
	return nil
}
