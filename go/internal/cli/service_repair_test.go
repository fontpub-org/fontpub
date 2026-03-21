package cli

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestInspectRepairPackageMissingFile(t *testing.T) {
	app := App{}
	pkg := protocol.LockedPackage{
		InstalledVersions: map[string]protocol.InstalledVersion{
			"1.2.3": {
				VersionKey: "1.2.3",
				Assets: []protocol.LockedAsset{{
					Path:      "dist/ExampleSans-Regular.otf",
					SHA256:    strings.Repeat("a", 64),
					LocalPath: filepath.Join(t.TempDir(), "missing.otf"),
				}},
			},
		},
	}
	plan := app.inspectRepairPackage("example/family", pkg, t.TempDir())
	if plan.Result.OK || len(plan.Result.Findings) != 1 || plan.Result.Findings[0].Code != "LOCAL_FILE_MISSING" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestInspectRepairPackageHashMismatch(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "ExampleSans-Regular.otf")
	if err := os.WriteFile(localPath, []byte("actual-font-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	app := App{}
	pkg := protocol.LockedPackage{
		InstalledVersions: map[string]protocol.InstalledVersion{
			"1.2.3": {
				VersionKey: "1.2.3",
				Assets: []protocol.LockedAsset{{
					Path:      "dist/ExampleSans-Regular.otf",
					SHA256:    strings.Repeat("a", 64),
					LocalPath: localPath,
				}},
			},
		},
	}
	plan := app.inspectRepairPackage("example/family", pkg, t.TempDir())
	if plan.Result.OK || len(plan.Result.Findings) != 1 || plan.Result.Findings[0].Code != "LOCAL_FILE_HASH_MISMATCH" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestInspectRepairPackageRepairsWrongSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	activationDir := t.TempDir()
	localPath := filepath.Join(dir, "ExampleSans-Regular.otf")
	wrongTarget := filepath.Join(dir, "Wrong-Regular.otf")
	for _, path := range []string{localPath, wrongTarget} {
		if err := os.WriteFile(path, []byte("font-bytes"), 0o644); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
	}
	sum := sha256.Sum256([]byte("font-bytes"))
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	if err := os.Symlink(wrongTarget, symlinkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}
	active := "1.2.3"
	app := App{}
	pkg := protocol.LockedPackage{
		ActiveVersionKey: &active,
		InstalledVersions: map[string]protocol.InstalledVersion{
			"1.2.3": {
				VersionKey: "1.2.3",
				Assets: []protocol.LockedAsset{{
					Path:        "dist/ExampleSans-Regular.otf",
					SHA256:      fmtHex(sum[:]),
					LocalPath:   localPath,
					Active:      true,
					SymlinkPath: &symlinkPath,
				}},
			},
		},
	}
	plan := app.inspectRepairPackage("example/family", pkg, activationDir)
	if !plan.Result.OK || !plan.Changed || len(plan.Operations) != 1 || plan.Operations[0].Action.Type != "create_symlink" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if plan.Operations[0].LinkPath != symlinkPath {
		t.Fatalf("unexpected link path: %#v", plan.Operations[0])
	}
}

func TestInspectRepairPackageRequiresActivationDirForActiveAssets(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "ExampleSans-Regular.otf")
	if err := os.WriteFile(localPath, []byte("font-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	sum := sha256.Sum256([]byte("font-bytes"))
	active := "1.2.3"
	app := App{}
	pkg := protocol.LockedPackage{
		ActiveVersionKey: &active,
		InstalledVersions: map[string]protocol.InstalledVersion{
			"1.2.3": {
				VersionKey: "1.2.3",
				Assets: []protocol.LockedAsset{{
					Path:      "dist/ExampleSans-Regular.otf",
					SHA256:    fmtHex(sum[:]),
					LocalPath: localPath,
					Active:    true,
				}},
			},
		},
	}
	plan := app.inspectRepairPackage("example/family", pkg, "")
	if plan.Result.OK || len(plan.Result.Findings) != 1 || plan.Result.Findings[0].Code != "INPUT_REQUIRED" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
