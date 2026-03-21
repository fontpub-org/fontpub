package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func (a *App) runWorkflow(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return a.fail("workflow", &CLIError{Code: "INPUT_REQUIRED", Message: "workflow subcommand is required", Details: map[string]any{}})
	}
	switch args[0] {
	case "init":
		return a.runWorkflowInit(ctx, args[1:])
	default:
		return a.fail("workflow", &CLIError{Code: "INPUT_REQUIRED", Message: "unknown workflow subcommand", Details: map[string]any{"subcommand": args[0]}})
	}
}

func (a *App) runWorkflowInit(_ context.Context, args []string) int {
	opts, errObj := parseWorkflowInitOptions(args)
	if errObj != nil {
		return a.fail("workflow init", errObj)
	}
	target := filepath.Join(opts.Root, ".github", "workflows", "fontpub.yml")
	body := []byte(generatedWorkflowYAML(a.Config.BaseURL))
	planned, err := writeTrackedFile(target, body, "write_workflow", "workflow", opts.DryRun, opts.Yes)
	if err != nil {
		return a.fail("workflow init", asCLIError(err))
	}
	if a.JSON {
		data := map[string]any{"changed": true}
		if opts.DryRun {
			data["planned_actions"] = plannedActionsToAny(planned)
		}
		return a.writeJSON(protocol.CLIEnvelope{SchemaVersion: "1", OK: true, Command: "workflow init", Data: data})
	}
	if opts.DryRun {
		fmt.Fprintln(a.Stdout, "Workflow write plan")
		fmt.Fprintf(a.Stdout, "  path: %s\n", target)
		printPlannedActions(a.Stdout, planned)
		return 0
	}
	fmt.Fprintln(a.Stdout, "Wrote workflow")
	fmt.Fprintf(a.Stdout, "  path: %s\n", target)
	return 0
}
