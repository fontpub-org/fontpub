package protocol

import (
	"fmt"
	"sort"
	"strings"
)

func ValidateManifest(m Manifest) error {
	if m.Name == "" || m.Author == "" || m.Version == "" || m.License == "" {
		return fmt.Errorf("MANIFEST_SCHEMA_INVALID: missing required field")
	}
	if m.License != "OFL-1.1" {
		return fmt.Errorf("LICENSE_NOT_ALLOWED: invalid license")
	}
	if _, err := ParseVersion(m.Version); err != nil {
		return err
	}
	if len(m.Files) == 0 || len(m.Files) > 256 {
		return fmt.Errorf("MANIFEST_SCHEMA_INVALID: invalid file count")
	}
	seen := map[string]struct{}{}
	for _, file := range m.Files {
		if err := ValidateAssetPath(file.Path); err != nil {
			return err
		}
		if _, err := FormatFromPath(file.Path); err != nil {
			return err
		}
		if file.Style != "normal" && file.Style != "italic" && file.Style != "oblique" {
			return fmt.Errorf("MANIFEST_SCHEMA_INVALID: invalid style")
		}
		if file.Weight < 1 || file.Weight > 1000 {
			return fmt.Errorf("MANIFEST_SCHEMA_INVALID: invalid weight")
		}
		if _, ok := seen[file.Path]; ok {
			return fmt.Errorf("ASSET_DUPLICATE_PATH: duplicate path")
		}
		seen[file.Path] = struct{}{}
	}
	return nil
}

func SortedManifestFiles(files []ManifestFile) []ManifestFile {
	out := append([]ManifestFile(nil), files...)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Path, out[j].Path) < 0
	})
	return out
}
