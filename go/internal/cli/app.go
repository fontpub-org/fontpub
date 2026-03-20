package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"time"
)

type App struct {
	Config  Config
	Client  *MetadataClient
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
	IsTTY   func() bool
	JSON    bool
	Command string
	Now     func() time.Time
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	app := App{
		Config: DefaultConfig(),
		Stdin:  os.Stdin,
		Stdout: stdout,
		Stderr: stderr,
	}
	return app.Run(ctx, args)
}

func (a *App) Run(ctx context.Context, args []string) int {
	a.JSON = false
	a.Command = ""
	helpRequested := false
	rest := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			a.JSON = true
			continue
		}
		if arg == "--help" {
			helpRequested = true
			continue
		}
		rest = append(rest, arg)
	}
	if helpRequested {
		return a.writeHelp(rest)
	}
	if len(rest) == 0 {
		return a.fail("", &CLIError{Code: "INPUT_REQUIRED", Message: "command is required", Details: map[string]any{}})
	}
	a.Command = strings.Join(commandPath(rest), " ")
	if a.Client == nil {
		a.Client = NewMetadataClient(a.Config)
	}

	switch rest[0] {
	case "package":
		return a.runPackage(ctx, rest[1:])
	case "list":
		return a.runList(ctx, rest[1:])
	case "show":
		return a.runShow(ctx, rest[1:])
	case "install":
		return a.runInstall(ctx, rest[1:])
	case "activate":
		return a.runActivate(ctx, rest[1:])
	case "deactivate":
		return a.runDeactivate(ctx, rest[1:])
	case "status":
		return a.runStatus(ctx, rest[1:])
	case "verify":
		return a.runVerify(ctx, rest[1:])
	case "repair":
		return a.runRepair(ctx, rest[1:])
	case "uninstall":
		return a.runUninstall(ctx, rest[1:])
	case "update":
		return a.runUpdate(ctx, rest[1:])
	case "workflow":
		return a.runWorkflow(ctx, rest[1:])
	default:
		return a.fail(a.Command, &CLIError{Code: "INTERNAL_ERROR", Message: "command is not implemented", Details: map[string]any{"command": rest[0]}})
	}
}

func plannedActionsToAny(actions []PlannedAction) []any {
	out := make([]any, 0, len(actions))
	for _, action := range actions {
		item := map[string]any{
			"type":       action.Type,
			"package_id": action.PackageID,
		}
		if action.VersionKey != "" {
			item["version_key"] = action.VersionKey
		}
		if action.Path != "" {
			item["path"] = action.Path
		}
		out = append(out, item)
	}
	return out
}

func Main() {
	os.Exit(Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func (a *App) isInteractive() bool {
	if a.IsTTY != nil {
		return a.IsTTY()
	}
	if a.Stdin == nil {
		return false
	}
	file, ok := a.Stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (a *App) writeHelp(args []string) int {
	_, _ = io.WriteString(a.Stdout, helpText(args))
	return 0
}

func helpText(args []string) string {
	switch {
	case len(args) == 0:
		return strings.TrimSpace(`
Usage:
  fontpub <command> [options]

Commands:
  list       List published packages
  show       Show package metadata and assets
  install    Install a package version locally
  activate   Activate an installed version with symlinks
  deactivate Remove activation symlinks for a package
  update     Install newer published versions
  uninstall  Remove installed package files
  status     Show installed versions and activation state
  verify     Check local files and activation symlinks
  repair     Reconcile lockfile and activation state
  package    Publisher manifest commands
  workflow   Publisher workflow generation

Examples:
  fontpub list --json
  fontpub show owner/repo --version 1.2.3
  fontpub install owner/repo --activate --activation-dir ~/Library/Fonts/Fontpub
  fontpub package init /path/to/repo --write
  fontpub workflow init /path/to/repo --yes

Environment:
  FONTPUB_BASE_URL         Override the metadata service base URL
  FONTPUB_STATE_DIR        Override the local state directory
  FONTPUB_ACTIVATION_DIR   Default activation directory for activation commands
`) + "\n"
	case len(args) == 1 && args[0] == "package":
		return strings.TrimSpace(`
Usage:
  fontpub package <subcommand> [options]

Subcommands:
  init      Scan a repository and build a candidate fontpub.json
  validate  Validate fontpub.json and referenced files
  preview   Render the candidate package detail document
  inspect   Inspect a single font file for inferred metadata
  check     Validate readiness for publication

Examples:
  fontpub package init /path/to/repo --write
  fontpub package validate /path/to/repo --json
  fontpub package preview /path/to/repo --package-id owner/repo --json
`) + "\n"
	case len(args) == 1 && args[0] == "workflow":
		return strings.TrimSpace(`
Usage:
  fontpub workflow <subcommand> [options]

Subcommands:
  init   Generate a starter GitHub Actions workflow

Examples:
  fontpub workflow init /path/to/repo --dry-run
  fontpub workflow init /path/to/repo --yes
`) + "\n"
	case len(args) >= 2 && args[0] == "package":
		switch args[1] {
		case "init":
			return strings.TrimSpace(`
Usage:
  fontpub package init [PATH] [--write] [--dry-run] [--yes] [--json]

Build a candidate fontpub.json from the selected repository root.

Examples:
  fontpub package init /path/to/repo
  fontpub package init /path/to/repo --write --yes
`) + "\n"
		case "validate":
			return strings.TrimSpace(`
Usage:
  fontpub package validate [PATH] [--json]

Validate fontpub.json and verify all declared files exist.
`) + "\n"
		case "preview":
			return strings.TrimSpace(`
Usage:
  fontpub package preview [PATH] [--package-id <owner>/<repo>] [--json]

Render the candidate package detail document without publishing.
`) + "\n"
		case "inspect":
			return strings.TrimSpace(`
Usage:
  fontpub package inspect <font-file> [--json]

Inspect a font file and print inferred metadata.
`) + "\n"
		case "check":
			return strings.TrimSpace(`
Usage:
  fontpub package check [PATH] [--tag <tag>] [--json]

Validate publication readiness for the selected repository.
`) + "\n"
		}
	case len(args) >= 2 && args[0] == "workflow":
		if args[1] == "init" {
			return strings.TrimSpace(`
Usage:
  fontpub workflow init [PATH] [--dry-run] [--yes] [--json]

Generate a starter .github/workflows/fontpub.yml file.
`) + "\n"
		}
	default:
		switch args[0] {
		case "list":
			return strings.TrimSpace(`
Usage:
  fontpub list [--json]

List published packages and their latest versions.
`) + "\n"
		case "show":
			return strings.TrimSpace(`
Usage:
  fontpub show <owner>/<repo> [--version <v>] [--json]

Show package metadata and asset details.
`) + "\n"
		case "install":
			return strings.TrimSpace(`
Usage:
  fontpub install <owner>/<repo> [--version <v>] [--activate] [--activation-dir <path>] [--dry-run] [--json]

Install the latest or selected version into local state.
`) + "\n"
		case "activate":
			return strings.TrimSpace(`
Usage:
  fontpub activate <owner>/<repo> [--version <v>] [--activation-dir <path>] [--dry-run] [--json]

Create activation symlinks for an installed version.
`) + "\n"
		case "deactivate":
			return strings.TrimSpace(`
Usage:
  fontpub deactivate <owner>/<repo> [--activation-dir <path>] [--dry-run] [--json]

Remove activation symlinks for the package.
`) + "\n"
		case "update":
			return strings.TrimSpace(`
Usage:
  fontpub update [<owner>/<repo>] [--activate] [--activation-dir <path>] [--dry-run] [--json]

Install newer published versions for installed packages.
`) + "\n"
		case "uninstall":
			return strings.TrimSpace(`
Usage:
  fontpub uninstall <owner>/<repo> [--version <v> | --all] [--activation-dir <path>] [--dry-run] [--yes] [--json]

Remove installed package files and lockfile entries.
`) + "\n"
		case "status":
			return strings.TrimSpace(`
Usage:
  fontpub status [<owner>/<repo>] [--activation-dir <path>] [--json]

Show installed versions and activation state for the selected directory.
`) + "\n"
		case "verify":
			return strings.TrimSpace(`
Usage:
  fontpub verify [<owner>/<repo>] [--activation-dir <path>] [--json]

Verify installed files and activation symlinks.
`) + "\n"
		case "repair":
			return strings.TrimSpace(`
Usage:
  fontpub repair [<owner>/<repo>] [--activation-dir <path>] [--dry-run] [--yes] [--json]

Reconcile lockfile and activation state without downloading.
`) + "\n"
		}
	}
	return strings.TrimSpace(`
Usage:
  fontpub <command> --help
`) + "\n"
}
