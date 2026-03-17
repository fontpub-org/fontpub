package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type CLIError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *CLIError) Error() string {
	return e.Code + ": " + e.Message
}

type App struct {
	Config  Config
	Client  *MetadataClient
	Stdout  io.Writer
	Stderr  io.Writer
	JSON    bool
	Command string
	Now     func() time.Time
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	app := App{
		Config: DefaultConfig(),
		Stdout: stdout,
		Stderr: stderr,
	}
	return app.Run(ctx, args)
}

func (a *App) Run(ctx context.Context, args []string) int {
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			a.JSON = true
			continue
		}
		rest = append(rest, arg)
	}
	if len(rest) == 0 {
		return a.fail("", &CLIError{Code: "INPUT_REQUIRED", Message: "command is required", Details: map[string]any{}})
	}
	a.Command = strings.Join(commandPath(rest), " ")
	if a.Client == nil {
		a.Client = NewMetadataClient(a.Config)
	}

	switch rest[0] {
	case "list":
		return a.runList(ctx, rest[1:])
	case "show":
		return a.runShow(ctx, rest[1:])
	case "install":
		return a.runInstall(ctx, rest[1:])
	case "activate":
		return a.runActivate(ctx, rest[1:])
	case "deactivate":
		return a.runDeactivate(ctx, rest[1:])
	case "status":
		return a.runStatus(ctx, rest[1:])
	case "verify":
		return a.runVerify(ctx, rest[1:])
	case "repair":
		return a.runRepair(ctx, rest[1:])
	case "uninstall":
		return a.runUninstall(ctx, rest[1:])
	case "update":
		return a.runUpdate(ctx, rest[1:])
	default:
		return a.fail(a.Command, &CLIError{Code: "INTERNAL_ERROR", Message: "command is not implemented", Details: map[string]any{"command": rest[0]}})
	}
}

func (a *App) runList(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return a.fail("list", &CLIError{Code: "INPUT_REQUIRED", Message: "list does not accept positional arguments", Details: map[string]any{}})
	}
	root, err := a.Client.GetRootIndex(ctx)
	if err != nil {
		return a.fail("list", asCLIError(err))
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
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "list", Data: data})
	}
	for _, pkg := range packages {
		fmt.Fprintf(a.Stdout, "%s %s\n", pkg["package_id"], pkg["latest_version"])
	}
	return 0
}

func (a *App) runShow(ctx context.Context, args []string) int {
	version, rest, errObj := extractStringFlag(args, "--version")
	if errObj != nil {
		return a.fail("show", errObj)
	}
	if len(rest) != 1 {
		return a.fail("show", &CLIError{Code: "INPUT_REQUIRED", Message: "show requires <owner>/<repo>", Details: map[string]any{}})
	}
	packageID := normalizePackageID(rest[0])
	var (
		detail protocol.VersionedPackageDetail
		err    error
	)
	if version == "" {
		detail, err = a.Client.GetLatestPackageDetail(ctx, packageID)
	} else {
		detail, err = a.Client.GetPackageDetailVersion(ctx, packageID, version)
	}
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
	fmt.Fprintf(a.Stdout, "%s %s\n", detail.PackageID, detail.Version)
	fmt.Fprintf(a.Stdout, "name: %s\n", detail.DisplayName)
	fmt.Fprintf(a.Stdout, "author: %s\n", detail.Author)
	fmt.Fprintf(a.Stdout, "license: %s\n", detail.License)
	for _, asset := range detail.Assets {
		fmt.Fprintf(a.Stdout, "asset: %s %s %d\n", asset.Path, asset.Style, asset.Weight)
	}
	return 0
}

func (a *App) runStatus(_ context.Context, args []string) int {
	_, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("status", errObj)
	}
	var target string
	if len(rest) > 1 {
		return a.fail("status", &CLIError{Code: "INPUT_REQUIRED", Message: "status accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}

	lock, ok, err := LockfileStore{Path: a.Config.LockfilePath()}.Load()
	if err != nil {
		return a.fail("status", asCLIError(err))
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
			return a.fail("status", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
		}
	}

	data := map[string]any{"packages": packagesData}
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "status", Data: data}
		if err := protocol.ValidateStatusResult(env); err != nil {
			return a.fail("status", &CLIError{Code: "INTERNAL_ERROR", Message: "status output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	if len(packagesData) == 0 {
		fmt.Fprintln(a.Stdout, "no installed packages")
		return 0
	}
	packageIDs := make([]string, 0, len(packagesData))
	for packageID := range packagesData {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	for _, packageID := range packageIDs {
		entry := packagesData[packageID].(map[string]any)
		active := "inactive"
		if entry["active_version_key"] != nil {
			active = entry["active_version_key"].(string)
		}
		versions := entry["installed_versions"].([]any)
		versionTexts := make([]string, 0, len(versions))
		for _, version := range versions {
			versionTexts = append(versionTexts, version.(string))
		}
		fmt.Fprintf(a.Stdout, "%s installed=%s active=%s\n", packageID, strings.Join(versionTexts, ","), active)
	}
	return 0
}

func (a *App) runInstall(ctx context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("install", errObj)
	}
	activate, args, errObj := extractBoolFlag(args, "--activate")
	if errObj != nil {
		return a.fail("install", errObj)
	}
	activationDir, args, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("install", errObj)
	}
	version, rest, errObj := extractStringFlag(args, "--version")
	if errObj != nil {
		return a.fail("install", errObj)
	}
	if len(rest) != 1 {
		return a.fail("install", &CLIError{Code: "INPUT_REQUIRED", Message: "install requires <owner>/<repo>", Details: map[string]any{}})
	}
	packageID := normalizePackageID(rest[0])

	var detail protocol.VersionedPackageDetail
	var err error
	if version == "" {
		detail, err = a.Client.GetLatestPackageDetail(ctx, packageID)
	} else {
		detail, err = a.Client.GetPackageDetailVersion(ctx, packageID, version)
	}
	if err != nil {
		return a.fail("install", asCLIError(err))
	}

	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("install", asCLIError(err))
	}
	changed, planned, installErr := a.installDetail(ctx, &lock, detail, activate, activationDir, dryRun)
	if installErr != nil {
		return a.fail("install", asCLIError(installErr))
	}

	if !dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID, VersionKey: detail.VersionKey})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("install", asCLIError(err))
		}
	} else if dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID, VersionKey: detail.VersionKey})
	}
	return a.writeMutationResult("install", changed, planned, map[string]any{"package_id": packageID, "version_key": detail.VersionKey})
}

func (a *App) runActivate(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("activate", errObj)
	}
	activationDir, args, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("activate", errObj)
	}
	version, rest, errObj := extractStringFlag(args, "--version")
	if errObj != nil {
		return a.fail("activate", errObj)
	}
	if len(rest) != 1 {
		return a.fail("activate", &CLIError{Code: "INPUT_REQUIRED", Message: "activate requires <owner>/<repo>", Details: map[string]any{}})
	}
	if activationDir == "" {
		activationDir = a.Config.DefaultActivationDir
	}
	if activationDir == "" {
		return a.fail("activate", &CLIError{Code: "INPUT_REQUIRED", Message: "activation directory is required", Details: map[string]any{"flag": "--activation-dir"}})
	}
	packageID := normalizePackageID(rest[0])
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("activate", asCLIError(err))
	}
	pkg, ok := lock.Packages[packageID]
	if !ok {
		return a.fail("activate", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": packageID}})
	}
	versionKey, chooseErr := chooseInstalledVersion(pkg, version)
	if chooseErr != nil {
		return a.fail("activate", chooseErr)
	}
	planned, actErr := a.activateVersion(&lock, packageID, versionKey, activationDir, dryRun)
	if actErr != nil {
		return a.fail("activate", asCLIError(actErr))
	}
	if !dryRun {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID, VersionKey: versionKey})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("activate", asCLIError(err))
		}
	} else {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID, VersionKey: versionKey})
	}
	return a.writeMutationResult("activate", true, planned, map[string]any{"package_id": packageID, "version_key": versionKey})
}

func (a *App) runDeactivate(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("deactivate", errObj)
	}
	_, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("deactivate", errObj)
	}
	if len(rest) != 1 {
		return a.fail("deactivate", &CLIError{Code: "INPUT_REQUIRED", Message: "deactivate requires <owner>/<repo>", Details: map[string]any{}})
	}
	packageID := normalizePackageID(rest[0])
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("deactivate", asCLIError(err))
	}
	planned, decErr := a.deactivatePackage(&lock, packageID, dryRun)
	if decErr != nil {
		return a.fail("deactivate", asCLIError(decErr))
	}
	if !dryRun {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("deactivate", asCLIError(err))
		}
	} else {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID})
	}
	return a.writeMutationResult("deactivate", len(planned) > 1, planned, map[string]any{"package_id": packageID})
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
	for _, result := range results {
		fmt.Fprintf(a.Stdout, "%s ok\n", result.PackageID)
	}
	if len(results) == 0 {
		fmt.Fprintln(a.Stdout, "no installed packages")
	}
	return 0
}

func (a *App) runRepair(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("repair", errObj)
	}
	_, args, errObj = extractBoolFlag(args, "--yes")
	if errObj != nil {
		return a.fail("repair", errObj)
	}
	activationDir, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("repair", errObj)
	}
	if activationDir == "" {
		activationDir = a.Config.DefaultActivationDir
	}
	var target string
	if len(rest) > 1 {
		return a.fail("repair", &CLIError{Code: "INPUT_REQUIRED", Message: "repair accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return a.fail("repair", asCLIError(err))
	}
	if !ok {
		lock = protocol.Lockfile{SchemaVersion: "1", GeneratedAt: a.now().UTC().Format(time.RFC3339), Packages: map[string]protocol.LockedPackage{}}
	}
	results := make([]PackageCheckResult, 0)
	planned := make([]PlannedAction, 0)
	changed := false
	for packageID, pkg := range lock.Packages {
		if target != "" && packageID != target {
			continue
		}
		activeVersions := versionKeysWithActiveAssets(pkg)
		chosenActive := ""
		if len(activeVersions) > 0 {
			sorted := SortedInstalledVersionKeys(pkg)
			for _, key := range sorted {
				for _, active := range activeVersions {
					if key == active {
						chosenActive = key
						break
					}
				}
				if chosenActive != "" {
					break
				}
			}
		}
		findings := make([]Finding, 0)
		for versionKey, version := range pkg.InstalledVersions {
			for i := range version.Assets {
				finding := verifyLockedAsset(version.Assets[i])
				activationBroken := finding != nil && finding.Code == "ACTIVATION_BROKEN"
				if finding != nil && (finding.Code == "LOCAL_FILE_MISSING" || finding.Code == "LOCAL_FILE_HASH_MISMATCH") {
					findings = append(findings, *finding)
					continue
				}
				expectedActive := chosenActive != "" && versionKey == chosenActive
				linkPath := ""
				if expectedActive {
					if activationDir == "" {
						findings = append(findings, Finding{Code: "INPUT_REQUIRED", Severity: "error", Subject: "activation", Message: "activation directory is required to repair active assets", Details: map[string]any{"package_id": packageID}})
						continue
					}
					if version.Assets[i].SymlinkPath != nil && *version.Assets[i].SymlinkPath != "" {
						linkPath = *version.Assets[i].SymlinkPath
					} else {
						linkPath = a.resolveSymlinkPath(activationDir, packageID, version.Assets[i])
					}
					if version.Assets[i].SymlinkPath == nil || *version.Assets[i].SymlinkPath != linkPath || !version.Assets[i].Active || activationBroken {
						planned = append(planned, PlannedAction{Type: "create_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
						changed = true
						if !dryRun {
							if err := atomicSymlink(version.Assets[i].LocalPath, linkPath); err != nil {
								return a.fail("repair", &CLIError{Code: "INTERNAL_ERROR", Message: "could not repair symlink", Details: map[string]any{"path": linkPath, "reason": err.Error()}})
							}
						}
					}
					version.Assets[i].Active = true
					version.Assets[i].SymlinkPath = &linkPath
				} else {
					if version.Assets[i].SymlinkPath != nil {
						planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
						changed = true
						if !dryRun {
							if err := removeFileIfExists(*version.Assets[i].SymlinkPath); err != nil {
								return a.fail("repair", &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove stale symlink", Details: map[string]any{"path": *version.Assets[i].SymlinkPath, "reason": err.Error()}})
							}
						}
					}
					version.Assets[i].Active = false
					version.Assets[i].SymlinkPath = nil
				}
			}
			pkg.InstalledVersions[versionKey] = version
		}
		if len(findings) == 0 {
			if chosenActive == "" {
				pkg.ActiveVersionKey = nil
			} else {
				pkg.ActiveVersionKey = &chosenActive
			}
			lock.Packages[packageID] = pkg
		}
		sortFindings(findings)
		results = append(results, PackageCheckResult{PackageID: packageID, OK: len(findings) == 0, Findings: findings})
	}
	if target != "" && len(results) == 0 {
		return a.fail("repair", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
	}
	allOK := true
	repaired := make([]any, 0)
	for _, result := range results {
		allOK = allOK && result.OK
		if result.OK {
			repaired = append(repaired, result.PackageID)
		}
	}
	if !allOK {
		return a.writePackageFailure("repair", "repair failed", results)
	}
	if !dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: target})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("repair", asCLIError(err))
		}
	} else if dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: target})
	}
	data := map[string]any{"changed": changed, "repaired_packages": repaired}
	if dryRun {
		data["planned_actions"] = plannedActionsToAny(planned)
	}
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "repair", Data: data}
		if err := protocol.ValidateRepairResult(env); err != nil {
			return a.fail("repair", &CLIError{Code: "INTERNAL_ERROR", Message: "repair output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	return 0
}

func (a *App) runUninstall(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	_, args, errObj = extractBoolFlag(args, "--yes")
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	allVersions, args, errObj := extractBoolFlag(args, "--all")
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	_, args, errObj = extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	version, rest, errObj := extractStringFlag(args, "--version")
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	if len(rest) != 1 {
		return a.fail("uninstall", &CLIError{Code: "INPUT_REQUIRED", Message: "uninstall requires <owner>/<repo>", Details: map[string]any{}})
	}
	packageID := normalizePackageID(rest[0])
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("uninstall", asCLIError(err))
	}
	pkg, ok := lock.Packages[packageID]
	if !ok {
		return a.fail("uninstall", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": packageID}})
	}
	targetVersions := make([]string, 0)
	switch {
	case allVersions:
		targetVersions = SortedInstalledVersionKeys(pkg)
	case version != "":
		versionKey, chooseErr := chooseInstalledVersion(pkg, version)
		if chooseErr != nil {
			return a.fail("uninstall", chooseErr)
		}
		targetVersions = append(targetVersions, versionKey)
	default:
		keys := SortedInstalledVersionKeys(pkg)
		if len(keys) == 1 {
			targetVersions = append(targetVersions, keys[0])
		} else {
			return a.fail("uninstall", &CLIError{Code: "MULTIPLE_VERSIONS_INSTALLED", Message: "multiple installed versions require --version or --all", Details: map[string]any{"package_id": packageID}})
		}
	}
	planned := make([]PlannedAction, 0)
	changed := false
	for _, versionKey := range targetVersions {
		version := pkg.InstalledVersions[versionKey]
		for _, asset := range version.Assets {
			if asset.SymlinkPath != nil {
				planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: versionKey, Path: asset.Path})
				if !dryRun {
					if err := removeFileIfExists(*asset.SymlinkPath); err != nil {
						return a.fail("uninstall", &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove activation symlink", Details: map[string]any{"path": *asset.SymlinkPath, "reason": err.Error()}})
					}
				}
			}
			planned = append(planned, PlannedAction{Type: "remove_asset", PackageID: packageID, VersionKey: versionKey, Path: asset.Path})
			if !dryRun {
				if err := removeFileIfExists(asset.LocalPath); err != nil {
					return a.fail("uninstall", &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove installed asset", Details: map[string]any{"path": asset.LocalPath, "reason": err.Error()}})
				}
			}
		}
		planned = append(planned, PlannedAction{Type: "remove_lockfile_entry", PackageID: packageID, VersionKey: versionKey})
		delete(pkg.InstalledVersions, versionKey)
		changed = true
	}
	if len(pkg.InstalledVersions) == 0 {
		delete(lock.Packages, packageID)
	} else {
		pkg.ActiveVersionKey = nil
		lock.Packages[packageID] = pkg
	}
	if !dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("uninstall", asCLIError(err))
		}
	} else if dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: packageID})
	}
	return a.writeMutationResult("uninstall", changed, planned, map[string]any{"package_id": packageID})
}

func (a *App) runUpdate(ctx context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("update", errObj)
	}
	activate, args, errObj := extractBoolFlag(args, "--activate")
	if errObj != nil {
		return a.fail("update", errObj)
	}
	activationDir, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("update", errObj)
	}
	var target string
	if len(rest) > 1 {
		return a.fail("update", &CLIError{Code: "INPUT_REQUIRED", Message: "update accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return a.fail("update", asCLIError(err))
	}
	if !ok || len(lock.Packages) == 0 {
		if target != "" {
			return a.fail("update", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
		}
		return a.writeMutationResult("update", false, nil, map[string]any{})
	}
	root, err := a.Client.GetRootIndex(ctx)
	if err != nil {
		return a.fail("update", asCLIError(err))
	}
	planned := make([]PlannedAction, 0)
	changed := false
	packageIDs := make([]string, 0, len(lock.Packages))
	for packageID := range lock.Packages {
		if target != "" && packageID != target {
			continue
		}
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	if target != "" && len(packageIDs) == 0 {
		return a.fail("update", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
	}
	for _, packageID := range packageIDs {
		rootPkg, ok := root.Packages[packageID]
		if !ok {
			continue
		}
		pkg := lock.Packages[packageID]
		installed := SortedInstalledVersionKeys(pkg)
		if len(installed) > 0 && installed[0] == rootPkg.LatestVersionKey {
			continue
		}
		detail, err := a.Client.GetPackageDetailVersion(ctx, packageID, rootPkg.LatestVersionKey)
		if err != nil {
			return a.fail("update", asCLIError(err))
		}
		ch, actions, installErr := a.installDetail(ctx, &lock, detail, activate, activationDir, dryRun)
		if installErr != nil {
			return a.fail("update", asCLIError(installErr))
		}
		planned = append(planned, actions...)
		if ch {
			changed = true
		}
	}
	if !dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: target})
		if err := a.saveLockfile(lock); err != nil {
			return a.fail("update", asCLIError(err))
		}
	} else if dryRun && changed {
		planned = append(planned, PlannedAction{Type: "write_lockfile", PackageID: target})
	}
	return a.writeMutationResult("update", changed, planned, map[string]any{})
}

func (a *App) writeMutationResult(command string, changed bool, planned []PlannedAction, extra map[string]any) int {
	data := map[string]any{"changed": changed}
	for k, v := range extra {
		data[k] = v
	}
	if len(planned) > 0 {
		data["planned_actions"] = plannedActionsToAny(planned)
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: command, Data: data})
	}
	if !changed {
		fmt.Fprintln(a.Stdout, "no changes")
		return 0
	}
	fmt.Fprintf(a.Stdout, "%s changed\n", command)
	return 0
}

func (a *App) writePackageFailure(command, message string, results []PackageCheckResult) int {
	firstCode := "INTERNAL_ERROR"
	for _, result := range results {
		if len(result.Findings) > 0 {
			firstCode = result.Findings[0].Code
			break
		}
	}
	if a.JSON {
		_ = a.writeJSON(protocol.CLIEnvelope{
			SchemaVersion: "1",
			OK:            false,
			Command:       command,
			Error: &protocol.ErrorObject{
				Code:    firstCode,
				Message: message,
				Details: packageResultsToDetails(results),
			},
		})
		return 1
	}
	fmt.Fprintln(a.Stderr, message)
	return 1
}

func (a *App) fail(command string, err *CLIError) int {
	if err == nil {
		err = &CLIError{Code: "INTERNAL_ERROR", Message: "unknown error", Details: map[string]any{}}
	}
	if a.JSON {
		_ = a.writeJSON(protocol.CLIEnvelope{
			SchemaVersion: "1",
			OK:            false,
			Command:       command,
			Error: &protocol.ErrorObject{
				Code:    err.Code,
				Message: err.Message,
				Details: ensureDetails(err.Details),
			},
		})
		return 1
	}
	fmt.Fprintf(a.Stderr, "%s: %s\n", err.Code, err.Message)
	return 1
}

func (a *App) writeJSON(env protocol.CLIEnvelope) int {
	body, err := protocol.MarshalCanonical(env)
	if err != nil {
		fmt.Fprintf(a.Stderr, "INTERNAL_ERROR: %s\n", err.Error())
		return 1
	}
	_, _ = a.Stdout.Write(append(body, '\n'))
	return 0
}

func asCLIError(err error) *CLIError {
	var cliErr *CLIError
	if err != nil && errorAs(err, &cliErr) {
		return cliErr
	}
	return &CLIError{Code: "INTERNAL_ERROR", Message: err.Error(), Details: map[string]any{}}
}

func structToMap(value any) (map[string]any, *CLIError) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize output", Details: map[string]any{}}
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not build JSON output", Details: map[string]any{}}
	}
	return out, nil
}

func normalizePackageID(packageID string) string {
	return strings.ToLower(packageID)
}

func commandPath(args []string) []string {
	if len(args) >= 2 && args[0] == "package" {
		return args[:2]
	}
	if len(args) > 0 {
		return args[:1]
	}
	return nil
}

func ensureDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}

func extractStringFlag(args []string, name string) (string, []string, *CLIError) {
	rest := make([]string, 0, len(args))
	var value string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == name:
			if i+1 >= len(args) {
				return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "missing flag value", Details: map[string]any{"flag": name}}
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(arg, name+"="):
			value = strings.TrimPrefix(arg, name+"=")
		case strings.HasPrefix(arg, "--"):
			return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "unknown flag", Details: map[string]any{"flag": arg}}
		default:
			rest = append(rest, arg)
		}
	}
	return value, rest, nil
}

func extractBoolFlag(args []string, name string) (bool, []string, *CLIError) {
	rest := make([]string, 0, len(args))
	value := false
	for _, arg := range args {
		switch arg {
		case name:
			value = true
		default:
			rest = append(rest, arg)
		}
	}
	return value, rest, nil
}

func plannedActionsToAny(actions []PlannedAction) []any {
	out := make([]any, 0, len(actions))
	for _, action := range actions {
		item := map[string]any{
			"type":       action.Type,
			"package_id": action.PackageID,
		}
		if action.VersionKey != "" {
			item["version_key"] = action.VersionKey
		}
		if action.Path != "" {
			item["path"] = action.Path
		}
		out = append(out, item)
	}
	return out
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func errorAs(err error, target **CLIError) bool {
	typed, ok := err.(*CLIError)
	if !ok {
		return false
	}
	*target = typed
	return true
}

func Main() {
	os.Exit(Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
