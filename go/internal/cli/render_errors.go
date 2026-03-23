package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

var humanErrorDetailOrder = []string{
	"path",
	"package_id",
	"current_activation_dir",
	"requested_activation_dir",
	"version_key",
	"version",
	"flag",
	"command",
	"subcommand",
	"status",
	"url",
	"symlink_path",
	"local_path",
	"tag",
	"manifest_version",
	"reason",
}

func printHumanError(w io.Writer, command string, err *CLIError) {
	fmt.Fprintf(w, "%s: %s\n", err.Code, err.Message)
	printHumanErrorDetails(w, ensureDetails(err.Details))
	hints := humanErrorHints(command, err)
	if len(hints) == 0 {
		return
	}
	fmt.Fprintln(w, "Next:")
	for _, hint := range hints {
		fmt.Fprintf(w, "  %s\n", hint)
	}
}

func printHumanErrorDetails(w io.Writer, details map[string]any) {
	if len(details) == 0 {
		return
	}
	seen := map[string]struct{}{}
	for _, key := range humanErrorDetailOrder {
		value, ok := details[key]
		if !ok {
			continue
		}
		text := formatHumanDetailValue(value)
		if text == "" {
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", key, text)
		seen[key] = struct{}{}
	}
	extraKeys := make([]string, 0, len(details))
	for key := range details {
		if _, ok := seen[key]; ok {
			continue
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		text := formatHumanDetailValue(details[key])
		if text == "" {
			continue
		}
		fmt.Fprintf(w, "  %s: %s\n", key, text)
	}
}

func formatHumanDetailValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []string:
		return strings.Join(v, ", ")
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			text := formatHumanDetailValue(item)
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func humanErrorHints(command string, err *CLIError) []string {
	details := ensureDetails(err.Details)
	switch err.Code {
	case "INPUT_REQUIRED":
		return inputRequiredHints(command, err, details)
	case "LOCAL_FILE_MISSING":
		return localFileMissingHints(command, details)
	case "NOT_INSTALLED":
		if packageID := stringValue(details["package_id"]); packageID != "" {
			return []string{fmt.Sprintf("run: fontpub install %s", packageID)}
		}
		return nil
	case "MULTIPLE_VERSIONS_INSTALLED":
		if command == "uninstall" {
			return []string{
				"rerun with --version <v> to remove one version",
				"or rerun with --all to remove all installed versions",
			}
		}
		return []string{"rerun with --version <v> to choose the installed version"}
	case "PACKAGE_ID_REQUIRED", "PACKAGE_ID_AMBIGUOUS":
		if command == "package preview" {
			return []string{"rerun with --package-id <owner>/<repo>"}
		}
		return nil
	case "VERSION_INVALID":
		return []string{"use a version like 1.2.3 or v1.2.3"}
	case "INTERNAL_ERROR":
		if isNetworkFailure(err, details) {
			return []string{"check FONTPUB_BASE_URL or network connectivity"}
		}
	}
	return nil
}

func inputRequiredHints(command string, err *CLIError, details map[string]any) []string {
	flag := stringValue(details["flag"])
	switch {
	case strings.Contains(err.Message, "activation directory is required"):
		return []string{"pass --activation-dir <path> or set FONTPUB_ACTIVATION_DIR"}
	case err.Message == "package is active in a different activation directory":
		packageID := stringValue(details["package_id"])
		current := stringValue(details["current_activation_dir"])
		if packageID != "" && current != "" {
			return []string{
				fmt.Sprintf("run: fontpub deactivate %s --activation-dir %s", packageID, current),
				fmt.Sprintf("or rerun with --activation-dir %s", current),
			}
		}
		return nil
	case err.Message == "command is required":
		return []string{"run: fontpub --help"}
	case err.Message == "unknown command":
		return []string{"run: fontpub --help"}
	case err.Message == "package subcommand is required":
		return []string{"run: fontpub package --help"}
	case err.Message == "workflow subcommand is required":
		return []string{"run: fontpub workflow --help"}
	case err.Message == "unknown package subcommand":
		return []string{"run: fontpub package --help"}
	case err.Message == "unknown workflow subcommand":
		return []string{"run: fontpub workflow --help"}
	case err.Message == "unknown flag" || err.Message == "missing flag value":
		if command == "" {
			return []string{"run: fontpub --help"}
		}
		return []string{fmt.Sprintf("run: fontpub %s --help", command)}
	case command == "package init" && strings.Contains(err.Message, "required manifest fields"):
		return []string{"rerun interactively or edit fontpub.json to fill the missing fields"}
	case flag != "":
		return []string{fmt.Sprintf("run: fontpub %s --help", command)}
	default:
		return nil
	}
}

func localFileMissingHints(command string, details map[string]any) []string {
	path := stringValue(details["path"])
	if path != "" && filepath.Base(path) == "fontpub.json" {
		root := filepath.Dir(path)
		switch command {
		case "package validate", "package check", "package preview":
			return []string{fmt.Sprintf("run: fontpub package init %s --write", root)}
		}
	}
	return nil
}

func isNetworkFailure(err *CLIError, details map[string]any) bool {
	if stringValue(details["url"]) != "" || stringValue(details["status"]) != "" {
		return true
	}
	if path := stringValue(details["path"]); path != "" && strings.HasPrefix(path, "/v1/") {
		return true
	}
	return strings.Contains(err.Message, "request failed") || strings.Contains(err.Message, "download failed")
}
