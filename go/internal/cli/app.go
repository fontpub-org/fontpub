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
  list
  show
  install
  activate
  deactivate
  update
  uninstall
  status
  verify
  repair
  package
  workflow

Examples:
  fontpub list --json
  fontpub show owner/repo --version 1.2.3
  fontpub package init /path/to/repo --write
  fontpub workflow init /path/to/repo --yes
`) + "\n"
	case len(args) == 1 && args[0] == "package":
		return strings.TrimSpace(`
Usage:
  fontpub package <subcommand> [options]

Subcommands:
  init
  validate
  preview
  inspect
  check
`) + "\n"
	case len(args) == 1 && args[0] == "workflow":
		return strings.TrimSpace(`
Usage:
  fontpub workflow <subcommand> [options]

Subcommands:
  init
`) + "\n"
	case len(args) >= 2 && args[0] == "package":
		switch args[1] {
		case "init":
			return "Usage:\n  fontpub package init [PATH] [--write] [--dry-run] [--yes] [--json]\n"
		case "validate":
			return "Usage:\n  fontpub package validate [PATH] [--json]\n"
		case "preview":
			return "Usage:\n  fontpub package preview [PATH] [--package-id <owner>/<repo>] [--json]\n"
		case "inspect":
			return "Usage:\n  fontpub package inspect <font-file> [--json]\n"
		case "check":
			return "Usage:\n  fontpub package check [PATH] [--tag <tag>] [--json]\n"
		}
	case len(args) >= 2 && args[0] == "workflow":
		if args[1] == "init" {
			return "Usage:\n  fontpub workflow init [PATH] [--dry-run] [--yes] [--json]\n"
		}
	default:
		switch args[0] {
		case "list":
			return "Usage:\n  fontpub list [--json]\n"
		case "show":
			return "Usage:\n  fontpub show <owner>/<repo> [--version <v>] [--json]\n"
		case "install":
			return "Usage:\n  fontpub install <owner>/<repo> [--version <v>] [--activate] [--activation-dir <path>] [--dry-run] [--json]\n"
		case "activate":
			return "Usage:\n  fontpub activate <owner>/<repo> [--version <v>] [--activation-dir <path>] [--dry-run] [--json]\n"
		case "deactivate":
			return "Usage:\n  fontpub deactivate <owner>/<repo> [--activation-dir <path>] [--dry-run] [--json]\n"
		case "update":
			return "Usage:\n  fontpub update [<owner>/<repo>] [--activate] [--activation-dir <path>] [--dry-run] [--json]\n"
		case "uninstall":
			return "Usage:\n  fontpub uninstall <owner>/<repo> [--version <v> | --all] [--activation-dir <path>] [--dry-run] [--yes] [--json]\n"
		case "status":
			return "Usage:\n  fontpub status [<owner>/<repo>] [--activation-dir <path>] [--json]\n"
		case "verify":
			return "Usage:\n  fontpub verify [<owner>/<repo>] [--activation-dir <path>] [--json]\n"
		case "repair":
			return "Usage:\n  fontpub repair [<owner>/<repo>] [--activation-dir <path>] [--dry-run] [--yes] [--json]\n"
		}
	}
	return strings.TrimSpace(`
Usage:
  fontpub <command> --help
`) + "\n"
}
