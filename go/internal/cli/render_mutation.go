package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func plannedActionCount(planned []PlannedAction, actionType string) int {
	count := 0
	for _, action := range planned {
		if action.Type == actionType {
			count++
		}
	}
	return count
}

func plannedPackageVersions(planned []PlannedAction) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, action := range planned {
		if action.PackageID == "" || action.VersionKey == "" {
			continue
		}
		key := concisePackageVersion(action.PackageID, action.VersionKey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func printDetailLine(w io.Writer, label string, value any) {
	fmt.Fprintf(w, "  %s: %v\n", label, value)
}

func printPackageVersionLines(w io.Writer, heading string, packageVersions []string) {
	if len(packageVersions) == 0 {
		return
	}
	fmt.Fprintln(w, heading)
	for _, item := range packageVersions {
		fmt.Fprintf(w, "  - %s\n", item)
	}
}

func formatVersions(versionKeys []string) string {
	sorted := append([]string(nil), versionKeys...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

func packageInstallDisplayRoot(stateDir, packageID, versionKey string) string {
	return filepath.Clean(packageInstallRoot(stateDir, packageID, versionKey))
}

func (a *App) writeInstallResult(packageID, versionKey string, activate bool, activationDir string, changed, dryRun bool, planned []PlannedAction) int {
	if a.JSON {
		return a.writeMutationResult("install", changed, dryRun, planned)
	}
	if activate && activationDir == "" {
		activationDir = a.Config.DefaultActivationDir
	}
	if !changed {
		fmt.Fprintf(a.Stdout, "%s is already installed\n", concisePackageVersion(packageID, versionKey))
		return 0
	}
	if dryRun {
		fmt.Fprintf(a.Stdout, "Install plan for %s\n", concisePackageVersion(packageID, versionKey))
	} else if activate {
		fmt.Fprintf(a.Stdout, "Installed and activated %s\n", concisePackageVersion(packageID, versionKey))
	} else {
		fmt.Fprintf(a.Stdout, "Installed %s\n", concisePackageVersion(packageID, versionKey))
	}
	printDetailLine(a.Stdout, "assets", plannedActionCount(planned, "write_asset"))
	printDetailLine(a.Stdout, "install root", packageInstallDisplayRoot(a.Config.StateDir, packageID, versionKey))
	if activate {
		printDetailLine(a.Stdout, "activation dir", activationDir)
		printDetailLine(a.Stdout, "symlinks created", plannedActionCount(planned, "create_symlink"))
	}
	if dryRun {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) writeActivateResult(packageID, versionKey, activationDir string, dryRun bool, planned []PlannedAction) int {
	if a.JSON {
		return a.writeMutationResult("activate", true, dryRun, planned)
	}
	if dryRun {
		fmt.Fprintf(a.Stdout, "Activation plan for %s\n", concisePackageVersion(packageID, versionKey))
	} else {
		fmt.Fprintf(a.Stdout, "Activated %s\n", concisePackageVersion(packageID, versionKey))
	}
	printDetailLine(a.Stdout, "activation dir", activationDir)
	printDetailLine(a.Stdout, "symlinks created", plannedActionCount(planned, "create_symlink"))
	removed := plannedActionCount(planned, "remove_symlink")
	if removed > 0 {
		printDetailLine(a.Stdout, "symlinks removed", removed)
	}
	if dryRun {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) writeDeactivateResult(packageID string, changed, dryRun bool, planned []PlannedAction) int {
	if a.JSON {
		return a.writeMutationResult("deactivate", changed, dryRun, planned)
	}
	if !changed {
		fmt.Fprintf(a.Stdout, "%s is already inactive\n", packageID)
		return 0
	}
	if dryRun {
		fmt.Fprintf(a.Stdout, "Deactivation plan for %s\n", packageID)
	} else {
		fmt.Fprintf(a.Stdout, "Deactivated %s\n", packageID)
	}
	printDetailLine(a.Stdout, "symlinks removed", plannedActionCount(planned, "remove_symlink"))
	if dryRun {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) writeUninstallResult(packageID string, targetVersions []string, changed, dryRun bool, planned []PlannedAction) int {
	if a.JSON {
		return a.writeMutationResult("uninstall", changed, dryRun, planned)
	}
	if !changed {
		fmt.Fprintf(a.Stdout, "No installed versions were removed for %s\n", packageID)
		return 0
	}
	if dryRun {
		fmt.Fprintf(a.Stdout, "Uninstall plan for %s\n", packageID)
	} else if len(targetVersions) == 1 {
		fmt.Fprintf(a.Stdout, "Uninstalled %s\n", concisePackageVersion(packageID, targetVersions[0]))
	} else {
		fmt.Fprintf(a.Stdout, "Uninstalled %d versions of %s\n", len(targetVersions), packageID)
	}
	printDetailLine(a.Stdout, "versions", formatVersions(targetVersions))
	printDetailLine(a.Stdout, "assets removed", plannedActionCount(planned, "remove_asset"))
	removed := plannedActionCount(planned, "remove_symlink")
	if removed > 0 {
		printDetailLine(a.Stdout, "symlinks removed", removed)
	}
	if dryRun {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) writeUpdateResult(target string, changed, activate, dryRun bool, activationDir string, planned []PlannedAction) int {
	if a.JSON {
		return a.writeMutationResult("update", changed, dryRun, planned)
	}
	if activate && activationDir == "" {
		activationDir = a.Config.DefaultActivationDir
	}
	if !changed {
		if target != "" {
			fmt.Fprintf(a.Stdout, "%s is already up to date\n", target)
		} else {
			fmt.Fprintln(a.Stdout, "All installed packages are already up to date")
		}
		return 0
	}
	pkgs := plannedPackageVersions(planned)
	if dryRun {
		fmt.Fprintf(a.Stdout, "Update plan for %d package(s)\n", len(pkgs))
	} else {
		fmt.Fprintf(a.Stdout, "Updated %d package(s)\n", len(pkgs))
	}
	printPackageVersionLines(a.Stdout, "Packages:", pkgs)
	printDetailLine(a.Stdout, "assets written", plannedActionCount(planned, "write_asset"))
	if activate {
		printDetailLine(a.Stdout, "activation dir", activationDir)
		printDetailLine(a.Stdout, "symlinks created", plannedActionCount(planned, "create_symlink"))
	}
	if dryRun {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) writeRepairResult(results []PackageCheckResult, repaired []any, changed, dryRun bool, planned []PlannedAction) int {
	data := map[string]any{"changed": changed, "repaired_packages": repaired}
	if dryRun {
		data["planned_actions"] = plannedActionsToAny(planned)
	}
	if a.JSON {
		return a.writeValidatedJSONSuccess("repair", data, protocol.ValidateRepairResult, "repair output validation failed")
	}
	if len(results) == 0 {
		printNoInstalledPackages(a.Stdout)
		return 0
	}
	if !changed {
		fmt.Fprintln(a.Stdout, "Repair results:")
		for _, result := range results {
			fmt.Fprintf(a.Stdout, "  %s: no changes\n", result.PackageID)
		}
		fmt.Fprintf(a.Stdout, "  symlinks created: %d\n", plannedActionCount(planned, "create_symlink"))
		fmt.Fprintf(a.Stdout, "  symlinks removed: %d\n", plannedActionCount(planned, "remove_symlink"))
		return 0
	}
	fmt.Fprintln(a.Stdout, "Repair results:")
	for _, item := range repaired {
		fmt.Fprintf(a.Stdout, "  %s: repaired\n", item)
	}
	fmt.Fprintf(a.Stdout, "  symlinks created: %d\n", plannedActionCount(planned, "create_symlink"))
	fmt.Fprintf(a.Stdout, "  symlinks removed: %d\n", plannedActionCount(planned, "remove_symlink"))
	if dryRun && len(planned) > 0 {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}
