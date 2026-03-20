package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) writeMutationResult(command string, changed bool, planned []PlannedAction, extra map[string]any) int {
	data := map[string]any{"changed": changed}
	for k, v := range extra {
		data[k] = v
	}
	if len(planned) > 0 {
		data["planned_actions"] = plannedActionsToAny(planned)
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: command, Data: data})
	}
	if !changed {
		fmt.Fprintln(a.Stdout, "no changes")
		return 0
	}
	fmt.Fprintf(a.Stdout, "%s changed\n", command)
	return 0
}

func (a *App) writePackageFailure(command, message string, results []PackageCheckResult) int {
	firstCode := "INTERNAL_ERROR"
	for _, result := range results {
		if len(result.Findings) > 0 {
			firstCode = result.Findings[0].Code
			break
		}
	}
	if a.JSON {
		_ = a.writeJSON(protocol.CLIEnvelope{
			SchemaVersion: "1",
			OK:            false,
			Command:       command,
			Error: &protocol.ErrorObject{
				Code:    firstCode,
				Message: message,
				Details: packageResultsToDetails(results),
			},
		})
		return 1
	}
	fmt.Fprintln(a.Stderr, message)
	printPackageCheckResults(a.Stderr, "Details:", results)
	return 1
}

func (a *App) fail(command string, err *CLIError) int {
	if err == nil {
		err = &CLIError{Code: "INTERNAL_ERROR", Message: "unknown error", Details: map[string]any{}}
	}
	if a.JSON {
		_ = a.writeJSON(protocol.CLIEnvelope{
			SchemaVersion: "1",
			OK:            false,
			Command:       command,
			Error: &protocol.ErrorObject{
				Code:    err.Code,
				Message: err.Message,
				Details: ensureDetails(err.Details),
			},
		})
		return 1
	}
	printHumanError(a.Stderr, command, err)
	return 1
}

func (a *App) writeJSON(env protocol.CLIEnvelope) int {
	body, err := protocol.MarshalCanonical(env)
	if err != nil {
		fmt.Fprintf(a.Stderr, "INTERNAL_ERROR: %s\n", err.Error())
		return 1
	}
	_, _ = a.Stdout.Write(append(body, '\n'))
	return 0
}

func printPackageDetailSummary(w io.Writer, detail protocol.VersionedPackageDetail) {
	fmt.Fprintf(w, "Package: %s\n", detail.PackageID)
	fmt.Fprintf(w, "Display name: %s\n", detail.DisplayName)
	fmt.Fprintf(w, "Author: %s\n", detail.Author)
	fmt.Fprintf(w, "License: %s\n", detail.License)
	fmt.Fprintf(w, "Version: %s (key %s)\n", detail.Version, detail.VersionKey)
	fmt.Fprintf(w, "Published: %s\n", detail.PublishedAt)
	fmt.Fprintf(w, "GitHub: %s/%s @ %s\n", detail.GitHub.Owner, detail.GitHub.Repo, shortSHA(detail.GitHub.SHA))
	fmt.Fprintf(w, "Manifest: %s\n", detail.ManifestURL)
	fmt.Fprintln(w, "Assets:")
	for _, asset := range detail.Assets {
		fmt.Fprintf(
			w,
			"  - %s [%s] path=%s style=%s weight=%d size=%d\n",
			filepath.Base(filepath.FromSlash(asset.Path)),
			asset.Format,
			asset.Path,
			asset.Style,
			asset.Weight,
			asset.SizeBytes,
		)
	}
}

func shortSHA(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func printPackageCheckResults(w io.Writer, header string, results []PackageCheckResult) {
	if header != "" {
		fmt.Fprintln(w, header)
	}
	for _, result := range results {
		if result.OK {
			fmt.Fprintf(w, "  %s: ok\n", result.PackageID)
			continue
		}
		fmt.Fprintf(w, "  %s: failed\n", result.PackageID)
		for _, finding := range result.Findings {
			fmt.Fprintf(w, "    - [%s/%s] %s", finding.Severity, finding.Subject, finding.Message)
			if path, ok := finding.Details["path"].(string); ok && path != "" {
				fmt.Fprintf(w, " (%s)", path)
			}
			fmt.Fprintln(w)
			printFindingDetailLines(w, finding.Details)
		}
	}
}

func printNextHints(w io.Writer, hints ...string) {
	if len(hints) == 0 {
		return
	}
	fmt.Fprintln(w, "Next:")
	for _, hint := range hints {
		fmt.Fprintf(w, "  %s\n", hint)
	}
}

func printNoPublishedPackages(w io.Writer) {
	fmt.Fprintln(w, "no published packages")
	printNextHints(w, "check FONTPUB_BASE_URL or publish package metadata to the service")
}

func printNoInstalledPackages(w io.Writer) {
	fmt.Fprintln(w, "no installed packages")
	printNextHints(w, "run: fontpub ls-remote", "run: fontpub install <owner>/<repo>")
}

func printFindingDetailLines(w io.Writer, details map[string]any) {
	if len(details) == 0 {
		return
	}
	for _, key := range []string{"local_path", "symlink_path", "reason", "status", "url"} {
		value, ok := details[key]
		if !ok {
			continue
		}
		text := formatHumanDetailValue(value)
		if text == "" {
			continue
		}
		fmt.Fprintf(w, "      %s: %s\n", key, text)
	}
}

func printPlannedActions(w io.Writer, planned []PlannedAction) {
	if len(planned) == 0 {
		return
	}
	fmt.Fprintln(w, "Planned actions:")
	for _, action := range planned {
		fmt.Fprintf(w, "  - %s", humanizePlannedAction(action))
		if action.PackageID != "" {
			fmt.Fprintf(w, " [%s", action.PackageID)
			if action.VersionKey != "" {
				fmt.Fprintf(w, "@%s", action.VersionKey)
			}
			fmt.Fprint(w, "]")
		}
		if action.Path != "" {
			fmt.Fprintf(w, " %s", action.Path)
		}
		fmt.Fprintln(w)
	}
}

func humanizePlannedAction(action PlannedAction) string {
	switch action.Type {
	case "download_asset":
		return "download asset"
	case "write_asset":
		return "write asset"
	case "remove_asset":
		return "remove asset"
	case "create_symlink":
		return "create symlink"
	case "remove_symlink":
		return "remove symlink"
	case "write_lockfile":
		return "write lockfile"
	case "remove_lockfile_entry":
		return "remove lockfile entry"
	case "write_manifest":
		return "write manifest"
	case "write_workflow":
		return "write workflow"
	default:
		return action.Type
	}
}
