package main

import (
	"fmt"

	"github.com/fontpub/cli/pkg/activation"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/spf13/cobra"
)

var deactivateCmd = &cobra.Command{
	Use:   "deactivate [username/fontname]",
	Short: "Deactivate a font package",
	Long: `Deactivate a font package by removing its symlinks from ~/Library/Fonts/from_fontpub/.

The font files remain installed in ~/.fontpub/ and can be reactivated later.`,
	Args: cobra.ExactArgs(1),
	RunE: runDeactivate,
}

func runDeactivate(cmd *cobra.Command, args []string) error {
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

	// Check if already inactive
	if pkgEntry.Status == lockfile.StatusInactive {
		fmt.Printf("Package %s is already inactive\n", pkg.String())
		return nil
	}

	// Remove symlinks
	fmt.Printf("Deactivating %s...\n", pkg.String())
	mgr := activation.NewManager(paths.ActivationDir)

	if err := mgr.RemovePackageSymlinks(pkg.Username, pkg.Fontname); err != nil {
		return fmt.Errorf("failed to remove symlinks: %w", err)
	}

	// Update lockfile status
	if err := lf.SetStatus(pkg.String(), lockfile.StatusInactive); err != nil {
		return err
	}

	if err := lf.Save(paths.Root); err != nil {
		return fmt.Errorf("failed to save lockfile: %w", err)
	}

	fmt.Printf("Successfully deactivated %s\n", pkg.String())
	fmt.Printf("Font files remain in ~/.fontpub/. Run 'fontpub activate %s' to reactivate.\n", pkg.String())
	return nil
}
