package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) runLSRemote(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return a.fail("ls-remote", &CLIError{Code: "INPUT_REQUIRED", Message: "ls-remote does not accept positional arguments", Details: map[string]any{}})
	}
	root, err := a.Client.GetRootIndex(ctx)
	if err != nil {
		return a.fail("ls-remote", asCLIError(err))
	}
	packageIDs := make([]string, 0, len(root.Packages))
	for packageID := range root.Packages {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	packages := make([]map[string]any, 0, len(packageIDs))
	for _, packageID := range packageIDs {
		entry := root.Packages[packageID]
		packages = append(packages, map[string]any{
			"package_id":          packageID,
			"latest_version":      entry.LatestVersion,
			"latest_version_key":  entry.LatestVersionKey,
			"latest_published_at": entry.LatestPublishedAt,
		})
	}
	data := map[string]any{"packages": packages}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "ls-remote", Data: data})
	}
	if len(packages) == 0 {
		printNoPublishedPackages(a.Stdout)
		return 0
	}
	fmt.Fprintln(a.Stdout, "Available packages:")
	packageWidth := len("package")
	versionWidth := len("latest")
	for _, pkg := range packages {
		if n := len(stringValue(pkg["package_id"])); n > packageWidth {
			packageWidth = n
		}
		if n := len(stringValue(pkg["latest_version"])); n > versionWidth {
			versionWidth = n
		}
	}
	for _, pkg := range packages {
		fmt.Fprintf(
			a.Stdout,
			"  - %-*s  latest %-*s  published %s\n",
			packageWidth,
			pkg["package_id"],
			versionWidth,
			pkg["latest_version"],
			humanDate(stringValue(pkg["latest_published_at"])),
		)
	}
	return 0
}

func humanDate(value string) string {
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.Format("2006-01-02")
	}
	return value
}

func (a *App) runShow(ctx context.Context, args []string) int {
	opts, errObj := parseShowOptions(args)
	if errObj != nil {
		return a.fail("show", errObj)
	}
	detail, err := a.resolvePackageDetail(ctx, opts.PackageID, opts.Version)
	if err != nil {
		return a.fail("show", asCLIError(err))
	}
	data, cliErr := structToMap(detail)
	if cliErr != nil {
		return a.fail("show", cliErr)
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "show", Data: data})
	}
	printPackageDetailSummary(a.Stdout, detail)
	return 0
}

func (a *App) runLS(_ context.Context, args []string) int {
	activationDir, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("ls", errObj)
	}
	if activationDir == "" {
		activationDir = a.Config.DefaultActivationDir
	}
	var target string
	if len(rest) > 1 {
		return a.fail("ls", &CLIError{Code: "INPUT_REQUIRED", Message: "ls accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}

	lock, ok, err := LockfileStore{Path: a.Config.LockfilePath()}.Load()
	if err != nil {
		return a.fail("ls", asCLIError(err))
	}
	packagesData := map[string]any{}
	if ok {
		packageIDs := make([]string, 0, len(lock.Packages))
		for packageID := range lock.Packages {
			packageIDs = append(packageIDs, packageID)
		}
		sort.Strings(packageIDs)
		for _, packageID := range packageIDs {
			if target != "" && packageID != target {
				continue
			}
			pkg := lock.Packages[packageID]
			installed := make([]any, 0, len(pkg.InstalledVersions))
			for _, versionKey := range SortedInstalledVersionKeys(pkg) {
				installed = append(installed, versionKey)
			}
			var active any
			if pkg.ActiveVersionKey != nil {
				active = *pkg.ActiveVersionKey
			}
			packagesData[packageID] = map[string]any{
				"installed_versions": installed,
				"active_version_key": active,
			}
		}
	}
	if target != "" {
		if _, exists := packagesData[target]; !exists {
			return a.fail("ls", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
		}
	}

	data := map[string]any{"packages": packagesData}
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "ls", Data: data}
		if err := protocol.ValidateStatusResult(env); err != nil {
			return a.fail("ls", &CLIError{Code: "INTERNAL_ERROR", Message: "ls output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	if len(packagesData) == 0 {
		printNoInstalledPackages(a.Stdout)
		return 0
	}
	packageIDs := make([]string, 0, len(packagesData))
	for packageID := range packagesData {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	for _, packageID := range packageIDs {
		entry := packagesData[packageID].(map[string]any)
		active := "none"
		if entry["active_version_key"] != nil {
			active = entry["active_version_key"].(string)
		}
		versions := entry["installed_versions"].([]any)
		versionTexts := make([]string, 0, len(versions))
		for _, version := range versions {
			versionTexts = append(versionTexts, version.(string))
		}
		fmt.Fprintf(a.Stdout, "%s\n", packageID)
		fmt.Fprintf(a.Stdout, "  installed versions: %s\n", strings.Join(versionTexts, ", "))
		fmt.Fprintf(a.Stdout, "  active version: %s\n", active)
		pkg := lock.Packages[packageID]
		dirText, statusText := humanStatusActivationSummary(pkg, activationDir)
		fmt.Fprintf(a.Stdout, "  activation dir: %s\n", dirText)
		fmt.Fprintf(a.Stdout, "  activation status: %s\n", statusText)
	}
	return 0
}

func humanStatusActivationSummary(pkg protocol.LockedPackage, activationDir string) (string, string) {
	if activationDir == "" {
		if pkg.ActiveVersionKey == nil {
			return "not set", "inactive"
		}
		return "not set", "not checked (pass --activation-dir or set FONTPUB_ACTIVATION_DIR)"
	}
	if pkg.ActiveVersionKey == nil {
		return activationDir, "inactive"
	}
	version, ok := pkg.InstalledVersions[*pkg.ActiveVersionKey]
	if !ok {
		return activationDir, "broken (active version missing from lockfile)"
	}
	total := len(version.Assets)
	if total == 0 {
		return activationDir, "inactive"
	}
	linked := 0
	for _, asset := range version.Assets {
		if activationLinkMatches(asset, activationDir) {
			linked++
		}
	}
	if linked == total {
		return activationDir, fmt.Sprintf("active (%d/%d assets linked)", linked, total)
	}
	return activationDir, fmt.Sprintf("broken (%d/%d assets linked)", linked, total)
}

func (a *App) runVerify(_ context.Context, args []string) int {
	activationDir, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("verify", errObj)
	}
	var target string
	if len(rest) > 1 {
		return a.fail("verify", &CLIError{Code: "INPUT_REQUIRED", Message: "verify accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return a.fail("verify", asCLIError(err))
	}
	results := make([]PackageCheckResult, 0)
	if ok {
		for packageID, pkg := range lock.Packages {
			if target != "" && packageID != target {
				continue
			}
			findings := make([]Finding, 0)
			for _, version := range pkg.InstalledVersions {
				for _, asset := range version.Assets {
					if finding := verifyLockedAsset(asset); finding != nil {
						findings = append(findings, *finding)
					}
					if activationDir != "" && asset.Active && asset.SymlinkPath != nil && filepath.Dir(*asset.SymlinkPath) != activationDir {
						findings = append(findings, Finding{
							Code:     "ACTIVATION_BROKEN",
							Severity: "error",
							Subject:  "activation",
							Message:  "active symlink is outside the selected activation directory",
							Details:  map[string]any{"path": asset.Path, "symlink_path": *asset.SymlinkPath},
						})
					}
				}
			}
			sortFindings(findings)
			results = append(results, PackageCheckResult{PackageID: packageID, OK: len(findings) == 0, Findings: findings})
		}
	}
	if target != "" && len(results) == 0 {
		return a.fail("verify", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
	}
	allOK := true
	for _, result := range results {
		allOK = allOK && result.OK
	}
	if !allOK {
		return a.writePackageFailure("verify", "verification failed", results)
	}
	data := packageResultsToDetails(results)
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "verify", Data: data}
		if err := protocol.ValidateVerifyResult(env); err != nil {
			return a.fail("verify", &CLIError{Code: "INTERNAL_ERROR", Message: "verify output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	if len(results) == 0 {
		printNoInstalledPackages(a.Stdout)
		return 0
	}
	printPackageCheckResults(a.Stdout, "Verification results:", results)
	return 0
}
