package cli

import "testing"

func TestParseArgsSupportsMixedFlagOrder(t *testing.T) {
	parsed, err := parseArgs(
		[]string{"example/family", "--activate", "--version=v1.2.3", "--activation-dir", "/tmp/fonts", "--dry-run"},
		[]string{"--dry-run", "--activate"},
		[]string{"--version", "--activation-dir"},
	)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !parsed.boolValue("--dry-run") || !parsed.boolValue("--activate") {
		t.Fatalf("unexpected bool flags: %#v", parsed.bools)
	}
	if parsed.stringValue("--version") != "v1.2.3" || parsed.stringValue("--activation-dir") != "/tmp/fonts" {
		t.Fatalf("unexpected string flags: %#v", parsed.strings)
	}
	if len(parsed.positionals) != 1 || parsed.positionals[0] != "example/family" {
		t.Fatalf("unexpected positionals: %#v", parsed.positionals)
	}
}

func TestParseArgsRejectsUnknownFlag(t *testing.T) {
	_, err := parseArgs([]string{"--bogus"}, []string{"--dry-run"}, []string{"--version"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Code != "INPUT_REQUIRED" || err.Message != "unknown flag" || err.Details["flag"] != "--bogus" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestParseArgsRejectsMissingStringFlagValue(t *testing.T) {
	_, err := parseArgs([]string{"--version"}, nil, []string{"--version"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Code != "INPUT_REQUIRED" || err.Message != "missing flag value" || err.Details["flag"] != "--version" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestParseActivateOptionsAcceptsFlagsInAnyOrder(t *testing.T) {
	opts, err := parseActivateOptions([]string{"--activation-dir", "/tmp/fonts", "example/family", "--version", "v1.3.0", "--dry-run"})
	if err != nil {
		t.Fatalf("parseActivateOptions: %v", err)
	}
	if opts.PackageID != "example/family" || opts.Version != "v1.3.0" || opts.ActivationDir != "/tmp/fonts" || !opts.DryRun {
		t.Fatalf("unexpected options: %#v", opts)
	}
}

func TestParsePackageInitOptionsAcceptsFlagsInAnyOrder(t *testing.T) {
	root := t.TempDir()
	opts, err := parsePackageInitOptions([]string{"--write", root, "--yes", "--dry-run"})
	if err != nil {
		t.Fatalf("parsePackageInitOptions: %v", err)
	}
	if opts.Root != root || !opts.WriteFile || !opts.Yes || !opts.DryRun {
		t.Fatalf("unexpected options: %#v", opts)
	}
}

func TestParsePackagePreviewOptionsSupportsEqualsSyntax(t *testing.T) {
	root := t.TempDir()
	opts, err := parsePackagePreviewOptions([]string{"--package-id=Example/Family", root})
	if err != nil {
		t.Fatalf("parsePackagePreviewOptions: %v", err)
	}
	if opts.Root != root || opts.PackageID != "Example/Family" {
		t.Fatalf("unexpected options: %#v", opts)
	}
}

func TestParseWorkflowInitOptionsRejectsUnknownFlag(t *testing.T) {
	_, err := parseWorkflowInitOptions([]string{"--bogus"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Code != "INPUT_REQUIRED" || err.Message != "unknown flag" {
		t.Fatalf("unexpected error: %#v", err)
	}
}
