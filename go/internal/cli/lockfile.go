package cli

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type LockfileStore struct {
	Path string
}

func (s LockfileStore) Load() (protocol.Lockfile, bool, error) {
	body, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return protocol.Lockfile{}, false, nil
		}
		return protocol.Lockfile{}, false, err
	}
	var lock protocol.Lockfile
	if err := json.Unmarshal(body, &lock); err != nil {
		return protocol.Lockfile{}, false, &CLIError{
			Code:    "LOCKFILE_INVALID",
			Message: "lockfile is not valid JSON",
			Details: map[string]any{"path": s.Path},
		}
	}
	if err := ValidateLockfile(lock); err != nil {
		return protocol.Lockfile{}, false, err
	}
	return lock, true, nil
}

func ValidateLockfile(lock protocol.Lockfile) error {
	if lock.SchemaVersion != "1" {
		return lockfileError("invalid schema_version")
	}
	if lock.Packages == nil {
		return lockfileError("missing packages")
	}
	for packageID, pkg := range lock.Packages {
		if packageID == "" || packageID != strings.ToLower(packageID) {
			return lockfileError("invalid package id")
		}
		if pkg.InstalledVersions == nil {
			return lockfileError("missing installed_versions")
		}
		activeCounts := map[string]int{}
		for versionKey, version := range pkg.InstalledVersions {
			if versionKey != version.VersionKey || version.VersionKey == "" {
				return lockfileError("installed version key mismatch")
			}
			for _, asset := range version.Assets {
				if asset.Path == "" || asset.SHA256 == "" || asset.LocalPath == "" {
					return lockfileError("invalid locked asset")
				}
				if asset.Active {
					if asset.SymlinkPath == nil || *asset.SymlinkPath == "" {
						return lockfileError("active asset missing symlink_path")
					}
					activeCounts[versionKey]++
				}
			}
		}
		if pkg.ActiveVersionKey != nil {
			active := *pkg.ActiveVersionKey
			if _, ok := pkg.InstalledVersions[active]; !ok {
				return lockfileError("active_version_key not installed")
			}
			if activeCounts[active] == 0 {
				return lockfileError("active_version_key has no active assets")
			}
		}
		for versionKey, count := range activeCounts {
			if count > 0 {
				if pkg.ActiveVersionKey == nil || *pkg.ActiveVersionKey != versionKey {
					return lockfileError("active asset does not match active_version_key")
				}
			}
		}
	}
	return nil
}

func SortedInstalledVersionKeys(pkg protocol.LockedPackage) []string {
	keys := make([]string, 0, len(pkg.InstalledVersions))
	for versionKey := range pkg.InstalledVersions {
		keys = append(keys, versionKey)
	}
	sort.Slice(keys, func(i, j int) bool {
		cmp, err := protocol.CompareVersions(keys[i], keys[j])
		if err != nil {
			return keys[i] > keys[j]
		}
		return cmp > 0
	})
	return keys
}

func lockfileError(message string) error {
	return &CLIError{
		Code:    "LOCKFILE_INVALID",
		Message: message,
		Details: map[string]any{},
	}
}
