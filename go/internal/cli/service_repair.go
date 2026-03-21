package cli

import "github.com/fontpub-org/fontpub/go/internal/protocol"

type repairOperation struct {
	Action    PlannedAction
	LinkPath  string
	LocalPath string
}

type repairPackagePlan struct {
	PackageID      string
	UpdatedPackage protocol.LockedPackage
	Result         PackageCheckResult
	Operations     []repairOperation
	Changed        bool
}

func chooseRepairActiveVersion(pkg protocol.LockedPackage) string {
	activeVersions := versionKeysWithActiveAssets(pkg)
	if len(activeVersions) == 0 {
		return ""
	}
	sorted := SortedInstalledVersionKeys(pkg)
	for _, key := range sorted {
		for _, active := range activeVersions {
			if key == active {
				return key
			}
		}
	}
	return ""
}

func (a *App) inspectRepairPackage(packageID string, pkg protocol.LockedPackage, activationDir string) repairPackagePlan {
	plan := repairPackagePlan{
		PackageID:      packageID,
		UpdatedPackage: pkg,
		Operations:     make([]repairOperation, 0),
	}
	chosenActive := chooseRepairActiveVersion(pkg)
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
					plan.Operations = append(plan.Operations, repairOperation{
						Action:    PlannedAction{Type: "create_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path},
						LinkPath:  linkPath,
						LocalPath: version.Assets[i].LocalPath,
					})
					plan.Changed = true
				}
				version.Assets[i].Active = true
				version.Assets[i].SymlinkPath = &linkPath
			} else {
				if version.Assets[i].SymlinkPath != nil {
					plan.Operations = append(plan.Operations, repairOperation{
						Action:   PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path},
						LinkPath: *version.Assets[i].SymlinkPath,
					})
					plan.Changed = true
				}
				version.Assets[i].Active = false
				version.Assets[i].SymlinkPath = nil
			}
		}
		plan.UpdatedPackage.InstalledVersions[versionKey] = version
	}
	if len(findings) == 0 {
		if chosenActive == "" {
			plan.UpdatedPackage.ActiveVersionKey = nil
		} else {
			plan.UpdatedPackage.ActiveVersionKey = &chosenActive
		}
	}
	sortFindings(findings)
	plan.Result = PackageCheckResult{PackageID: packageID, OK: len(findings) == 0, Findings: findings}
	return plan
}

func (a *App) applyRepairPlan(lock *protocol.Lockfile, plan repairPackagePlan, dryRun bool) ([]PlannedAction, error) {
	planned := make([]PlannedAction, 0, len(plan.Operations))
	if !plan.Result.OK {
		return planned, nil
	}
	for _, op := range plan.Operations {
		planned = append(planned, op.Action)
		if dryRun {
			continue
		}
		switch op.Action.Type {
		case "create_symlink":
			if err := atomicSymlink(op.LocalPath, op.LinkPath); err != nil {
				return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not repair symlink", Details: map[string]any{"path": op.LinkPath, "reason": err.Error()}}
			}
		case "remove_symlink":
			if err := removeFileIfExists(op.LinkPath); err != nil {
				return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove stale symlink", Details: map[string]any{"path": op.LinkPath, "reason": err.Error()}}
			}
		}
	}
	lock.Packages[plan.PackageID] = plan.UpdatedPackage
	return planned, nil
}
