package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) runInstall(ctx context.Context, args []string) int {
	opts, errObj := parseInstallOptions(args)
	if errObj != nil {
		return a.fail("install", errObj)
	}
	detail, err := a.resolvePackageDetail(ctx, opts.PackageID, opts.Version)
	if err != nil {
		return a.fail("install", asCLIError(err))
	}

	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("install", asCLIError(err))
	}
	changed, planned, installErr := a.installDetail(ctx, &lock, detail, opts.Activate, opts.ActivationDir, opts.DryRun)
	if installErr != nil {
		return a.fail("install", asCLIError(installErr))
	}
	planned, err = a.finalizeLockMutation(lock, changed, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID, VersionKey: detail.VersionKey})
	if err != nil {
		return a.fail("install", asCLIError(err))
	}
	return a.writeInstallResult(opts.PackageID, detail.VersionKey, opts.Activate, opts.ActivationDir, changed, opts.DryRun, planned)
}

func (a *App) runActivate(_ context.Context, args []string) int {
	opts, errObj := parseActivateOptions(args)
	if errObj != nil {
		return a.fail("activate", errObj)
	}
	if opts.ActivationDir == "" {
		opts.ActivationDir = a.Config.DefaultActivationDir
	}
	if opts.ActivationDir == "" {
		return a.fail("activate", &CLIError{Code: "INPUT_REQUIRED", Message: "activation directory is required", Details: map[string]any{"flag": "--activation-dir"}})
	}
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("activate", asCLIError(err))
	}
	pkg, ok := lock.Packages[opts.PackageID]
	if !ok {
		return a.fail("activate", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": opts.PackageID}})
	}
	versionKey, chooseErr := chooseInstalledVersion(pkg, opts.Version)
	if chooseErr != nil {
		return a.fail("activate", chooseErr)
	}
	planned, actErr := a.activateVersion(&lock, opts.PackageID, versionKey, opts.ActivationDir, opts.DryRun)
	if actErr != nil {
		return a.fail("activate", asCLIError(actErr))
	}
	planned, err = a.finalizeLockMutation(lock, true, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID, VersionKey: versionKey})
	if err != nil {
		return a.fail("activate", asCLIError(err))
	}
	return a.writeActivateResult(opts.PackageID, versionKey, opts.ActivationDir, opts.DryRun, planned)
}

func (a *App) runDeactivate(_ context.Context, args []string) int {
	opts, errObj := parseDeactivateOptions(args)
	if errObj != nil {
		return a.fail("deactivate", errObj)
	}
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("deactivate", asCLIError(err))
	}
	planned, changed, decErr := a.deactivatePackage(&lock, opts.PackageID, opts.DryRun)
	if decErr != nil {
		return a.fail("deactivate", asCLIError(decErr))
	}
	planned, err = a.finalizeLockMutation(lock, changed, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID})
	if err != nil {
		return a.fail("deactivate", asCLIError(err))
	}
	return a.writeDeactivateResult(opts.PackageID, changed, opts.DryRun, planned)
}

func (a *App) runRepair(_ context.Context, args []string) int {
	opts, errObj := parseRepairOptions(args)
	if errObj != nil {
		return a.fail("repair", errObj)
	}
	if opts.ActivationDir == "" {
		opts.ActivationDir = a.Config.DefaultActivationDir
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
		if opts.PackageID != "" && packageID != opts.PackageID {
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
					if opts.ActivationDir == "" {
						findings = append(findings, Finding{Code: "INPUT_REQUIRED", Severity: "error", Subject: "activation", Message: "activation directory is required to repair active assets", Details: map[string]any{"package_id": packageID}})
						continue
					}
					if version.Assets[i].SymlinkPath != nil && *version.Assets[i].SymlinkPath != "" {
						linkPath = *version.Assets[i].SymlinkPath
					} else {
						linkPath = a.resolveSymlinkPath(opts.ActivationDir, packageID, version.Assets[i])
					}
					if version.Assets[i].SymlinkPath == nil || *version.Assets[i].SymlinkPath != linkPath || !version.Assets[i].Active || activationBroken {
						planned = append(planned, PlannedAction{Type: "create_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
						changed = true
						if !opts.DryRun {
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
						if !opts.DryRun {
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
	if opts.PackageID != "" && len(results) == 0 {
		return a.fail("repair", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": opts.PackageID}})
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
	planned, err = a.finalizeLockMutation(lock, changed, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID})
	if err != nil {
		return a.fail("repair", asCLIError(err))
	}
	data := map[string]any{"changed": changed, "repaired_packages": repaired}
	if opts.DryRun {
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
	if opts.DryRun && len(planned) > 0 {
		printPlannedActions(a.Stdout, planned)
	}
	return 0
}

func (a *App) runUninstall(_ context.Context, args []string) int {
	opts, errObj := parseUninstallOptions(args)
	if errObj != nil {
		return a.fail("uninstall", errObj)
	}
	lock, err := a.loadOrInitLockfile()
	if err != nil {
		return a.fail("uninstall", asCLIError(err))
	}
	pkg, ok := lock.Packages[opts.PackageID]
	if !ok {
		return a.fail("uninstall", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": opts.PackageID}})
	}
	targetVersions := make([]string, 0)
	switch {
	case opts.All:
		targetVersions = SortedInstalledVersionKeys(pkg)
	case opts.Version != "":
		versionKey, chooseErr := chooseInstalledVersion(pkg, opts.Version)
		if chooseErr != nil {
			return a.fail("uninstall", chooseErr)
		}
		targetVersions = append(targetVersions, versionKey)
	default:
		keys := SortedInstalledVersionKeys(pkg)
		if len(keys) == 1 {
			targetVersions = append(targetVersions, keys[0])
		} else {
			return a.fail("uninstall", &CLIError{Code: "MULTIPLE_VERSIONS_INSTALLED", Message: "multiple installed versions require --version or --all", Details: map[string]any{"package_id": opts.PackageID}})
		}
	}
	planned := make([]PlannedAction, 0)
	changed := false
	for _, versionKey := range targetVersions {
		version := pkg.InstalledVersions[versionKey]
		for _, asset := range version.Assets {
			if asset.SymlinkPath != nil {
				planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: opts.PackageID, VersionKey: versionKey, Path: asset.Path})
				if !opts.DryRun {
					if err := removeFileIfExists(*asset.SymlinkPath); err != nil {
						return a.fail("uninstall", &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove activation symlink", Details: map[string]any{"path": *asset.SymlinkPath, "reason": err.Error()}})
					}
				}
			}
			planned = append(planned, PlannedAction{Type: "remove_asset", PackageID: opts.PackageID, VersionKey: versionKey, Path: asset.Path})
			if !opts.DryRun {
				if err := removeFileIfExists(asset.LocalPath); err != nil {
					return a.fail("uninstall", &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove installed asset", Details: map[string]any{"path": asset.LocalPath, "reason": err.Error()}})
				}
			}
		}
		planned = append(planned, PlannedAction{Type: "remove_lockfile_entry", PackageID: opts.PackageID, VersionKey: versionKey})
		delete(pkg.InstalledVersions, versionKey)
		changed = true
	}
	if len(pkg.InstalledVersions) == 0 {
		delete(lock.Packages, opts.PackageID)
	} else {
		pkg.ActiveVersionKey = nil
		lock.Packages[opts.PackageID] = pkg
	}
	planned, err = a.finalizeLockMutation(lock, changed, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID})
	if err != nil {
		return a.fail("uninstall", asCLIError(err))
	}
	return a.writeUninstallResult(opts.PackageID, targetVersions, changed, opts.DryRun, planned)
}

func (a *App) runUpdate(ctx context.Context, args []string) int {
	opts, errObj := parseUpdateOptions(args)
	if errObj != nil {
		return a.fail("update", errObj)
	}
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return a.fail("update", asCLIError(err))
	}
	if !ok || len(lock.Packages) == 0 {
		if opts.PackageID != "" {
			return a.fail("update", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": opts.PackageID}})
		}
		if !a.JSON {
			printNoInstalledPackages(a.Stdout)
			return 0
		}
		return a.writeUpdateResult(opts.PackageID, false, opts.Activate, opts.DryRun, opts.ActivationDir, nil)
	}
	root, err := a.Client.GetRootIndex(ctx)
	if err != nil {
		return a.fail("update", asCLIError(err))
	}
	planned := make([]PlannedAction, 0)
	changed := false
	packageIDs := make([]string, 0, len(lock.Packages))
	for packageID := range lock.Packages {
		if opts.PackageID != "" && packageID != opts.PackageID {
			continue
		}
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	if opts.PackageID != "" && len(packageIDs) == 0 {
		return a.fail("update", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": opts.PackageID}})
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
		ch, actions, installErr := a.installDetail(ctx, &lock, detail, opts.Activate, opts.ActivationDir, opts.DryRun)
		if installErr != nil {
			return a.fail("update", asCLIError(installErr))
		}
		planned = append(planned, actions...)
		if ch {
			changed = true
		}
	}
	planned, err = a.finalizeLockMutation(lock, changed, opts.DryRun, planned, PlannedAction{Type: "write_lockfile", PackageID: opts.PackageID})
	if err != nil {
		return a.fail("update", asCLIError(err))
	}
	return a.writeUpdateResult(opts.PackageID, changed, opts.Activate, opts.DryRun, opts.ActivationDir, planned)
}
