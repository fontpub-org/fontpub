package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type inferenceRecord struct {
	Field  string `json:"field"`
	Value  any    `json:"value"`
	Source string `json:"source"`
}

type conflictCandidate struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

type conflictRecord struct {
	Field       string              `json:"field"`
	Resolved    bool                `json:"resolved"`
	ChosenValue string              `json:"chosen_value,omitempty"`
	Candidates  []conflictCandidate `json:"candidates"`
}

type inspection struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Style  string `json:"style"`
	Weight int    `json:"weight"`
	Name   string `json:"name,omitempty"`

	styleSource  string
	weightSource string
	nameSource   string
}

func (a *App) buildCandidateManifest(root string) (protocol.Manifest, []inspection, []inferenceRecord, []conflictRecord, []string, error) {
	assets, err := scanFontAssets(root)
	if err != nil {
		return protocol.Manifest{}, nil, nil, nil, nil, err
	}
	assets = applyStemGrouping(assets)
	if len(assets) == 0 {
		return protocol.Manifest{}, nil, nil, nil, nil, &CLIError{Code: "INPUT_REQUIRED", Message: "no distributable font files found", Details: map[string]any{"root_path": root}}
	}
	files := make([]protocol.ManifestFile, 0, len(assets))
	inferences := make([]inferenceRecord, 0)
	conflicts := make([]conflictRecord, 0)
	nameCandidates := make([]string, 0)
	for i, asset := range assets {
		files = append(files, protocol.ManifestFile{Path: asset.Path, Style: asset.Style, Weight: asset.Weight})
		inferences = append(inferences,
			inferenceRecord{Field: fmt.Sprintf("files[%d].style", i), Value: asset.Style, Source: asset.styleSource},
			inferenceRecord{Field: fmt.Sprintf("files[%d].weight", i), Value: asset.Weight, Source: asset.weightSource},
		)
		if asset.Name != "" {
			nameCandidates = append(nameCandidates, asset.Name)
		}
	}
	manifest := protocol.Manifest{
		License: "OFL-1.1",
		Files:   files,
	}
	inferences = append(inferences, inferenceRecord{Field: "license", Value: "OFL-1.1", Source: "filename_heuristic"})
	if existing, err := readManifestAtRoot(root); err == nil {
		if existing.Name != "" {
			manifest.Name = existing.Name
			inferences = append(inferences, inferenceRecord{Field: "name", Value: existing.Name, Source: "user_input"})
		}
		if existing.Author != "" {
			manifest.Author = existing.Author
			inferences = append(inferences, inferenceRecord{Field: "author", Value: existing.Author, Source: "user_input"})
		}
		if existing.Version != "" {
			manifest.Version = existing.Version
			inferences = append(inferences, inferenceRecord{Field: "version", Value: existing.Version, Source: "user_input"})
		}
		if existing.License != "" {
			manifest.License = existing.License
			inferences = append(inferences, inferenceRecord{Field: "license", Value: existing.License, Source: "user_input"})
		}
	}
	if len(nameCandidates) > 0 {
		name := commonName(nameCandidates)
		if name != "" {
			manifest.Name = name
			inferences = append(inferences, inferenceRecord{Field: "name", Value: name, Source: commonNameSource(assets)})
		} else if conflict := detectNameConflict(assets); len(conflict.Candidates) > 0 {
			conflicts = append(conflicts, conflict)
		}
	}
	if manifest.Author == "" {
		author, source, authorConflict := inferAuthorFromRepository(root)
		if len(authorConflict.Candidates) > 0 {
			conflicts = append(conflicts, authorConflict)
		}
		if author != "" {
			manifest.Author = author
			inferences = append(inferences, inferenceRecord{Field: "author", Value: author, Source: source})
		}
	}
	if manifest.Version == "" {
		version, source, versionConflict := inferVersionFromRepository(root)
		if len(versionConflict.Candidates) > 0 {
			conflicts = append(conflicts, versionConflict)
		}
		if version != "" {
			manifest.Version = version
			inferences = append(inferences, inferenceRecord{Field: "version", Value: version, Source: source})
		}
	}
	conflicts = finalizeConflicts(conflicts, manifest)
	return manifest, assets, inferences, conflicts, unresolvedFields(manifest), nil
}

func (a *App) buildCandidatePackageDetail(root, explicitPackageID string) (protocol.CandidatePackageDetail, error) {
	manifest, err := readManifestAtRoot(root)
	if err != nil {
		return protocol.CandidatePackageDetail{}, err
	}
	if err := protocol.ValidateManifest(manifest); err != nil {
		return protocol.CandidatePackageDetail{}, protocolErrorToCLI(err)
	}
	versionKey, err := protocol.NormalizeVersionKey(manifest.Version)
	if err != nil {
		return protocol.CandidatePackageDetail{}, protocolErrorToCLI(err)
	}
	packageID, err := derivePackageID(root, explicitPackageID)
	if err != nil {
		return protocol.CandidatePackageDetail{}, err
	}
	assets := make([]protocol.CandidateAsset, 0, len(manifest.Files))
	for _, file := range protocol.SortedManifestFiles(manifest.Files) {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		body, err := os.ReadFile(path)
		if err != nil {
			return protocol.CandidatePackageDetail{}, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "declared manifest file is missing", Details: map[string]any{"path": file.Path}}
		}
		sum := sha256.Sum256(body)
		format, err := protocol.FormatFromPath(file.Path)
		if err != nil {
			return protocol.CandidatePackageDetail{}, protocolErrorToCLI(err)
		}
		assets = append(assets, protocol.CandidateAsset{
			Path:      file.Path,
			SHA256:    hex.EncodeToString(sum[:]),
			Format:    format,
			Style:     file.Style,
			Weight:    file.Weight,
			SizeBytes: int64(len(body)),
		})
	}
	return protocol.CandidatePackageDetail{
		SchemaVersion: "1",
		PackageID:     packageID,
		DisplayName:   manifest.Name,
		Author:        manifest.Author,
		License:       manifest.License,
		Version:       manifest.Version,
		VersionKey:    versionKey,
		Source: protocol.CandidateSource{
			Kind:     "local_repository",
			RootPath: root,
		},
		Assets: assets,
	}, nil
}

func (a *App) promptForManifestFields(manifest *protocol.Manifest, inferences *[]inferenceRecord, unresolved []string) error {
	if a.Stdin == nil {
		return &CLIError{Code: "TTY_REQUIRED", Message: "interactive input is required", Details: map[string]any{}}
	}
	reader := bufio.NewReader(a.Stdin)
	for _, field := range unresolved {
		switch field {
		case "name":
			value, err := prompt(reader, a.Stdout, "Name")
			if err != nil {
				return &CLIError{Code: "INPUT_REQUIRED", Message: "could not read name", Details: map[string]any{}}
			}
			manifest.Name = value
			*inferences = append(*inferences, inferenceRecord{Field: "name", Value: value, Source: "user_input"})
		case "author":
			value, err := prompt(reader, a.Stdout, "Author")
			if err != nil {
				return &CLIError{Code: "INPUT_REQUIRED", Message: "could not read author", Details: map[string]any{}}
			}
			manifest.Author = value
			*inferences = append(*inferences, inferenceRecord{Field: "author", Value: value, Source: "user_input"})
		case "version":
			value, err := prompt(reader, a.Stdout, "Version")
			if err != nil {
				return &CLIError{Code: "INPUT_REQUIRED", Message: "could not read version", Details: map[string]any{}}
			}
			manifest.Version = value
			*inferences = append(*inferences, inferenceRecord{Field: "version", Value: value, Source: "user_input"})
		}
	}
	return nil
}

func readManifestAtRoot(root string) (protocol.Manifest, error) {
	body, err := os.ReadFile(filepath.Join(root, "fontpub.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return protocol.Manifest{}, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "fontpub.json was not found", Details: map[string]any{"path": filepath.Join(root, "fontpub.json")}}
		}
		return protocol.Manifest{}, &CLIError{Code: "INTERNAL_ERROR", Message: "could not read fontpub.json", Details: map[string]any{"path": filepath.Join(root, "fontpub.json"), "reason": err.Error()}}
	}
	var manifest protocol.Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return protocol.Manifest{}, &CLIError{Code: "MANIFEST_INVALID_JSON", Message: "fontpub.json is not valid JSON", Details: map[string]any{}}
	}
	return manifest, nil
}

func scanFontAssets(root string) ([]inspection, error) {
	out := make([]inspection, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		format, err := protocol.FormatFromPath(rel)
		if err != nil {
			return nil
		}
		info, err := inspectFontFile(path, rel, format)
		if err != nil {
			return err
		}
		out = append(out, info)
		return nil
	})
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not scan repository", Details: map[string]any{"root_path": root, "reason": err.Error()}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func inspectFontPath(path, root string) (inspection, error) {
	rel := path
	fullPath := path
	if !filepath.IsAbs(path) {
		rel = filepath.ToSlash(path)
		fullPath = filepath.Join(root, filepath.FromSlash(rel))
	}
	if _, err := os.Stat(fullPath); err != nil {
		return inspection{}, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "font file does not exist", Details: map[string]any{"path": path}}
	}
	format, err := protocol.FormatFromPath(rel)
	if err != nil {
		return inspection{}, protocolErrorToCLI(err)
	}
	return inspectFontFile(fullPath, rel, format)
}

func inspectFontFile(fullPath, relPath, format string) (inspection, error) {
	style, weight, name := inferFromFilename(relPath)
	info := inspection{
		Path:         relPath,
		Format:       format,
		Style:        style,
		Weight:       weight,
		Name:         name,
		styleSource:  "filename_heuristic",
		weightSource: "filename_heuristic",
		nameSource:   "filename_heuristic",
	}

	body, err := os.ReadFile(fullPath)
	if err != nil {
		return inspection{}, &CLIError{Code: "INTERNAL_ERROR", Message: "could not read font file", Details: map[string]any{"path": relPath, "reason": err.Error()}}
	}
	if meta, ok := parseEmbeddedFontMetadata(relPath, body); ok {
		if meta.Style != "" {
			info.Style = meta.Style
			info.styleSource = "embedded_metadata"
		}
		if meta.Weight > 0 {
			info.Weight = meta.Weight
			info.weightSource = "embedded_metadata"
		}
		if meta.Family != "" {
			info.Name = meta.Family
			info.nameSource = "embedded_metadata"
		}
	}
	return info, nil
}

func applyStemGrouping(assets []inspection) []inspection {
	grouped := make(map[string][]int)
	for i, asset := range assets {
		grouped[stemGroupKey(asset.Path)] = append(grouped[stemGroupKey(asset.Path)], i)
	}
	for _, indexes := range grouped {
		if len(indexes) < 2 {
			continue
		}
		bestNameValue, bestNameSource, hasName := bestGroupedStringField(assets, indexes, func(asset inspection) (string, string) {
			return asset.Name, asset.nameSource
		})
		bestStyleValue, bestStyleSource, hasStyle := bestGroupedStringField(assets, indexes, func(asset inspection) (string, string) {
			return asset.Style, asset.styleSource
		})
		bestWeightValue, bestWeightSource, hasWeight := bestGroupedIntField(assets, indexes, func(asset inspection) (int, string) {
			return asset.Weight, asset.weightSource
		})
		for _, idx := range indexes {
			if hasName && groupedSourcePriority(bestNameSource) > groupedSourcePriority(assets[idx].nameSource) {
				assets[idx].Name = bestNameValue
				assets[idx].nameSource = groupedInheritedSource(bestNameSource)
			}
			if hasStyle && groupedSourcePriority(bestStyleSource) > groupedSourcePriority(assets[idx].styleSource) {
				assets[idx].Style = bestStyleValue
				assets[idx].styleSource = groupedInheritedSource(bestStyleSource)
			}
			if hasWeight && groupedSourcePriority(bestWeightSource) > groupedSourcePriority(assets[idx].weightSource) {
				assets[idx].Weight = bestWeightValue
				assets[idx].weightSource = groupedInheritedSource(bestWeightSource)
			}
		}
	}
	return assets
}

func stemGroupKey(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if dir == "." || dir == "" {
		return base
	}
	return dir + "/" + base
}

func bestGroupedStringField(assets []inspection, indexes []int, get func(inspection) (string, string)) (string, string, bool) {
	bestValue := ""
	bestSource := ""
	bestPriority := -1
	bestFormatPriority := -1
	for _, idx := range indexes {
		value, source := get(assets[idx])
		if strings.TrimSpace(value) == "" {
			continue
		}
		priority := groupedSourcePriority(source)
		formatPriority := groupedFormatPriority(assets[idx].Format)
		if priority > bestPriority || (priority == bestPriority && formatPriority > bestFormatPriority) {
			bestValue = value
			bestSource = source
			bestPriority = priority
			bestFormatPriority = formatPriority
		}
	}
	return bestValue, bestSource, bestPriority >= 0
}

func bestGroupedIntField(assets []inspection, indexes []int, get func(inspection) (int, string)) (int, string, bool) {
	bestValue := 0
	bestSource := ""
	bestPriority := -1
	bestFormatPriority := -1
	for _, idx := range indexes {
		value, source := get(assets[idx])
		if value <= 0 {
			continue
		}
		priority := groupedSourcePriority(source)
		formatPriority := groupedFormatPriority(assets[idx].Format)
		if priority > bestPriority || (priority == bestPriority && formatPriority > bestFormatPriority) {
			bestValue = value
			bestSource = source
			bestPriority = priority
			bestFormatPriority = formatPriority
		}
	}
	return bestValue, bestSource, bestPriority >= 0
}

func groupedSourcePriority(source string) int {
	switch source {
	case "embedded_metadata":
		return 30
	case "group_embedded_metadata":
		return 25
	case "filename_heuristic":
		return 10
	default:
		return 0
	}
}

func groupedFormatPriority(format string) int {
	switch format {
	case "otf":
		return 3
	case "ttf":
		return 2
	case "woff2":
		return 1
	default:
		return 0
	}
}

func groupedInheritedSource(source string) string {
	if source == "embedded_metadata" || source == "group_embedded_metadata" {
		return "group_embedded_metadata"
	}
	return source
}

func inferFromFilename(path string) (string, int, string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	lower := strings.ToLower(base)
	style := "normal"
	weight := 400
	switch {
	case strings.Contains(lower, "italic"):
		style = "italic"
	case strings.Contains(lower, "oblique"):
		style = "oblique"
	}
	switch {
	case strings.Contains(lower, "thin"):
		weight = 100
	case strings.Contains(lower, "extralight") || strings.Contains(lower, "ultralight"):
		weight = 200
	case strings.Contains(lower, "light"):
		weight = 300
	case strings.Contains(lower, "medium"):
		weight = 500
	case strings.Contains(lower, "semibold") || strings.Contains(lower, "demibold"):
		weight = 600
	case strings.Contains(lower, "extrabold") || strings.Contains(lower, "ultrabold"):
		weight = 800
	case strings.Contains(lower, "bold"):
		weight = 700
	case strings.Contains(lower, "black") || strings.Contains(lower, "heavy"):
		weight = 900
	}
	name := base
	for _, token := range []string{"-thin", "-extralight", "-ultralight", "-light", "-regular", "-italic", "-oblique", "-medium", "-semibold", "-demibold", "-bold", "-extrabold", "-ultrabold", "-black", "-heavy"} {
		if idx := strings.Index(strings.ToLower(name), token); idx > 0 {
			name = name[:idx]
			break
		}
	}
	name = stripVariableAxisSuffix(name)
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = splitCamelCaseName(name)
	name = strings.TrimSpace(name)
	return style, weight, name
}

func stripVariableAxisSuffix(name string) string {
	if idx := strings.Index(name, "["); idx > 0 && strings.HasSuffix(name, "]") {
		return name[:idx]
	}
	return name
}

func splitCamelCaseName(name string) string {
	if name == "" {
		return name
	}
	runes := []rune(name)
	out := make([]rune, 0, len(runes)+4)
	for i, r := range runes {
		if i > 0 && shouldInsertNameSpace(runes, i) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	return string(out)
}

func shouldInsertNameSpace(runes []rune, idx int) bool {
	prev := runes[idx-1]
	current := runes[idx]
	if prev == ' ' || current == ' ' {
		return false
	}
	if prev >= '0' && prev <= '9' {
		return false
	}
	if idx > 1 {
		beforePrev := runes[idx-2]
		if beforePrev >= '0' && beforePrev <= '9' {
			return false
		}
	}
	return isLowerASCII(prev) && isUpperASCII(current)
}

func isLowerASCII(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isUpperASCII(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

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

func printPackageInitSummary(w io.Writer, root string, manifest protocol.Manifest, assets []inspection, inferences []inferenceRecord, conflicts []conflictRecord, unresolved []string) {
	fmt.Fprintf(w, "Repository: %s\n", root)
	fmt.Fprintln(w, "Discovered assets:")
	for _, asset := range assets {
		fmt.Fprintf(
			w,
			"  - %s [%s] style=%s (%s) weight=%d (%s)\n",
			asset.Path,
			asset.Format,
			asset.Style,
			humanizeInferenceSource(asset.styleSource),
			asset.Weight,
			humanizeInferenceSource(asset.weightSource),
		)
		if asset.Name != "" {
			fmt.Fprintf(w, "    family=%s (%s)\n", asset.Name, humanizeInferenceSource(asset.nameSource))
		}
	}

	inferenceByField := map[string]inferenceRecord{}
	for _, inference := range inferences {
		inferenceByField[inference.Field] = inference
	}

	fmt.Fprintln(w, "Manifest fields:")
	printManifestFieldSummary(w, "name", manifest.Name, inferenceByField["name"])
	printManifestFieldSummary(w, "author", manifest.Author, inferenceByField["author"])
	printManifestFieldSummary(w, "version", manifest.Version, inferenceByField["version"])
	printManifestFieldSummary(w, "license", manifest.License, inferenceByField["license"])

	if len(conflicts) == 0 {
		fmt.Fprintln(w, "Conflicts: none")
	} else {
		fmt.Fprintln(w, "Conflicts:")
		for _, conflict := range conflicts {
			status := "unresolved"
			if conflict.Resolved {
				status = "resolved"
			}
			fmt.Fprintf(w, "  %s (%s)\n", conflict.Field, status)
			if conflict.Resolved && conflict.ChosenValue != "" {
				fmt.Fprintf(w, "    chosen: %s\n", conflict.ChosenValue)
			}
			for _, candidate := range conflict.Candidates {
				fmt.Fprintf(w, "    - %s (%s)\n", candidate.Value, humanizeInferenceSource(candidate.Source))
			}
		}
	}

	if len(unresolved) == 0 {
		fmt.Fprintln(w, "Unresolved fields: none")
	} else {
		fmt.Fprintf(w, "Unresolved fields: %s\n", strings.Join(unresolved, ", "))
	}
	fmt.Fprintln(w)
}

func printManifestFieldSummary(w io.Writer, field, value string, inference inferenceRecord) {
	if value == "" {
		fmt.Fprintf(w, "  %s: unresolved\n", field)
		return
	}
	source := inference.Source
	if source == "" {
		source = "user_input"
	}
	fmt.Fprintf(w, "  %s: %s (%s)\n", field, value, humanizeInferenceSource(source))
}

func printInspectionSummary(w io.Writer, info inspection) {
	fmt.Fprintf(w, "Path: %s\n", info.Path)
	fmt.Fprintf(w, "Format: %s\n", info.Format)
	if info.Name != "" {
		fmt.Fprintf(w, "Family: %s (%s)\n", info.Name, humanizeInferenceSource(info.nameSource))
	}
	fmt.Fprintf(w, "Style: %s (%s)\n", info.Style, humanizeInferenceSource(info.styleSource))
	fmt.Fprintf(w, "Weight: %d (%s)\n", info.Weight, humanizeInferenceSource(info.weightSource))
}

func humanizeInferenceSource(source string) string {
	switch source {
	case "embedded_metadata":
		return "embedded metadata"
	case "group_embedded_metadata":
		return "grouped embedded metadata"
	case "repository_readme":
		return "repository README"
	case "repository_owner":
		return "repository owner"
	case "repository_changelog":
		return "repository changelog"
	case "repository_tag":
		return "repository tag"
	case "filename_heuristic":
		return "filename heuristic"
	case "user_input":
		return "user input"
	default:
		return source
	}
}

func oneOptionalPath(args []string) (string, *CLIError) {
	if len(args) > 1 {
		return "", &CLIError{Code: "INPUT_REQUIRED", Message: "too many positional arguments", Details: map[string]any{}}
	}
	if len(args) == 0 {
		root, err := os.Getwd()
		if err != nil {
			return "", &CLIError{Code: "INTERNAL_ERROR", Message: "could not determine current working directory", Details: map[string]any{}}
		}
		return root, nil
	}
	root, err := filepath.Abs(args[0])
	if err != nil {
		return "", &CLIError{Code: "INTERNAL_ERROR", Message: "could not resolve path", Details: map[string]any{"path": args[0]}}
	}
	return root, nil
}

func derivePackageID(root, explicit string) (string, error) {
	if explicit != "" {
		id := normalizePackageID(explicit)
		parts := strings.Split(id, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", &CLIError{Code: "PACKAGE_ID_REQUIRED", Message: "invalid package id", Details: map[string]any{"package_id": explicit}}
		}
		return id, nil
	}
	cmd := exec.Command("git", "config", "--get-regexp", "^remote\\..*\\.url$")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", &CLIError{Code: "PACKAGE_ID_REQUIRED", Message: "could not derive package id from git remotes", Details: map[string]any{}}
	}
	ids := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if id, ok := parseGitHubRemote(fields[1]); ok {
			ids[id] = struct{}{}
		}
	}
	switch len(ids) {
	case 0:
		return "", &CLIError{Code: "PACKAGE_ID_REQUIRED", Message: "could not derive package id from git remotes", Details: map[string]any{}}
	case 1:
		for id := range ids {
			return id, nil
		}
	default:
		return "", &CLIError{Code: "PACKAGE_ID_AMBIGUOUS", Message: "multiple package ids were derived from git remotes", Details: map[string]any{}}
	}
	return "", &CLIError{Code: "PACKAGE_ID_REQUIRED", Message: "could not derive package id", Details: map[string]any{}}
}

func parseGitHubRemote(raw string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(lower, "https://github.com/"):
		trimmed := strings.TrimPrefix(lower, "https://github.com/")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.Split(trimmed, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "/" + parts[1], true
		}
	case strings.HasPrefix(lower, "git@github.com:"):
		trimmed := strings.TrimPrefix(lower, "git@github.com:")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.Split(trimmed, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + "/" + parts[1], true
		}
	}
	return "", false
}

func prompt(r *bufio.Reader, w io.Writer, label string) (string, error) {
	fmt.Fprintf(w, "%s: ", label)
	value, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func inferenceRecordsToAny(records []inferenceRecord) []any {
	out := make([]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"field":  record.Field,
			"value":  record.Value,
			"source": record.Source,
		})
	}
	return out
}

func conflictRecordsToAny(records []conflictRecord) []any {
	out := make([]any, 0, len(records))
	for _, record := range records {
		candidates := make([]any, 0, len(record.Candidates))
		for _, candidate := range record.Candidates {
			candidates = append(candidates, map[string]any{
				"value":  candidate.Value,
				"source": candidate.Source,
			})
		}
		item := map[string]any{
			"field":      record.Field,
			"resolved":   record.Resolved,
			"candidates": candidates,
		}
		if record.Resolved && record.ChosenValue != "" {
			item["chosen_value"] = record.ChosenValue
		}
		out = append(out, item)
	}
	return out
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func mustMap(value any) map[string]any {
	body, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return out
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

func protocolErrorToCLI(err error) *CLIError {
	message := err.Error()
	code := message
	if idx := strings.Index(message, ":"); idx >= 0 {
		code = message[:idx]
		message = strings.TrimSpace(message[idx+1:])
	}
	return &CLIError{Code: code, Message: message, Details: map[string]any{}}
}

func generatedWorkflowYAML(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	return strings.TrimSpace(fmt.Sprintf(`
name: Fontpub

on:
  push:
    tags:
      - "[0-9]*"
      - "v*"
      - "V*"
  workflow_dispatch:
    inputs:
      tag:
        description: Release tag
        required: true
        type: string

permissions:
  id-token: write
  contents: read

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - name: Determine publication ref
        id: ref
        run: |
          set -euo pipefail
          if [ "${GITHUB_EVENT_NAME}" = "workflow_dispatch" ]; then
            TAG="${{ inputs.tag }}"
            if [ -z "${TAG}" ]; then
              echo "::error::workflow_dispatch requires a tag input"
              exit 1
            fi
            if ! printf '%%s\n' "${TAG}" | grep -Eq '^[vV]?(0|[1-9][0-9]*)(\.[0-9]+)*$'; then
              echo "::error::tag must match Fontpub versioning (example: 1.002 or v1.2.3)"
              exit 1
            fi
            echo "ref=refs/tags/${TAG}" >> "$GITHUB_OUTPUT"
          else
            echo "ref=${GITHUB_REF}" >> "$GITHUB_OUTPUT"
          fi
      - name: Checkout
        uses: actions/checkout@v4
        with:
          ref: ${{ steps.ref.outputs.ref }}
          fetch-depth: 0
          persist-credentials: false
      - name: Resolve publication commit
        id: sha
        run: |
          set -euo pipefail
          git rev-parse --verify "${{ steps.ref.outputs.ref }}^{commit}" > sha.txt
          echo "sha=$(cat sha.txt)" >> "$GITHUB_OUTPUT"
      - name: Request OIDC token
        id: token
        run: |
          set -euo pipefail
          RESPONSE=$(curl -fsSL -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=https://fontpub.org")
          TOKEN=$(printf '%%s' "$RESPONSE" | python -c 'import json,sys; print(json.load(sys.stdin)["value"])')
          printf 'token=%%s\n' "$TOKEN" >> "$GITHUB_OUTPUT"
      - name: Publish
        env:
          TOKEN: ${{ steps.token.outputs.token }}
          REPOSITORY: ${{ github.repository }}
          SHA: ${{ steps.sha.outputs.sha }}
          REF: ${{ steps.ref.outputs.ref }}
        run: |
          set -euo pipefail
          BODY=$(python - <<'PY'
import json
import os

print(json.dumps({
    "repository": os.environ["REPOSITORY"],
    "sha": os.environ["SHA"],
    "ref": os.environ["REF"],
}, separators=(",", ":")))
PY
          )
          curl -fsSL \
            -H "Authorization: Bearer ${TOKEN}" \
            -H "Content-Type: application/json" \
            -d "$BODY" \
            %s/v1/update
`, baseURL))
}
