package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/fontpub/cli/pkg/index"
	"github.com/fontpub/cli/pkg/lockfile"
	"github.com/fontpub/cli/pkg/storage"
	"github.com/fontpub/cli/pkg/version"
	"github.com/spf13/cobra"
)

var listJSONOutput bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed font packages",
	Long: `List all installed font packages with their versions and status.

Shows the current installed version, latest available version from the index,
and whether the package is active or inactive.`,
	RunE: runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSONOutput, "json", false, "Output in JSON format")
}

// PackageListItem represents a package in the list output.
type PackageListItem struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	LatestVersion   string `json:"latest_version,omitempty"`
	Status          string `json:"status"`
	UpdateAvailable bool   `json:"update_available"`
}

func runList(cmd *cobra.Command, args []string) error {
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
		if listJSONOutput {
			fmt.Println("[]")
		} else {
			fmt.Println("No packages installed.")
		}
		return nil
	}

	// Sort packages alphabetically
	sort.Strings(packages)

	// Try to fetch index for version comparison
	var idx *index.Index
	client := index.NewClient()
	idx, indexErr := client.FetchIndex()
	if indexErr != nil && !listJSONOutput {
		fmt.Fprintf(os.Stderr, "Warning: Could not fetch index, latest version info unavailable\n\n")
	}

	// Build list items
	items := make([]PackageListItem, 0, len(packages))
	for _, pkgName := range packages {
		pkgEntry := lf.GetPackage(pkgName)
		if pkgEntry == nil {
			continue
		}

		item := PackageListItem{
			Name:    pkgName,
			Version: pkgEntry.Version,
			Status:  string(pkgEntry.Status),
		}

		// Check for updates if index is available
		if idx != nil {
			if pkgInfo, err := idx.GetPackage(pkgName); err == nil {
				item.LatestVersion = pkgInfo.LatestVersion
				if isNewer, _ := version.IsNewer(pkgInfo.LatestVersion, pkgEntry.Version); isNewer {
					item.UpdateAvailable = true
				}
			}
		}

		items = append(items, item)
	}

	// Output
	if listJSONOutput {
		return outputListJSON(items)
	}
	return outputListTable(items)
}

func outputListJSON(items []PackageListItem) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func outputListTable(items []PackageListItem) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "PACKAGE\tVERSION\tLATEST\tSTATUS")
	fmt.Fprintln(w, "-------\t-------\t------\t------")

	for _, item := range items {
		latest := item.LatestVersion
		if latest == "" {
			latest = "-"
		} else if item.UpdateAvailable {
			latest = latest + " *"
		}

		status := item.Status
		if status == "active" {
			status = "[active]"
		} else {
			status = "[inactive]"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Name, item.Version, latest, status)
	}

	return nil
}
