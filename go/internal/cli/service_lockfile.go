package cli

import (
	"sort"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) now() time.Time {
	if a.Now == nil {
		return time.Now().UTC()
	}
	return a.Now()
}

func (a *App) lockfileStore() LockfileStore {
	return LockfileStore{Path: a.Config.LockfilePath()}
}

func (a *App) loadOrInitLockfile() (protocol.Lockfile, error) {
	lock, ok, err := a.lockfileStore().Load()
	if err != nil {
		return protocol.Lockfile{}, err
	}
	if ok {
		return lock, nil
	}
	return protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   a.now().Format(time.RFC3339),
		Packages:      map[string]protocol.LockedPackage{},
	}, nil
}

func cloneLockfile(lock protocol.Lockfile) protocol.Lockfile {
	out := protocol.Lockfile{
		SchemaVersion: lock.SchemaVersion,
		GeneratedAt:   lock.GeneratedAt,
		Packages:      map[string]protocol.LockedPackage{},
	}
	for packageID, pkg := range lock.Packages {
		clonedPkg := protocol.LockedPackage{
			InstalledVersions: map[string]protocol.InstalledVersion{},
		}
		if pkg.ActiveVersionKey != nil {
			active := *pkg.ActiveVersionKey
			clonedPkg.ActiveVersionKey = &active
		}
		for versionKey, version := range pkg.InstalledVersions {
			clonedVersion := protocol.InstalledVersion{
				Version:     version.Version,
				VersionKey:  version.VersionKey,
				InstalledAt: version.InstalledAt,
				Assets:      make([]protocol.LockedAsset, 0, len(version.Assets)),
			}
			for _, asset := range version.Assets {
				clonedAsset := protocol.LockedAsset{
					Path:      asset.Path,
					SHA256:    asset.SHA256,
					LocalPath: asset.LocalPath,
					Active:    asset.Active,
				}
				if asset.SymlinkPath != nil {
					symlinkPath := *asset.SymlinkPath
					clonedAsset.SymlinkPath = &symlinkPath
				}
				clonedVersion.Assets = append(clonedVersion.Assets, clonedAsset)
			}
			clonedPkg.InstalledVersions[versionKey] = clonedVersion
		}
		out.Packages[packageID] = clonedPkg
	}
	return out
}

func (a *App) finalizeLockMutation(lock protocol.Lockfile, changed, dryRun bool, planned []PlannedAction, writeAction PlannedAction) ([]PlannedAction, error) {
	if !changed {
		return planned, nil
	}
	planned = append(planned, writeAction)
	if dryRun {
		return planned, nil
	}
	if err := a.saveLockfile(lock); err != nil {
		return nil, err
	}
	return planned, nil
}

func packageResultsToDetails(results []PackageCheckResult) map[string]any {
	items := make([]any, 0, len(results))
	for _, result := range results {
		findings := make([]any, 0, len(result.Findings))
		for _, finding := range result.Findings {
			findings = append(findings, map[string]any{
				"code":     finding.Code,
				"severity": finding.Severity,
				"subject":  finding.Subject,
				"message":  finding.Message,
				"details":  finding.Details,
			})
		}
		items = append(items, map[string]any{
			"package_id": result.PackageID,
			"ok":         result.OK,
			"findings":   findings,
		})
	}
	return map[string]any{"packages": items}
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Code == findings[j].Code {
			return findings[i].Message < findings[j].Message
		}
		return findings[i].Code < findings[j].Code
	})
}
