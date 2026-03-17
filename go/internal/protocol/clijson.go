package protocol

import "fmt"

var allowedPlannedActionTypes = map[string]struct{}{
	"download_asset":        {},
	"write_asset":           {},
	"remove_asset":          {},
	"create_symlink":        {},
	"remove_symlink":        {},
	"write_lockfile":        {},
	"remove_lockfile_entry": {},
	"write_manifest":        {},
	"write_workflow":        {},
}

var allowedFindingSeverity = map[string]struct{}{
	"error":   {},
	"warning": {},
}

var allowedFindingSubject = map[string]struct{}{
	"package":    {},
	"version":    {},
	"asset":      {},
	"activation": {},
}

func ValidateCLIEnvelope(env CLIEnvelope) error {
	if env.SchemaVersion != "1" {
		return fmt.Errorf("invalid schema_version")
	}
	if env.Command == "" {
		return fmt.Errorf("missing command")
	}
	if env.OK {
		if env.Data == nil {
			return fmt.Errorf("missing data")
		}
		if env.Error != nil {
			return fmt.Errorf("unexpected error")
		}
	} else {
		if env.Error == nil {
			return fmt.Errorf("missing error")
		}
		if env.Data != nil {
			return fmt.Errorf("unexpected data")
		}
	}
	return nil
}

func ValidateStatusResult(env CLIEnvelope) error {
	if err := ValidateCLIEnvelope(env); err != nil {
		return err
	}
	packages, ok := env.Data["packages"].(map[string]any)
	if !ok {
		return fmt.Errorf("missing packages")
	}
	for _, raw := range packages {
		obj, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid package status")
		}
		installed, ok := obj["installed_versions"].([]any)
		if !ok {
			return fmt.Errorf("missing installed_versions")
		}
		valid := map[string]struct{}{}
		for _, item := range installed {
			version, ok := item.(string)
			if !ok {
				return fmt.Errorf("invalid installed version")
			}
			valid[version] = struct{}{}
		}
		if active, ok := obj["active_version_key"]; ok && active != nil {
			version, ok := active.(string)
			if !ok {
				return fmt.Errorf("invalid active_version_key")
			}
			if _, exists := valid[version]; !exists {
				return fmt.Errorf("active_version_key not installed")
			}
		}
	}
	return nil
}

func ValidateVerifyResult(env CLIEnvelope) error {
	if err := ValidateCLIEnvelope(env); err != nil {
		return err
	}
	if env.OK {
		return validatePackageResults(env.Data, "packages")
	}
	if env.Error == nil {
		return fmt.Errorf("missing error")
	}
	return validatePackageResults(env.Error.Details, "packages")
}

func ValidateRepairResult(env CLIEnvelope) error {
	if err := ValidateCLIEnvelope(env); err != nil {
		return err
	}
	if env.OK {
		if _, ok := env.Data["changed"].(bool); !ok {
			return fmt.Errorf("missing changed")
		}
		if _, ok := env.Data["repaired_packages"].([]any); !ok {
			return fmt.Errorf("missing repaired_packages")
		}
		if planned, ok := env.Data["planned_actions"]; ok {
			return validatePlannedActions(planned)
		}
		return nil
	}
	if env.Error == nil {
		return fmt.Errorf("missing error")
	}
	return validatePackageResults(env.Error.Details, "packages")
}

func ValidatePackageInitResult(env CLIEnvelope) error {
	if err := ValidateCLIEnvelope(env); err != nil {
		return err
	}
	if _, ok := env.Data["manifest"].(map[string]any); !ok {
		return fmt.Errorf("missing manifest")
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		return fmt.Errorf("missing inferences")
	}
	for _, inferenceRaw := range inferences {
		inference, ok := inferenceRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid inference")
		}
		if _, ok := inference["field"].(string); !ok {
			return fmt.Errorf("missing inference field")
		}
		source, ok := inference["source"].(string)
		if !ok {
			return fmt.Errorf("missing inference source")
		}
		if source != "embedded_metadata" && source != "filename_heuristic" && source != "user_input" {
			return fmt.Errorf("invalid inference source")
		}
	}
	if _, ok := env.Data["unresolved_fields"].([]any); !ok {
		return fmt.Errorf("missing unresolved_fields")
	}
	return nil
}

func ValidatePackagePreviewResult(env CLIEnvelope) error {
	if err := ValidateCLIEnvelope(env); err != nil {
		return err
	}
	if _, ok := env.Data["published_at"]; ok {
		return fmt.Errorf("published_at must be omitted")
	}
	return nil
}

func validatePackageResults(container map[string]any, key string) error {
	rawPackages, ok := container[key].([]any)
	if !ok {
		return fmt.Errorf("missing %s", key)
	}
	for _, raw := range rawPackages {
		result, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid package result")
		}
		if _, ok := result["package_id"].(string); !ok {
			return fmt.Errorf("missing package_id")
		}
		if _, ok := result["ok"].(bool); !ok {
			return fmt.Errorf("missing ok")
		}
		findings, ok := result["findings"].([]any)
		if !ok {
			return fmt.Errorf("missing findings")
		}
		for _, findingRaw := range findings {
			finding, ok := findingRaw.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid finding")
			}
			if _, ok := finding["code"].(string); !ok {
				return fmt.Errorf("missing finding code")
			}
			severity, ok := finding["severity"].(string)
			if !ok {
				return fmt.Errorf("missing severity")
			}
			if _, ok := allowedFindingSeverity[severity]; !ok {
				return fmt.Errorf("invalid severity")
			}
			subject, ok := finding["subject"].(string)
			if !ok {
				return fmt.Errorf("missing subject")
			}
			if _, ok := allowedFindingSubject[subject]; !ok {
				return fmt.Errorf("invalid subject")
			}
			if _, ok := finding["message"].(string); !ok {
				return fmt.Errorf("missing finding message")
			}
			if _, ok := finding["details"].(map[string]any); !ok {
				return fmt.Errorf("missing finding details")
			}
		}
	}
	return nil
}

func validatePlannedActions(raw any) error {
	actions, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("planned_actions must be array")
	}
	for _, rawAction := range actions {
		action, ok := rawAction.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid planned action")
		}
		actionType, ok := action["type"].(string)
		if !ok {
			return fmt.Errorf("missing planned action type")
		}
		if _, ok := allowedPlannedActionTypes[actionType]; !ok {
			return fmt.Errorf("invalid planned action type")
		}
		if _, ok := action["package_id"].(string); !ok {
			return fmt.Errorf("missing planned action package_id")
		}
	}
	return nil
}
