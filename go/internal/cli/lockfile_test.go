package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestLoadLockfileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fontpub.lock")
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "golden", "lockfile.json"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	lock, ok, err := LockfileStore{Path: path}.Load()
	if err != nil || !ok {
		t.Fatalf("Load() ok=%v err=%v", ok, err)
	}
	if lock.SchemaVersion != "1" {
		t.Fatalf("unexpected lockfile: %+v", lock)
	}
}

func TestLoadLockfileMissing(t *testing.T) {
	_, ok, err := LockfileStore{Path: filepath.Join(t.TempDir(), "missing.lock")}.Load()
	if err != nil || ok {
		t.Fatalf("Load() ok=%v err=%v", ok, err)
	}
}

func TestValidateLockfileRejectsBrokenActiveState(t *testing.T) {
	active := "1.2.3"
	err := ValidateLockfile(protocol.Lockfile{
		SchemaVersion: "1",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: &active,
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:    "1.2.3",
						VersionKey: "1.2.3",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: "abc", LocalPath: "/tmp/file", Active: false},
						},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected validation failure")
	}
}

func TestSortedInstalledVersionKeys(t *testing.T) {
	pkg := protocol.LockedPackage{
		InstalledVersions: map[string]protocol.InstalledVersion{
			"1.2.3":  {VersionKey: "1.2.3"},
			"1.10.0": {VersionKey: "1.10.0"},
		},
	}
	got := SortedInstalledVersionKeys(pkg)
	if len(got) != 2 || got[0] != "1.10.0" || got[1] != "1.2.3" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestFinalizeLockMutationNoChange(t *testing.T) {
	app := App{}
	planned, err := app.finalizeLockMutation(protocol.Lockfile{}, false, false, nil, PlannedAction{Type: "write_lockfile"})
	if err != nil {
		t.Fatalf("finalizeLockMutation: %v", err)
	}
	if len(planned) != 0 {
		t.Fatalf("expected no planned actions, got %#v", planned)
	}
}

func TestFinalizeLockMutationDryRunAppendsWriteAction(t *testing.T) {
	app := App{}
	planned, err := app.finalizeLockMutation(protocol.Lockfile{}, true, true, nil, PlannedAction{Type: "write_lockfile", PackageID: "example/family"})
	if err != nil {
		t.Fatalf("finalizeLockMutation: %v", err)
	}
	if len(planned) != 1 || planned[0].Type != "write_lockfile" || planned[0].PackageID != "example/family" {
		t.Fatalf("unexpected planned actions: %#v", planned)
	}
}

func TestFinalizeLockMutationWritesLockfile(t *testing.T) {
	dir := t.TempDir()
	app := App{
		Config: Config{StateDir: dir},
		Now:    func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) },
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "1970-01-01T00:00:00Z",
		Packages:      map[string]protocol.LockedPackage{},
	}
	planned, err := app.finalizeLockMutation(lock, true, false, nil, PlannedAction{Type: "write_lockfile", PackageID: "example/family"})
	if err != nil {
		t.Fatalf("finalizeLockMutation: %v", err)
	}
	if len(planned) != 1 || planned[0].Type != "write_lockfile" {
		t.Fatalf("unexpected planned actions: %#v", planned)
	}
	saved, ok, err := (LockfileStore{Path: filepath.Join(dir, "fontpub.lock")}).Load()
	if err != nil || !ok {
		t.Fatalf("Load lockfile ok=%v err=%v", ok, err)
	}
	if saved.GeneratedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("unexpected generated_at: %q", saved.GeneratedAt)
	}
}
