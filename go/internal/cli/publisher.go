package cli

import (
	"bufio"
	"context"
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

type inspection struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Style  string `json:"style"`
	Weight int    `json:"weight"`
	Name   string `json:"name,omitempty"`
}

func (a *App) runPackage(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.fail("package", &CLIError{Code: "INPUT_REQUIRED", Message: "package subcommand is required", Details: map[string]any{}})
	}
	switch args[0] {
	case "init":
		return a.runPackageInit(ctx, args[1:])
	case "validate":
		return a.runPackageValidate(ctx, args[1:])
	case "preview":
		return a.runPackagePreview(ctx, args[1:])
	case "inspect":
		return a.runPackageInspect(ctx, args[1:])
	case "check":
		return a.runPackageCheck(ctx, args[1:])
	default:
		return a.fail("package "+args[0], &CLIError{Code: "INTERNAL_ERROR", Message: "package subcommand is not implemented", Details: map[string]any{"command": args[0]}})
	}
}

func (a *App) runWorkflow(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.fail("workflow", &CLIError{Code: "INPUT_REQUIRED", Message: "workflow subcommand is required", Details: map[string]any{}})
	}
	switch args[0] {
	case "init":
		return a.runWorkflowInit(ctx, args[1:])
	default:
		return a.fail("workflow "+args[0], &CLIError{Code: "INTERNAL_ERROR", Message: "workflow subcommand is not implemented", Details: map[string]any{"command": args[0]}})
	}
}

func (a *App) runPackageInit(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("package init", errObj)
	}
	writeFile, args, errObj := extractBoolFlag(args, "--write")
	if errObj != nil {
		return a.fail("package init", errObj)
	}
	yes, args, errObj := extractBoolFlag(args, "--yes")
	if errObj != nil {
		return a.fail("package init", errObj)
	}
	root, errObj := oneOptionalPath(args)
	if errObj != nil {
		return a.fail("package init", errObj)
	}

	manifest, inferences, unresolved, err := a.buildCandidateManifest(root)
	if err != nil {
		return a.fail("package init", asCLIError(err))
	}

	if len(unresolved) > 0 {
		if a.JSON {
			return a.fail("package init", &CLIError{Code: "INPUT_REQUIRED", Message: "required manifest fields could not be inferred", Details: map[string]any{"unresolved_fields": unresolved}})
		}
		if err := a.promptForManifestFields(&manifest, &inferences, unresolved); err != nil {
			return a.fail("package init", asCLIError(err))
		}
		unresolved = unresolvedFields(manifest)
		if len(unresolved) > 0 {
			return a.fail("package init", &CLIError{Code: "INPUT_REQUIRED", Message: "required manifest fields could not be resolved", Details: map[string]any{"unresolved_fields": unresolved}})
		}
	}

	data := map[string]any{
		"manifest":          mustMap(manifest),
		"inferences":        inferenceRecordsToAny(inferences),
		"unresolved_fields": stringSliceToAny(unresolved),
	}
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "package init", Data: data}
		if err := protocol.ValidatePackageInitResult(env); err != nil {
			return a.fail("package init", &CLIError{Code: "INTERNAL_ERROR", Message: "package init output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}

	if writeFile {
		target := filepath.Join(root, "fontpub.json")
		if _, err := os.Stat(target); err == nil && !yes {
			return a.fail("package init", &CLIError{Code: "INPUT_REQUIRED", Message: "refusing to overwrite existing fontpub.json without --yes", Details: map[string]any{"path": target}})
		}
		body, err := protocol.MarshalCanonical(manifest)
		if err != nil {
			return a.fail("package init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize manifest", Details: map[string]any{"reason": err.Error()}})
		}
		if !dryRun {
			if err := os.WriteFile(target, append(body, '\n'), 0o644); err != nil {
				return a.fail("package init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not write manifest", Details: map[string]any{"path": target, "reason": err.Error()}})
			}
		}
		fmt.Fprintf(a.Stdout, "manifest ready: %s\n", target)
		return 0
	}

	body, err := protocol.MarshalCanonical(manifest)
	if err != nil {
		return a.fail("package init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize manifest", Details: map[string]any{"reason": err.Error()}})
	}
	_, _ = a.Stdout.Write(append(body, '\n'))
	return 0
}

func (a *App) runPackageValidate(_ context.Context, args []string) int {
	root, errObj := oneOptionalPath(args)
	if errObj != nil {
		return a.fail("package validate", errObj)
	}
	manifest, err := readManifestAtRoot(root)
	if err != nil {
		return a.fail("package validate", asCLIError(err))
	}
	if err := protocol.ValidateManifest(manifest); err != nil {
		return a.fail("package validate", protocolErrorToCLI(err))
	}
	for _, file := range manifest.Files {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file.Path))); err != nil {
			return a.fail("package validate", &CLIError{Code: "LOCAL_FILE_MISSING", Message: "declared manifest file is missing", Details: map[string]any{"path": file.Path}})
		}
	}
	data := map[string]any{
		"manifest":   mustMap(manifest),
		"root_path":  root,
		"validated":  true,
		"file_count": len(manifest.Files),
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "package validate", Data: data})
	}
	fmt.Fprintf(a.Stdout, "valid manifest: %s\n", filepath.Join(root, "fontpub.json"))
	return 0
}

func (a *App) runPackagePreview(_ context.Context, args []string) int {
	packageID, args, errObj := extractStringFlag(args, "--package-id")
	if errObj != nil {
		return a.fail("package preview", errObj)
	}
	root, errObj := oneOptionalPath(args)
	if errObj != nil {
		return a.fail("package preview", errObj)
	}
	candidate, err := a.buildCandidatePackageDetail(root, packageID)
	if err != nil {
		return a.fail("package preview", asCLIError(err))
	}
	data := mustMap(candidate)
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "package preview", Data: data}
		if err := protocol.ValidatePackagePreviewResult(env); err != nil {
			return a.fail("package preview", &CLIError{Code: "INTERNAL_ERROR", Message: "package preview output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	body, err := protocol.MarshalCanonical(candidate)
	if err != nil {
		return a.fail("package preview", &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize preview", Details: map[string]any{"reason": err.Error()}})
	}
	_, _ = a.Stdout.Write(append(body, '\n'))
	return 0
}

func (a *App) runPackageInspect(_ context.Context, args []string) int {
	if len(args) != 1 {
		return a.fail("package inspect", &CLIError{Code: "INPUT_REQUIRED", Message: "package inspect requires <font-file>", Details: map[string]any{}})
	}
	info, err := inspectFontPath(args[0], ".")
	if err != nil {
		return a.fail("package inspect", asCLIError(err))
	}
	data := mustMap(info)
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "package inspect", Data: data})
	}
	fmt.Fprintf(a.Stdout, "%s %s %d %s\n", info.Path, info.Style, info.Weight, info.Format)
	return 0
}

func (a *App) runPackageCheck(_ context.Context, args []string) int {
	tag, args, errObj := extractStringFlag(args, "--tag")
	if errObj != nil {
		return a.fail("package check", errObj)
	}
	root, errObj := oneOptionalPath(args)
	if errObj != nil {
		return a.fail("package check", errObj)
	}
	manifest, err := readManifestAtRoot(root)
	if err != nil {
		return a.fail("package check", asCLIError(err))
	}
	if err := protocol.ValidateManifest(manifest); err != nil {
		return a.fail("package check", protocolErrorToCLI(err))
	}
	for _, file := range manifest.Files {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(file.Path))); err != nil {
			return a.fail("package check", &CLIError{Code: "LOCAL_FILE_MISSING", Message: "declared manifest file is missing", Details: map[string]any{"path": file.Path}})
		}
	}
	if tag != "" {
		tagKey, err := protocol.NormalizeVersionKey(tag)
		if err != nil {
			return a.fail("package check", protocolErrorToCLI(err))
		}
		versionKey, err := protocol.NormalizeVersionKey(manifest.Version)
		if err != nil {
			return a.fail("package check", protocolErrorToCLI(err))
		}
		if tagKey != versionKey {
			return a.fail("package check", &CLIError{Code: "TAG_VERSION_MISMATCH", Message: "tag version does not match manifest version", Details: map[string]any{"tag": tag, "manifest_version": manifest.Version}})
		}
	}
	data := map[string]any{
		"root_path": root,
		"ready":     true,
	}
	if tag != "" {
		data["tag"] = tag
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "package check", Data: data})
	}
	fmt.Fprintln(a.Stdout, "package is ready for publication")
	return 0
}

func (a *App) runWorkflowInit(_ context.Context, args []string) int {
	dryRun, args, errObj := extractBoolFlag(args, "--dry-run")
	if errObj != nil {
		return a.fail("workflow init", errObj)
	}
	yes, args, errObj := extractBoolFlag(args, "--yes")
	if errObj != nil {
		return a.fail("workflow init", errObj)
	}
	root, errObj := oneOptionalPath(args)
	if errObj != nil {
		return a.fail("workflow init", errObj)
	}
	target := filepath.Join(root, ".github", "workflows", "fontpub.yml")
	body := []byte(generatedWorkflowYAML(a.Config.BaseURL))
	planned := []PlannedAction{{Type: "write_workflow", PackageID: ""}}
	if a.JSON {
		data := map[string]any{"changed": true}
		if dryRun {
			data["planned_actions"] = plannedActionsToAny(planned)
		}
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "workflow init", Data: data})
	}
	if _, err := os.Stat(target); err == nil && !yes {
		return a.fail("workflow init", &CLIError{Code: "INPUT_REQUIRED", Message: "refusing to overwrite existing workflow without --yes", Details: map[string]any{"path": target}})
	}
	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return a.fail("workflow init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not create workflow directory", Details: map[string]any{"path": target, "reason": err.Error()}})
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return a.fail("workflow init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not write workflow", Details: map[string]any{"path": target, "reason": err.Error()}})
		}
	}
	fmt.Fprintf(a.Stdout, "workflow ready: %s\n", target)
	return 0
}

func (a *App) buildCandidateManifest(root string) (protocol.Manifest, []inferenceRecord, []string, error) {
	assets, err := scanFontAssets(root)
	if err != nil {
		return protocol.Manifest{}, nil, nil, err
	}
	if len(assets) == 0 {
		return protocol.Manifest{}, nil, nil, &CLIError{Code: "INPUT_REQUIRED", Message: "no distributable font files found", Details: map[string]any{"root_path": root}}
	}
	files := make([]protocol.ManifestFile, 0, len(assets))
	inferences := make([]inferenceRecord, 0)
	nameCandidates := make([]string, 0)
	for i, asset := range assets {
		files = append(files, protocol.ManifestFile{Path: asset.Path, Style: asset.Style, Weight: asset.Weight})
		inferences = append(inferences,
			inferenceRecord{Field: fmt.Sprintf("files[%d].style", i), Value: asset.Style, Source: "filename_heuristic"},
			inferenceRecord{Field: fmt.Sprintf("files[%d].weight", i), Value: asset.Weight, Source: "filename_heuristic"},
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
			inferences = append(inferences, inferenceRecord{Field: "name", Value: name, Source: "filename_heuristic"})
		}
	}
	return manifest, inferences, unresolvedFields(manifest), nil
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
		style, weight, name := inferFromFilename(rel)
		out = append(out, inspection{
			Path:   rel,
			Format: format,
			Style:  style,
			Weight: weight,
			Name:   name,
		})
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
	if !filepath.IsAbs(path) {
		rel = filepath.ToSlash(path)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil && !filepath.IsAbs(path) {
		return inspection{}, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "font file does not exist", Details: map[string]any{"path": path}}
	}
	format, err := protocol.FormatFromPath(rel)
	if err != nil {
		return inspection{}, protocolErrorToCLI(err)
	}
	style, weight, name := inferFromFilename(rel)
	return inspection{Path: rel, Format: format, Style: style, Weight: weight, Name: name}, nil
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
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.TrimSpace(name)
	return style, weight, name
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
      - name: Determine ref
        id: ref
        run: |
          if [ "${GITHUB_EVENT_NAME}" = "workflow_dispatch" ]; then
            echo "ref=refs/tags/${{ inputs.tag }}" >> "$GITHUB_OUTPUT"
          else
            echo "ref=${GITHUB_REF}" >> "$GITHUB_OUTPUT"
          fi
      - name: Checkout
        uses: actions/checkout@v4
        with:
          ref: ${{ steps.ref.outputs.ref }}
      - name: Resolve tag commit
        id: sha
        run: |
          git rev-parse "${{ steps.ref.outputs.ref }}" > sha.txt
          echo "sha=$(cat sha.txt)" >> "$GITHUB_OUTPUT"
      - name: Request OIDC token
        id: token
        env:
          ACTIONS_ID_TOKEN_REQUEST_URL: ${{ env.ACTIONS_ID_TOKEN_REQUEST_URL }}
          ACTIONS_ID_TOKEN_REQUEST_TOKEN: ${{ env.ACTIONS_ID_TOKEN_REQUEST_TOKEN }}
        run: |
          RESPONSE=$(curl -fsSL -H "Authorization: bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=https://fontpub.org")
          python - <<'PY'
import json,sys
data=json.load(sys.stdin)
print(f"token={data['value']}")
PY <<< "$RESPONSE" >> "$GITHUB_OUTPUT"
      - name: Publish
        env:
          TOKEN: ${{ steps.token.outputs.token }}
        run: |
          curl -fsSL \
            -H "Authorization: Bearer ${TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{\"repository\":\"${GITHUB_REPOSITORY}\",\"sha\":\"${{ steps.sha.outputs.sha }}\",\"ref\":\"${{ steps.ref.outputs.ref }}\"}" \
            %s/v1/update
`, baseURL))
}
