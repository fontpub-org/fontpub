package cli

import (
	"context"
	"fmt"
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
		return a.fail("package", &CLIError{Code: "INPUT_REQUIRED", Message: "unknown package subcommand", Details: map[string]any{"subcommand": args[0]}})
	}
}

func (a *App) runPackageInit(_ context.Context, args []string) int {
	opts, errObj := parsePackageInitOptions(args)
	if errObj != nil {
		return a.fail("package init", errObj)
	}

	manifest, assets, inferences, conflicts, unresolved, err := a.buildCandidateManifest(opts.Root)
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
		conflicts = finalizeConflicts(conflicts, manifest)
		if len(unresolved) > 0 {
			return a.fail("package init", &CLIError{Code: "INPUT_REQUIRED", Message: "required manifest fields could not be resolved", Details: map[string]any{"unresolved_fields": unresolved}})
		}
	}

	if !a.JSON {
		printPackageInitSummary(a.Stdout, opts.Root, manifest, assets, inferences, conflicts, unresolved)
	}

	manifestData, cliErr := structToMap(manifest)
	if cliErr != nil {
		return a.fail("package init", cliErr)
	}
	data := map[string]any{
		"manifest":          manifestData,
		"inferences":        inferenceRecordsToAny(inferences),
		"conflicts":         conflictRecordsToAny(conflicts),
		"unresolved_fields": stringSliceToAny(unresolved),
	}
	if opts.WriteFile {
		target := filepath.Join(opts.Root, "fontpub.json")
		body, err := protocol.MarshalCanonical(manifest)
		if err != nil {
			return a.fail("package init", &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize manifest", Details: map[string]any{"reason": err.Error()}})
		}
		planned, err := writeTrackedFile(target, append(body, '\n'), "write_manifest", "fontpub.json", opts.DryRun, opts.Yes)
		if err != nil {
			return a.fail("package init", asCLIError(err))
		}
		if a.JSON {
			return a.writeValidatedJSONSuccess("package init", data, protocol.ValidatePackageInitResult, "package init output validation failed")
		}
		if opts.DryRun {
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

	if a.JSON {
		return a.writeValidatedJSONSuccess("package init", data, protocol.ValidatePackageInitResult, "package init output validation failed")
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
	opts, errObj := parsePackageValidateOptions(args)
	if errObj != nil {
		return a.fail("package validate", errObj)
	}
	manifest, err := validateManifestRoot(opts.Root)
	if err != nil {
		return a.fail("package validate", asCLIError(err))
	}
	manifestData, cliErr := structToMap(manifest)
	if cliErr != nil {
		return a.fail("package validate", cliErr)
	}
	data := map[string]any{
		"manifest":   manifestData,
		"root_path":  opts.Root,
		"validated":  true,
		"file_count": len(manifest.Files),
	}
	if a.JSON {
		return a.writeJSONSuccess("package validate", data)
	}
	printPackageValidateSummary(a.Stdout, opts.Root, manifest)
	return 0
}

func (a *App) runPackagePreview(_ context.Context, args []string) int {
	opts, errObj := parsePackagePreviewOptions(args)
	if errObj != nil {
		return a.fail("package preview", errObj)
	}
	candidate, err := a.buildCandidatePackageDetail(opts.Root, opts.PackageID)
	if err != nil {
		return a.fail("package preview", asCLIError(err))
	}
	data, cliErr := structToMap(candidate)
	if cliErr != nil {
		return a.fail("package preview", cliErr)
	}
	if a.JSON {
		return a.writeValidatedJSONSuccess("package preview", data, protocol.ValidatePackagePreviewResult, "package preview output validation failed")
	}
	printPackagePreviewSummary(a.Stdout, candidate)
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
	data, cliErr := structToMap(info)
	if cliErr != nil {
		return a.fail("package inspect", cliErr)
	}
	if a.JSON {
		return a.writeJSONSuccess("package inspect", data)
	}
	printInspectionSummary(a.Stdout, info)
	return 0
}

func (a *App) runPackageCheck(_ context.Context, args []string) int {
	opts, errObj := parsePackageCheckOptions(args)
	if errObj != nil {
		return a.fail("package check", errObj)
	}
	manifest, err := validateManifestRoot(opts.Root)
	if err != nil {
		return a.fail("package check", asCLIError(err))
	}
	if err := checkManifestTagMatches(manifest, opts.Tag); err != nil {
		return a.fail("package check", asCLIError(err))
	}
	data := map[string]any{
		"root_path": opts.Root,
		"ready":     true,
	}
	if opts.Tag != "" {
		data["tag"] = opts.Tag
	}
	if a.JSON {
		return a.writeJSONSuccess("package check", data)
	}
	printPackageCheckSummary(a.Stdout, opts.Root, manifest, opts.Tag)
	return 0
}
