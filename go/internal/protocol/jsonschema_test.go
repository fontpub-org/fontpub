package protocol

import "testing"

func TestValidateCLISchemaAcceptsValidEnvelope(t *testing.T) {
	env := CLIEnvelope{
		SchemaVersion: "1",
		OK:            true,
		Command:       "ls-remote",
		Data: map[string]any{
			"packages": []any{
				map[string]any{
					"package_id":          "example/family",
					"latest_version":      "1.2.3",
					"latest_version_key":  "1.2.3",
					"latest_published_at": "2026-01-02T00:00:00Z",
				},
			},
		},
	}
	if err := ValidateCLISchema("ls-remote-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema: %v", err)
	}
}

func TestValidateCLISchemaRejectsAdditionalProperties(t *testing.T) {
	env := CLIEnvelope{
		SchemaVersion: "1",
		OK:            true,
		Command:       "ls",
		Data: map[string]any{
			"packages": map[string]any{
				"example/family": map[string]any{
					"installed_versions": []any{"1.2.3"},
					"active_version_key": "1.2.3",
					"unexpected":         true,
				},
			},
		},
	}
	if err := ValidateCLISchema("ls-result.schema.json", env); err == nil {
		t.Fatalf("expected schema rejection")
	}
}

func TestSchemaFileNameForCLICommand(t *testing.T) {
	tests := map[string]string{
		"ls-remote":       "ls-remote-result.schema.json",
		"ls":              "ls-result.schema.json",
		"verify":          "verify-result.schema.json",
		"repair":          "repair-result.schema.json",
		"package init":    "package-init-result.schema.json",
		"package preview": "package-preview-result.schema.json",
		"show":            "",
	}
	for command, want := range tests {
		if got := schemaFileNameForCLICommand(command); got != want {
			t.Fatalf("schemaFileNameForCLICommand(%q)=%q want %q", command, got, want)
		}
	}
}
