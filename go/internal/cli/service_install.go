package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) resolvePackageDetail(ctx context.Context, packageID, version string) (protocol.VersionedPackageDetail, error) {
	if version == "" {
		return a.Client.GetLatestPackageDetail(ctx, packageID)
	}
	return a.Client.GetPackageDetailVersion(ctx, packageID, version)
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
			installCopy := cloneLockfile(*lock)
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
