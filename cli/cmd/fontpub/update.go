package main

import (
	"fmt"

	"github.com/fontpub/cli/pkg/activation"
	"github.com/fontpub/cli/pkg/downloader"
	"github.com/fontpub/cli/pkg/index"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/fontpub/cli/pkg/version"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all installed font packages",
	Long: `Check for updates to all installed font packages and download newer versions.

If a package was active, the symlinks will be updated to point to the new version.`,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
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

	packages := lf.ListPackages()
	if len(packages) == 0 {
		fmt.Println("No packages installed.")
		return nil
	}

	// Fetch index
	fmt.Println("Fetching package index...")
	client := index.NewClient()
	idx, err := client.Fetch()
	if err != nil {
		return fmt.Errorf("failed to fetch index: %w", err)
	}

	dl := downloader.New()
	mgr := activation.NewManager(paths.ActivationDir)

	updatedCount := 0
	for _, pkgName := range packages {
		pkgEntry := lf.GetPackage(pkgName)
		if pkgEntry == nil {
			continue
		}

		pkg, err := pkgname.Parse(pkgName)
		if err != nil {
			fmt.Printf("Warning: invalid package name %s, skipping\n", pkgName)
			continue
		}

		// Get latest version from index
		pkgInfo, err := idx.GetPackage(pkgName)
		if err != nil {
			fmt.Printf("Warning: package %s not found in index, skipping\n", pkgName)
			continue
		}

		// Compare versions
		isNewer, err := version.IsNewer(pkgInfo.LatestVersion, pkgEntry.Version)
		if err != nil {
			fmt.Printf("Warning: failed to compare versions for %s: %v\n", pkgName, err)
			continue
		}

		if !isNewer {
			fmt.Printf("%s is up to date (%s)\n", pkgName, pkgEntry.Version)
			continue
		}

		// Download new version
		newVersion := pkgInfo.LatestVersion
		fmt.Printf("Updating %s: %s -> %s\n", pkgName, pkgEntry.Version, newVersion)

		var files []lockfile.FileEntry
		for filename, asset := range pkgInfo.Assets {
			destPath := paths.PackageFilePath(pkg.Username, pkg.Fontname, newVersion, filename)
			fmt.Printf("  Downloading %s...\n", filename)

			if err := dl.Download(asset.URL, destPath, asset.SHA256); err != nil {
				return fmt.Errorf("failed to download %s: %w", filename, err)
			}

			files = append(files, lockfile.FileEntry{
				Filename:    filename,
				SHA256:      asset.SHA256,
				SymlinkName: activation.BuildSymlinkName(pkg.Username, pkg.Fontname, filename),
			})
		}

		// If package was active, update symlinks
		wasActive := pkgEntry.Status == lockfile.StatusActive
		if wasActive {
			// Remove old symlinks
			if err := mgr.RemovePackageSymlinks(pkg.Username, pkg.Fontname); err != nil {
				return fmt.Errorf("failed to remove old symlinks: %w", err)
			}

			// Create new symlinks
			for _, file := range files {
				targetPath := paths.PackageFilePath(pkg.Username, pkg.Fontname, newVersion, file.Filename)
				if err := mgr.CreateSymlink(targetPath, file.SymlinkName); err != nil {
					return fmt.Errorf("failed to create symlink for %s: %w", file.Filename, err)
				}
			}
		}

		// Update lockfile
		lf.SetPackage(pkgName, &lockfile.PackageEntry{
			Version: newVersion,
			Status:  pkgEntry.Status, // preserve active/inactive status
			Files:   files,
		})

		updatedCount++
	}

	if err := lf.Save(paths.Root); err != nil {
		return fmt.Errorf("failed to save lockfile: %w", err)
	}

	if updatedCount == 0 {
		fmt.Println("All packages are up to date.")
	} else {
		fmt.Printf("Updated %d package(s).\n", updatedCount)
	}

	return nil
}
