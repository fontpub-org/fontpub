package main

import (
	"fmt"
	"path/filepath"

	"github.com/fontpub/cli/pkg/activation"
	"github.com/fontpub/cli/pkg/downloader"
	"github.com/fontpub/cli/pkg/index"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/pkgname"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [username/fontname]",
	Short: "Install a font package",
	Long: `Install a font package from the Fontpub index.

This command will:
1. Fetch the package index
2. Download the latest version of the specified font
3. Verify the SHA256 hash
4. Store it in ~/.fontpub/packages/`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func runInstall(cmd *cobra.Command, args []string) error {
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

	// Ensure directories exist
	if err := paths.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Load lockfile
	lf, err := lockfile.Load(paths.Root)
	if err != nil {
		return fmt.Errorf("failed to load lockfile: %w", err)
	}

	// Step 1: Fetch root index to get latest version
	fmt.Println("Fetching package index...")
	client := index.NewClient()
	idx, err := client.FetchIndex()
	if err != nil {
		return fmt.Errorf("failed to fetch index: %w", err)
	}

	// Find package in index
	pkgSummary, err := idx.GetPackage(pkg.String())
	if err != nil {
		return err
	}

	version := pkgSummary.LatestVersion
	fmt.Printf("Found %s version %s\n", pkg.String(), version)

	// Check if already installed
	if paths.PackageExists(pkg.Username, pkg.Fontname, version) {
		existing := lf.GetPackage(pkg.String())
		if existing != nil && existing.Version == version {
			fmt.Printf("Package %s@%s is already installed\n", pkg.String(), version)
			return nil
		}
	}

	// Step 2: Fetch package detail to get asset information
	fmt.Println("Fetching package details...")
	detail, err := client.FetchPackageDetail(pkg.String())
	if err != nil {
		return fmt.Errorf("failed to fetch package details: %w", err)
	}

	// Download and verify each asset
	fmt.Println("Downloading fonts...")
	dl := downloader.New()

	var files []lockfile.FileEntry
	for _, asset := range detail.Assets {
		// Extract filename from path
		filename := filepath.Base(asset.Path)
		destPath := paths.PackageFilePath(pkg.Username, pkg.Fontname, version, filename)
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

	// Update lockfile
	lf.SetPackage(pkg.String(), &lockfile.PackageEntry{
		Version: version,
		Status:  lockfile.StatusInactive,
		Files:   files,
	})

	if err := lf.Save(paths.Root); err != nil {
		return fmt.Errorf("failed to save lockfile: %w", err)
	}

	fmt.Printf("Successfully installed %s@%s\n", pkg.String(), version)
	fmt.Printf("Run 'fontpub activate %s' to make it available to applications.\n", pkg.String())

	return nil
}
