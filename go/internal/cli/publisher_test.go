package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestValidateManifestRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "ExampleSans-Regular.otf"), []byte("font"), 0o644); err != nil {
		t.Fatalf("os.WriteFile font: %v", err)
	}
	manifest := protocol.Manifest{
		Name:    "Example Sans",
		Author:  "Example Studio",
		Version: "1.2.3",
		License: "OFL-1.1",
		Files:   []protocol.ManifestFile{{Path: "dist/ExampleSans-Regular.otf", Style: "normal", Weight: 400}},
	}
	body, _ := protocol.MarshalCanonical(manifest)
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), body, 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	got, err := validateManifestRoot(root)
	if err != nil {
		t.Fatalf("validateManifestRoot: %v", err)
	}
	if got.Name != manifest.Name || got.Version != manifest.Version || len(got.Files) != 1 {
		t.Fatalf("unexpected manifest: %#v", got)
	}
}

func TestValidateManifestRootRejectsMissingDeclaredFile(t *testing.T) {
	root := t.TempDir()
	manifest := protocol.Manifest{
		Name:    "Example Sans",
		Author:  "Example Studio",
		Version: "1.2.3",
		License: "OFL-1.1",
		Files:   []protocol.ManifestFile{{Path: "dist/ExampleSans-Regular.otf", Style: "normal", Weight: 400}},
	}
	body, _ := protocol.MarshalCanonical(manifest)
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), body, 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	_, err := validateManifestRoot(root)
	if err == nil {
		t.Fatalf("expected error")
	}
	cliErr := asCLIError(err)
	if cliErr.Code != "LOCAL_FILE_MISSING" || cliErr.Details["path"] != "dist/ExampleSans-Regular.otf" {
		t.Fatalf("unexpected error: %#v", cliErr)
	}
}

func TestCheckManifestTagMatches(t *testing.T) {
	manifest := protocol.Manifest{Version: "1.2.3"}
	if err := checkManifestTagMatches(manifest, "v1.2.3"); err != nil {
		t.Fatalf("checkManifestTagMatches: %v", err)
	}
	err := checkManifestTagMatches(manifest, "v2.0.0")
	if err == nil {
		t.Fatalf("expected mismatch")
	}
	cliErr := asCLIError(err)
	if cliErr.Code != "TAG_VERSION_MISMATCH" {
		t.Fatalf("unexpected error: %#v", cliErr)
	}
}

func TestWriteTrackedFileDryRun(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".github", "workflows", "fontpub.yml")
	planned, err := writeTrackedFile(target, []byte("body"), "write_workflow", "workflow", true, false)
	if err != nil {
		t.Fatalf("writeTrackedFile: %v", err)
	}
	if len(planned) != 1 || planned[0].Type != "write_workflow" || planned[0].Path != target {
		t.Fatalf("unexpected planned actions: %#v", planned)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected no file, got err=%v", err)
	}
}
