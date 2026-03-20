package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

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

	manifest, assets, inferences, conflicts, unresolved, err := a.buildCandidateManifest(root)
	if err != nil {
		return a.fail("package init", asCLIError(err))
	}

	if len(unresolved) > 0 {
		if a.JSON {
			return a.fail("package init", &CLIError{Code: "INPUT_REQUIRED", Message: "required manifest fields could not be inferred", Details: map[string]any{"unresolved_fields": unresolved}})
		}
		if !a.isInteractive() {
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

	if !a.JSON {
		printPackageInitSummary(a.Stdout, root, manifest, assets, inferences, conflicts, unresolved)
	}

	data := map[string]any{
		"manifest":          mustMap(manifest),
		"inferences":        inferenceRecordsToAny(inferences),
		"conflicts":         conflictRecordsToAny(conflicts),
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
		planned := []PlannedAction{{Type: "write_manifest", Path: target}}
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
		if dryRun {
			fmt.Fprintln(a.Stdout, "Manifest write plan")
			fmt.Fprintf(a.Stdout, "  path: %s\n", target)
			fmt.Fprintf(a.Stdout, "  files discovered: %d\n", len(manifest.Files))
			printPlannedActions(a.Stdout, planned)
			return 0
		}
		fmt.Fprintln(a.Stdout, "Wrote fontpub.json")
		fmt.Fprintf(a.Stdout, "  path: %s\n", target)
		fmt.Fprintf(a.Stdout, "  files discovered: %d\n", len(manifest.Files))
		return 0
	}

	fmt.Fprintln(a.Stdout, "Candidate fontpub.json:")
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
	printInspectionSummary(a.Stdout, info)
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
