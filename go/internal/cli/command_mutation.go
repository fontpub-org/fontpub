package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

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
	return a.writeInstallResult(packageID, detail.VersionKey, activate, activationDir, changed, dryRun, planned)
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
	return a.writeActivateResult(packageID, versionKey, activationDir, dryRun, planned)
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
	return a.writeDeactivateResult(packageID, len(planned) > 1, dryRun, planned)
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
	if len(results) == 0 {
		fmt.Fprintln(a.Stdout, "no installed packages")
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
	return a.writeUninstallResult(packageID, targetVersions, changed, dryRun, planned)
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
		return a.writeUpdateResult(target, false, activate, dryRun, activationDir, nil)
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
	return a.writeUpdateResult(target, changed, activate, dryRun, activationDir, planned)
}
