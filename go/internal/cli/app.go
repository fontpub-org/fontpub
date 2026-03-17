package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type CLIError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *CLIError) Error() string {
	return e.Code + ": " + e.Message
}

type App struct {
	Config  Config
	Client  *MetadataClient
	Stdout  io.Writer
	Stderr  io.Writer
	JSON    bool
	Command string
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	app := App{
		Config: DefaultConfig(),
		Stdout: stdout,
		Stderr: stderr,
	}
	return app.Run(ctx, args)
}

func (a *App) Run(ctx context.Context, args []string) int {
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			a.JSON = true
			continue
		}
		rest = append(rest, arg)
	}
	if len(rest) == 0 {
		return a.fail("", &CLIError{Code: "INPUT_REQUIRED", Message: "command is required", Details: map[string]any{}})
	}
	a.Command = strings.Join(commandPath(rest), " ")
	if a.Client == nil {
		a.Client = NewMetadataClient(a.Config)
	}

	switch rest[0] {
	case "list":
		return a.runList(ctx, rest[1:])
	case "show":
		return a.runShow(ctx, rest[1:])
	case "status":
		return a.runStatus(ctx, rest[1:])
	default:
		return a.fail(a.Command, &CLIError{Code: "INTERNAL_ERROR", Message: "command is not implemented", Details: map[string]any{"command": rest[0]}})
	}
}

func (a *App) runList(ctx context.Context, args []string) int {
	if len(args) != 0 {
		return a.fail("list", &CLIError{Code: "INPUT_REQUIRED", Message: "list does not accept positional arguments", Details: map[string]any{}})
	}
	root, err := a.Client.GetRootIndex(ctx)
	if err != nil {
		return a.fail("list", asCLIError(err))
	}
	packageIDs := make([]string, 0, len(root.Packages))
	for packageID := range root.Packages {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	packages := make([]map[string]any, 0, len(packageIDs))
	for _, packageID := range packageIDs {
		entry := root.Packages[packageID]
		packages = append(packages, map[string]any{
			"package_id":          packageID,
			"latest_version":      entry.LatestVersion,
			"latest_version_key":  entry.LatestVersionKey,
			"latest_published_at": entry.LatestPublishedAt,
		})
	}
	data := map[string]any{"packages": packages}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "list", Data: data})
	}
	for _, pkg := range packages {
		fmt.Fprintf(a.Stdout, "%s %s\n", pkg["package_id"], pkg["latest_version"])
	}
	return 0
}

func (a *App) runShow(ctx context.Context, args []string) int {
	version, rest, errObj := extractStringFlag(args, "--version")
	if errObj != nil {
		return a.fail("show", errObj)
	}
	if len(rest) != 1 {
		return a.fail("show", &CLIError{Code: "INPUT_REQUIRED", Message: "show requires <owner>/<repo>", Details: map[string]any{}})
	}
	packageID := normalizePackageID(rest[0])
	var (
		detail protocol.VersionedPackageDetail
		err    error
	)
	if version == "" {
		detail, err = a.Client.GetLatestPackageDetail(ctx, packageID)
	} else {
		detail, err = a.Client.GetPackageDetailVersion(ctx, packageID, version)
	}
	if err != nil {
		return a.fail("show", asCLIError(err))
	}
	data, cliErr := structToMap(detail)
	if cliErr != nil {
		return a.fail("show", cliErr)
	}
	if a.JSON {
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "show", Data: data})
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", detail.PackageID, detail.Version)
	fmt.Fprintf(a.Stdout, "name: %s\n", detail.DisplayName)
	fmt.Fprintf(a.Stdout, "author: %s\n", detail.Author)
	fmt.Fprintf(a.Stdout, "license: %s\n", detail.License)
	for _, asset := range detail.Assets {
		fmt.Fprintf(a.Stdout, "asset: %s %s %d\n", asset.Path, asset.Style, asset.Weight)
	}
	return 0
}

func (a *App) runStatus(_ context.Context, args []string) int {
	_, rest, errObj := extractStringFlag(args, "--activation-dir")
	if errObj != nil {
		return a.fail("status", errObj)
	}
	var target string
	if len(rest) > 1 {
		return a.fail("status", &CLIError{Code: "INPUT_REQUIRED", Message: "status accepts at most one package id", Details: map[string]any{}})
	}
	if len(rest) == 1 {
		target = normalizePackageID(rest[0])
	}

	lock, ok, err := LockfileStore{Path: a.Config.LockfilePath()}.Load()
	if err != nil {
		return a.fail("status", asCLIError(err))
	}
	packagesData := map[string]any{}
	if ok {
		packageIDs := make([]string, 0, len(lock.Packages))
		for packageID := range lock.Packages {
			packageIDs = append(packageIDs, packageID)
		}
		sort.Strings(packageIDs)
		for _, packageID := range packageIDs {
			if target != "" && packageID != target {
				continue
			}
			pkg := lock.Packages[packageID]
			installed := make([]any, 0, len(pkg.InstalledVersions))
			for _, versionKey := range SortedInstalledVersionKeys(pkg) {
				installed = append(installed, versionKey)
			}
			var active any
			if pkg.ActiveVersionKey != nil {
				active = *pkg.ActiveVersionKey
			}
			packagesData[packageID] = map[string]any{
				"installed_versions": installed,
				"active_version_key": active,
			}
		}
	}
	if target != "" {
		if _, exists := packagesData[target]; !exists {
			return a.fail("status", &CLIError{Code: "NOT_INSTALLED", Message: "package is not installed", Details: map[string]any{"package_id": target}})
		}
	}

	data := map[string]any{"packages": packagesData}
	if a.JSON {
		env := protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "status", Data: data}
		if err := protocol.ValidateStatusResult(env); err != nil {
			return a.fail("status", &CLIError{Code: "INTERNAL_ERROR", Message: "status output validation failed", Details: map[string]any{"reason": err.Error()}})
		}
		return a.writeJSON(env)
	}
	if len(packagesData) == 0 {
		fmt.Fprintln(a.Stdout, "no installed packages")
		return 0
	}
	packageIDs := make([]string, 0, len(packagesData))
	for packageID := range packagesData {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	for _, packageID := range packageIDs {
		entry := packagesData[packageID].(map[string]any)
		active := "inactive"
		if entry["active_version_key"] != nil {
			active = entry["active_version_key"].(string)
		}
		versions := entry["installed_versions"].([]any)
		versionTexts := make([]string, 0, len(versions))
		for _, version := range versions {
			versionTexts = append(versionTexts, version.(string))
		}
		fmt.Fprintf(a.Stdout, "%s installed=%s active=%s\n", packageID, strings.Join(versionTexts, ","), active)
	}
	return 0
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
	fmt.Fprintf(a.Stderr, "%s: %s\n", err.Code, err.Message)
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

func asCLIError(err error) *CLIError {
	var cliErr *CLIError
	if err != nil && errorAs(err, &cliErr) {
		return cliErr
	}
	return &CLIError{Code: "INTERNAL_ERROR", Message: err.Error(), Details: map[string]any{}}
}

func structToMap(value any) (map[string]any, *CLIError) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize output", Details: map[string]any{}}
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not build JSON output", Details: map[string]any{}}
	}
	return out, nil
}

func normalizePackageID(packageID string) string {
	return strings.ToLower(packageID)
}

func commandPath(args []string) []string {
	if len(args) >= 2 && args[0] == "package" {
		return args[:2]
	}
	if len(args) > 0 {
		return args[:1]
	}
	return nil
}

func ensureDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}

func extractStringFlag(args []string, name string) (string, []string, *CLIError) {
	rest := make([]string, 0, len(args))
	var value string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == name:
			if i+1 >= len(args) {
				return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "missing flag value", Details: map[string]any{"flag": name}}
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(arg, name+"="):
			value = strings.TrimPrefix(arg, name+"=")
		case strings.HasPrefix(arg, "--"):
			return "", nil, &CLIError{Code: "INPUT_REQUIRED", Message: "unknown flag", Details: map[string]any{"flag": arg}}
		default:
			rest = append(rest, arg)
		}
	}
	return value, rest, nil
}

func errorAs(err error, target **CLIError) bool {
	typed, ok := err.(*CLIError)
	if !ok {
		return false
	}
	*target = typed
	return true
}

func Main() {
	os.Exit(Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
