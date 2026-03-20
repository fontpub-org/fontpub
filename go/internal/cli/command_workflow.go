package cli

import (
	"context"
	"fmt"
	"os"
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
		return a.fail("workflow "+args[0], &CLIError{Code: "INTERNAL_ERROR", Message: "workflow subcommand is not implemented", Details: map[string]any{"command": args[0]}})
	}
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
	planned := []PlannedAction{{Type: "write_workflow", PackageID: "", Path: target}}
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
	if dryRun {
		fmt.Fprintln(a.Stdout, "Workflow write plan")
		fmt.Fprintf(a.Stdout, "  path: %s\n", target)
		printPlannedActions(a.Stdout, planned)
		return 0
	}
	fmt.Fprintln(a.Stdout, "Wrote workflow")
	fmt.Fprintf(a.Stdout, "  path: %s\n", target)
	return 0
}
