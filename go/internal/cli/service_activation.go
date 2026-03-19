package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

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
