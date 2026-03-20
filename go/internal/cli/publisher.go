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
	if !a.isInteractive() || a.Stdin == nil {
		return &CLIError{Code: "INPUT_REQUIRED", Message: "required manifest fields could not be inferred", Details: map[string]any{"unresolved_fields": unresolved}}
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
