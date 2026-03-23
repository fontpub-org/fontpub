package cli

import (
	"os"
	"path/filepath"
	"sort"
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
	if err := ensurePackageActivationDirMatch(packageID, pkg, activationDir); err != nil {
		return nil, err
	}
	planned := make([]PlannedAction, 0)

	for otherVersionKey, other := range pkg.InstalledVersions {
		if otherVersionKey == versionKey {
			continue
		}
		for i := range other.Assets {
			if !assetSymlinkPathMatchesActivationDir(other.Assets[i], activationDir) {
				continue
			}
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

func ensurePackageActivationDirMatch(packageID string, pkg protocol.LockedPackage, activationDir string) error {
	dirs := activeAssetActivationDirs(pkg)
	if len(dirs) == 0 {
		return nil
	}
	requested := filepath.Clean(activationDir)
	for _, dir := range dirs {
		if dir != requested {
			return &CLIError{
				Code:    "INPUT_REQUIRED",
				Message: "package is active in a different activation directory",
				Details: map[string]any{
					"package_id":               packageID,
					"current_activation_dir":   strings.Join(dirs, ", "),
					"requested_activation_dir": requested,
				},
			}
		}
	}
	return nil
}

func activeAssetActivationDirs(pkg protocol.LockedPackage) []string {
	seen := map[string]struct{}{}
	dirs := make([]string, 0)
	for _, version := range pkg.InstalledVersions {
		for _, asset := range version.Assets {
			if !asset.Active || asset.SymlinkPath == nil || *asset.SymlinkPath == "" {
				continue
			}
			dir := filepath.Clean(filepath.Dir(*asset.SymlinkPath))
			if _, ok := seen[dir]; ok {
				continue
			}
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func (a *App) deactivatePackage(lock *protocol.Lockfile, packageID, activationDir string, dryRun bool) ([]PlannedAction, bool, error) {
	pkg, ok := lock.Packages[packageID]
	if !ok {
		return nil, false, &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": packageID}}
	}
	planned := make([]PlannedAction, 0)
	changed := false
	for versionKey, version := range pkg.InstalledVersions {
		for i := range version.Assets {
			if activationDir != "" && !assetSymlinkPathMatchesActivationDir(version.Assets[i], activationDir) {
				continue
			}
			if version.Assets[i].Active || version.Assets[i].SymlinkPath != nil {
				changed = true
			}
			if version.Assets[i].SymlinkPath != nil {
				planned = append(planned, PlannedAction{Type: "remove_symlink", PackageID: packageID, VersionKey: versionKey, Path: version.Assets[i].Path})
				if !dryRun {
					if err := removeFileIfExists(*version.Assets[i].SymlinkPath); err != nil {
						return nil, false, &CLIError{Code: "INTERNAL_ERROR", Message: "could not remove activation symlink", Details: map[string]any{"path": *version.Assets[i].SymlinkPath, "reason": err.Error()}}
					}
				}
			}
			version.Assets[i].Active = false
			version.Assets[i].SymlinkPath = nil
		}
		pkg.InstalledVersions[versionKey] = version
	}
	if active := chooseRepairActiveVersion(pkg); active == "" {
		pkg.ActiveVersionKey = nil
	} else {
		pkg.ActiveVersionKey = &active
	}
	lock.Packages[packageID] = pkg
	return planned, changed, nil
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
