package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func packageInstallRoot(stateDir, packageID, versionKey string) string {
	parts := strings.SplitN(normalizePackageID(packageID), "/", 2)
	if len(parts) != 2 {
		return filepath.Join(stateDir, "packages", normalizePackageID(packageID), versionKey)
	}
	return filepath.Join(stateDir, "packages", parts[0], parts[1], versionKey)
}

func assetInstallPath(stateDir, packageID, versionKey, assetPath string) string {
	root := packageInstallRoot(stateDir, packageID, versionKey)
	return filepath.Join(root, filepath.FromSlash(assetPath))
}

func symlinkName(packageID, filename, sha256hex string) string {
	parts := strings.SplitN(normalizePackageID(packageID), "/", 2)
	base := strings.Join([]string{parts[0], parts[1], filename}, "--")
	if len(sha256hex) >= 8 {
		return base + "--" + sha256hex[:8]
	}
	return base
}

func ensureActivationDir(dir string) *CLIError {
	if dir == "" {
		return &CLIError{Code: "INPUT_REQUIRED", Message: "activation directory is required", Details: map[string]any{"flag": "--activation-dir"}}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "could not create activation directory", Details: map[string]any{"path": dir, "reason": err.Error()}}
	}
	return nil
}

func computeFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyLockedAsset(asset protocol.LockedAsset) *Finding {
	info, err := os.Lstat(asset.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Finding{
				Code:     "LOCAL_FILE_MISSING",
				Severity: "error",
				Subject:  "asset",
				Message:  "installed asset file is missing",
				Details:  map[string]any{"path": asset.Path, "local_path": asset.LocalPath},
			}
		}
		return &Finding{
			Code:     "INTERNAL_ERROR",
			Severity: "error",
			Subject:  "asset",
			Message:  "could not stat installed asset file",
			Details:  map[string]any{"path": asset.Path, "local_path": asset.LocalPath, "reason": err.Error()},
		}
	}
	if info.IsDir() {
		return &Finding{
			Code:     "LOCAL_FILE_MISSING",
			Severity: "error",
			Subject:  "asset",
			Message:  "installed asset path is not a file",
			Details:  map[string]any{"path": asset.Path, "local_path": asset.LocalPath},
		}
	}
	sum, err := computeFileSHA256(asset.LocalPath)
	if err != nil {
		return &Finding{
			Code:     "INTERNAL_ERROR",
			Severity: "error",
			Subject:  "asset",
			Message:  "could not hash installed asset file",
			Details:  map[string]any{"path": asset.Path, "local_path": asset.LocalPath, "reason": err.Error()},
		}
	}
	if sum != asset.SHA256 {
		return &Finding{
			Code:     "LOCAL_FILE_HASH_MISMATCH",
			Severity: "error",
			Subject:  "asset",
			Message:  "installed asset file hash does not match lockfile",
			Details:  map[string]any{"path": asset.Path, "local_path": asset.LocalPath},
		}
	}
	if asset.Active {
		if asset.SymlinkPath == nil || *asset.SymlinkPath == "" {
			return &Finding{
				Code:     "ACTIVATION_BROKEN",
				Severity: "error",
				Subject:  "activation",
				Message:  "active asset is missing symlink_path",
				Details:  map[string]any{"path": asset.Path},
			}
		}
		target, err := os.Readlink(*asset.SymlinkPath)
		if err != nil {
			return &Finding{
				Code:     "ACTIVATION_BROKEN",
				Severity: "error",
				Subject:  "activation",
				Message:  "activation symlink is missing or unreadable",
				Details:  map[string]any{"path": asset.Path, "symlink_path": *asset.SymlinkPath},
			}
		}
		resolved := target
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(*asset.SymlinkPath), resolved)
		}
		if filepath.Clean(resolved) != filepath.Clean(asset.LocalPath) {
			return &Finding{
				Code:     "ACTIVATION_BROKEN",
				Severity: "error",
				Subject:  "activation",
				Message:  "activation symlink points to the wrong file",
				Details:  map[string]any{"path": asset.Path, "symlink_path": *asset.SymlinkPath},
			}
		}
	}
	return nil
}

func activationLinkMatches(asset protocol.LockedAsset, activationDir string) bool {
	if !asset.Active || activationDir == "" || asset.SymlinkPath == nil || *asset.SymlinkPath == "" {
		return false
	}
	if filepath.Clean(filepath.Dir(*asset.SymlinkPath)) != filepath.Clean(activationDir) {
		return false
	}
	target, err := os.Readlink(*asset.SymlinkPath)
	if err != nil {
		return false
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(*asset.SymlinkPath), resolved)
	}
	return filepath.Clean(resolved) == filepath.Clean(asset.LocalPath)
}

func assetSymlinkPathMatchesActivationDir(asset protocol.LockedAsset, activationDir string) bool {
	if activationDir == "" {
		return asset.SymlinkPath != nil && *asset.SymlinkPath != ""
	}
	if asset.SymlinkPath == nil || *asset.SymlinkPath == "" {
		return false
	}
	return filepath.Clean(filepath.Dir(*asset.SymlinkPath)) == filepath.Clean(activationDir)
}

func chooseInstalledVersion(pkg protocol.LockedPackage, requested string) (string, *CLIError) {
	if requested != "" {
		versionKey, err := protocol.NormalizeVersionKey(requested)
		if err != nil {
			return "", &CLIError{Code: "VERSION_INVALID", Message: "invalid version", Details: map[string]any{"version": requested}}
		}
		if _, ok := pkg.InstalledVersions[versionKey]; ok {
			return versionKey, nil
		}
		for installedKey := range pkg.InstalledVersions {
			cmp, err := protocol.CompareVersions(installedKey, versionKey)
			if err == nil && cmp == 0 {
				return installedKey, nil
			}
		}
		return "", &CLIError{Code: "NOT_INSTALLED", Message: "requested version is not installed", Details: map[string]any{"version_key": versionKey}}
	}
	keys := SortedInstalledVersionKeys(pkg)
	if len(keys) == 0 {
		return "", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{}}
	}
	if len(keys) == 1 {
		return keys[0], nil
	}
	highest := keys[0]
	second := keys[1]
	cmp, err := protocol.CompareVersions(highest, second)
	if err != nil {
		return "", &CLIError{Code: "MULTIPLE_VERSIONS_INSTALLED", Message: "multiple installed versions require --version", Details: map[string]any{}}
	}
	if cmp == 0 {
		return "", &CLIError{Code: "MULTIPLE_VERSIONS_INSTALLED", Message: "multiple installed versions require --version", Details: map[string]any{}}
	}
	return highest, nil
}

func versionKeysWithActiveAssets(pkg protocol.LockedPackage) []string {
	active := make([]string, 0)
	for versionKey, version := range pkg.InstalledVersions {
		for _, asset := range version.Assets {
			if asset.Active {
				active = append(active, versionKey)
				break
			}
		}
	}
	sort.Strings(active)
	return active
}

func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func atomicSymlink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}
	tmpPath := linkPath + ".tmp"
	_ = os.Remove(tmpPath)
	if err := os.Symlink(target, tmpPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, linkPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func fileBaseName(path string) string {
	return filepath.Base(filepath.FromSlash(path))
}

func concisePackageVersion(packageID, versionKey string) string {
	return fmt.Sprintf("%s@%s", packageID, versionKey)
}
