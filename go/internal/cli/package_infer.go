package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func unresolvedFields(manifest protocol.Manifest) []string {
	out := make([]string, 0)
	if manifest.Name == "" {
		out = append(out, "name")
	}
	if manifest.Author == "" {
		out = append(out, "author")
	}
	if manifest.Version == "" {
		out = append(out, "version")
	}
	return out
}

func commonName(values []string) string {
	if len(values) == 0 {
		return ""
	}
	first := values[0]
	for _, value := range values[1:] {
		if strings.EqualFold(first, value) {
			continue
		}
		return ""
	}
	return first
}

func commonNameSource(assets []inspection) string {
	if len(assets) == 0 {
		return "filename_heuristic"
	}
	source := ""
	for _, asset := range assets {
		if asset.Name == "" {
			continue
		}
		if source == "" {
			source = asset.nameSource
			continue
		}
		if source != asset.nameSource {
			return "filename_heuristic"
		}
	}
	if source == "" {
		return "filename_heuristic"
	}
	return source
}

func inferAuthorFromRepository(root string) (string, string, conflictRecord) {
	candidates := make([]conflictCandidate, 0, 2)
	if author, ok := inferAuthorFromREADME(root); ok {
		candidates = append(candidates, conflictCandidate{Value: author, Source: "repository_readme"})
	}
	if packageID, err := derivePackageID(root, ""); err == nil {
		parts := strings.SplitN(packageID, "/", 2)
		if len(parts) == 2 && parts[0] != "" {
			candidates = append(candidates, conflictCandidate{Value: parts[0], Source: "repository_owner"})
		}
	}
	return chooseRepositoryCandidate("author", candidates)
}

func inferAuthorFromREADME(root string) (string, bool) {
	body, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if !strings.Contains(lower, "copyright") {
			continue
		}
		open := strings.LastIndex(line, "[")
		close := strings.LastIndex(line, "]")
		if open >= 0 && close > open {
			value := strings.TrimSpace(line[open+1 : close])
			if value != "" {
				return value, true
			}
		}
		text := strings.TrimSpace(line)
		if text != "" {
			return text, true
		}
	}
	return "", false
}

func inferVersionFromRepository(root string) (string, string, conflictRecord) {
	candidates := make([]conflictCandidate, 0, 2)
	if version, ok := inferVersionFromChangelog(root); ok {
		candidates = append(candidates, conflictCandidate{Value: version, Source: "repository_changelog"})
	}
	if version, ok := inferVersionFromGitTags(root); ok {
		candidates = append(candidates, conflictCandidate{Value: version, Source: "repository_tag"})
	}
	return chooseRepositoryCandidate("version", candidates)
}

func inferVersionFromChangelog(root string) (string, bool) {
	body, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		candidate := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if candidate == "" {
			continue
		}
		versionKey, err := protocol.NormalizeVersionKey(candidate)
		if err == nil {
			return versionKey, true
		}
	}
	return "", false
}

func inferVersionFromGitTags(root string) (string, bool) {
	cmd := exec.Command("git", "tag", "--list")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	best := ""
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		tag := strings.TrimSpace(line)
		if tag == "" {
			continue
		}
		versionKey, err := protocol.NormalizeVersionKey(tag)
		if err != nil {
			continue
		}
		if best == "" {
			best = versionKey
			continue
		}
		cmp, err := protocol.CompareVersions(versionKey, best)
		if err == nil && cmp > 0 {
			best = versionKey
		}
	}
	return best, best != ""
}

func detectNameConflict(assets []inspection) conflictRecord {
	seen := map[string]string{}
	ordered := make([]conflictCandidate, 0)
	for _, asset := range assets {
		name := strings.TrimSpace(asset.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name) + "\x00" + asset.nameSource
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = name
		ordered = append(ordered, conflictCandidate{Value: name, Source: asset.nameSource})
	}
	distinct := map[string]struct{}{}
	for _, candidate := range ordered {
		distinct[strings.ToLower(candidate.Value)] = struct{}{}
	}
	if len(distinct) <= 1 {
		return conflictRecord{}
	}
	return conflictRecord{Field: "name", Candidates: ordered}
}

func chooseRepositoryCandidate(field string, candidates []conflictCandidate) (string, string, conflictRecord) {
	ordered := dedupeConflictCandidates(candidates)
	if len(ordered) == 0 {
		return "", "", conflictRecord{}
	}
	chosen := ordered[0]
	conflict := conflictRecord{}
	distinct := map[string]struct{}{}
	for _, candidate := range ordered {
		distinct[candidate.Value] = struct{}{}
	}
	if len(distinct) > 1 {
		conflict = conflictRecord{
			Field:       field,
			Resolved:    true,
			ChosenValue: chosen.Value,
			Candidates:  ordered,
		}
	}
	return chosen.Value, chosen.Source, conflict
}

func dedupeConflictCandidates(candidates []conflictCandidate) []conflictCandidate {
	out := make([]conflictCandidate, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Value) == "" {
			continue
		}
		key := candidate.Value + "\x00" + candidate.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

func finalizeConflicts(conflicts []conflictRecord, manifest protocol.Manifest) []conflictRecord {
	out := make([]conflictRecord, 0, len(conflicts))
	seen := map[string]struct{}{}
	for _, conflict := range conflicts {
		if len(conflict.Candidates) == 0 {
			continue
		}
		if _, ok := seen[conflict.Field]; ok {
			continue
		}
		seen[conflict.Field] = struct{}{}
		switch conflict.Field {
		case "name":
			if manifest.Name != "" {
				conflict.Resolved = true
				conflict.ChosenValue = manifest.Name
			}
		case "author":
			if manifest.Author != "" {
				conflict.Resolved = true
				conflict.ChosenValue = manifest.Author
			}
		case "version":
			if manifest.Version != "" {
				conflict.Resolved = true
				conflict.ChosenValue = manifest.Version
			}
		}
		out = append(out, conflict)
	}
	return out
}
