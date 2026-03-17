package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type Finding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Subject  string         `json:"subject"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details"`
}

type PackageCheckResult struct {
	PackageID string    `json:"package_id"`
	OK        bool      `json:"ok"`
	Findings  []Finding `json:"findings"`
}

type PlannedAction struct {
	Type       string `json:"type"`
	PackageID  string `json:"package_id"`
	VersionKey string `json:"version_key,omitempty"`
	Path       string `json:"path,omitempty"`
}

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

func (a *App) installDetail(ctx context.Context, lock *protocol.Lockfile, detail protocol.VersionedPackageDetail, activate bool, activationDir string, dryRun bool) (bool, []PlannedAction, error) {
	packageID := detail.PackageID
	pkg := lock.Packages[packageID]
	if pkg.InstalledVersions == nil {
		pkg.InstalledVersions = map[string]protocol.InstalledVersion{}
	}
	planned := make([]PlannedAction, 0)
	changed := false
	if _, exists := pkg.InstalledVersions[detail.VersionKey]; !exists {
		installed := protocol.InstalledVersion{
			Version:     detail.Version,
			VersionKey:  detail.VersionKey,
			InstalledAt: a.now().UTC().Format(time.RFC3339),
			Assets:      make([]protocol.LockedAsset, 0, len(detail.Assets)),
		}
		for _, asset := range detail.Assets {
			planned = append(planned,
				PlannedAction{Type: "download_asset", PackageID: packageID, VersionKey: detail.VersionKey, Path: asset.Path},
				PlannedAction{Type: "write_asset", PackageID: packageID, VersionKey: detail.VersionKey, Path: asset.Path},
			)
			if !dryRun {
				lockedAsset, err := a.downloadAndWriteAsset(ctx, packageID, detail.VersionKey, asset)
				if err != nil {
					return false, nil, err
				}
				installed.Assets = append(installed.Assets, lockedAsset)
			}
		}
		if !dryRun {
			pkg.InstalledVersions[detail.VersionKey] = installed
			lock.Packages[packageID] = pkg
		}
		changed = true
	}
	if activate {
		if activationDir == "" {
			activationDir = a.Config.DefaultActivationDir
		}
		if activationDir == "" {
			return false, nil, &CLIError{Code: "INPUT_REQUIRED", Message: "activation directory is required for --activate", Details: map[string]any{"flag": "--activation-dir"}}
		}
		if dryRun {
			installCopy := *lock
			if installCopy.Packages == nil {
				installCopy.Packages = map[string]protocol.LockedPackage{}
			}
			pkgCopy := installCopy.Packages[packageID]
			if pkgCopy.InstalledVersions == nil {
				pkgCopy.InstalledVersions = map[string]protocol.InstalledVersion{}
			}
			if _, exists := pkgCopy.InstalledVersions[detail.VersionKey]; !exists {
				pkgCopy.InstalledVersions[detail.VersionKey] = protocol.InstalledVersion{
					Version:    detail.Version,
					VersionKey: detail.VersionKey,
					Assets: func() []protocol.LockedAsset {
						out := make([]protocol.LockedAsset, 0, len(detail.Assets))
						for _, asset := range detail.Assets {
							out = append(out, protocol.LockedAsset{
								Path:      asset.Path,
								SHA256:    asset.SHA256,
								LocalPath: assetInstallPath(a.Config.StateDir, packageID, detail.VersionKey, asset.Path),
							})
						}
						return out
					}(),
				}
			}
			installCopy.Packages[packageID] = pkgCopy
			activatePlan, err := a.activateVersion(&installCopy, packageID, detail.VersionKey, activationDir, true)
			if err != nil {
				return false, nil, err
			}
			planned = append(planned, activatePlan...)
			changed = true
		} else {
			activatePlan, err := a.activateVersion(lock, packageID, detail.VersionKey, activationDir, false)
			if err != nil {
				return false, nil, err
			}
			planned = append(planned, activatePlan...)
			changed = true
		}
	}
	return changed, planned, nil
}

func (a *App) saveLockfile(lock protocol.Lockfile) error {
	lock.SchemaVersion = "1"
	lock.GeneratedAt = a.now().UTC().Format(time.RFC3339)
	if lock.Packages == nil {
		lock.Packages = map[string]protocol.LockedPackage{}
	}
	return a.lockfileStore().Save(lock)
}

func (a *App) downloadAndWriteAsset(ctx context.Context, packageID, versionKey string, asset protocol.VersionedAsset) (protocol.LockedAsset, error) {
	body, err := a.Client.Download(ctx, asset.URL)
	if err != nil {
		return protocol.LockedAsset{}, err
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != asset.SHA256 {
		return protocol.LockedAsset{}, &CLIError{
			Code:    "LOCAL_FILE_HASH_MISMATCH",
			Message: "downloaded asset hash does not match published metadata",
			Details: map[string]any{"path": asset.Path, "package_id": packageID, "version_key": versionKey},
		}
	}
	localPath := assetInstallPath(a.Config.StateDir, packageID, versionKey, asset.Path)
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return protocol.LockedAsset{}, &CLIError{Code: "INTERNAL_ERROR", Message: "could not create install directory", Details: map[string]any{"path": localPath, "reason": err.Error()}}
	}
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		return protocol.LockedAsset{}, &CLIError{Code: "INTERNAL_ERROR", Message: "could not write downloaded asset", Details: map[string]any{"path": localPath, "reason": err.Error()}}
	}
	return protocol.LockedAsset{
		Path:      asset.Path,
		SHA256:    asset.SHA256,
		LocalPath: localPath,
		Active:    false,
	}, nil
}

func (a *App) activateVersion(lock *protocol.Lockfile, packageID, versionKey, activationDir string, dryRun bool) ([]PlannedAction, error) {
	if err := ensureActivationDir(activationDir); err != nil && !dryRun {
		return nil, err
	}
	pkg, ok := lock.Packages[packageID]
	if !ok {
		return nil, &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": packageID}}
	}
	version, ok := pkg.InstalledVersions[versionKey]
	if !ok {
		return nil, &CLIError{Code: "NOT_INSTALLED", Message: "requested version is not installed", Details: map[string]any{"package_id": packageID, "version_key": versionKey}}
	}
	planned := make([]PlannedAction, 0)

	for otherVersionKey, other := range pkg.InstalledVersions {
		if otherVersionKey == versionKey {
			continue
		}
		for i := range other.Assets {
			if other.Assets[i].SymlinkPath != nil {
				planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: otherVersionKey, Path: other.Assets[i].Path})
				if !dryRun {
					if err := removeFileIfExists(*other.Assets[i].SymlinkPath); err != nil {
						return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove activation symlink", Details: map[string]any{"path": *other.Assets[i].SymlinkPath, "reason": err.Error()}}
					}
				}
			}
			other.Assets[i].Active = false
			other.Assets[i].SymlinkPath = nil
		}
		pkg.InstalledVersions[otherVersionKey] = other
	}

	for i := range version.Assets {
		linkPath := a.resolveSymlinkPath(activationDir, packageID, version.Assets[i])
		planned = append(planned, PlannedAction{Type: "create_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
		if !dryRun {
			if err := atomicSymlink(version.Assets[i].LocalPath, linkPath); err != nil {
				return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not create activation symlink", Details: map[string]any{"path": linkPath, "reason": err.Error()}}
			}
		}
		version.Assets[i].Active = true
		version.Assets[i].SymlinkPath = &linkPath
	}
	pkg.InstalledVersions[versionKey] = version
	pkg.ActiveVersionKey = &versionKey
	lock.Packages[packageID] = pkg
	return planned, nil
}

func (a *App) deactivatePackage(lock *protocol.Lockfile, packageID string, dryRun bool) ([]PlannedAction, error) {
	pkg, ok := lock.Packages[packageID]
	if !ok {
		return nil, &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": packageID}}
	}
	planned := make([]PlannedAction, 0)
	for versionKey, version := range pkg.InstalledVersions {
		for i := range version.Assets {
			if version.Assets[i].SymlinkPath != nil {
				planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
				if !dryRun {
					if err := removeFileIfExists(*version.Assets[i].SymlinkPath); err != nil {
						return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove activation symlink", Details: map[string]any{"path": *version.Assets[i].SymlinkPath, "reason": err.Error()}}
					}
				}
			}
			version.Assets[i].Active = false
			version.Assets[i].SymlinkPath = nil
		}
		pkg.InstalledVersions[versionKey] = version
	}
	pkg.ActiveVersionKey = nil
	lock.Packages[packageID] = pkg
	return planned, nil
}

func (a *App) resolveSymlinkPath(activationDir, packageID string, asset protocol.LockedAsset) string {
	filename := fileBaseName(asset.Path)
	base := strings.SplitN(normalizePackageID(packageID), "/", 2)
	name := strings.Join([]string{base[0], base[1], filename}, "--")
	path := filepath.Join(activationDir, name)
	info, err := os.Lstat(path)
	if err == nil && info != nil {
		if target, err := os.Readlink(path); err == nil {
			resolved := target
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(path), resolved)
			}
			if filepath.Clean(resolved) == filepath.Clean(asset.LocalPath) {
				return path
			}
		}
		name = symlinkName(packageID, filename, asset.SHA256)
		path = filepath.Join(activationDir, name)
	}
	return path
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
