package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestRunLSRemoteJSON(t *testing.T) {
	client := &MetadataClient{
		BaseURL:   "https://fontpub.org",
		UserAgent: "test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/v1/index.json" {
					return jsonResponse(http.StatusNotFound, protocol.ErrorEnvelope{Error: protocol.ErrorObject{Code: "PACKAGE_NOT_FOUND", Message: "not found", Details: map[string]any{}}}), nil
				}
				return jsonResponse(http.StatusOK, protocol.RootIndex{
					SchemaVersion: "1",
					GeneratedAt:   "2026-01-02T00:00:00Z",
					Packages: map[string]protocol.RootIndexPackage{
						"example/family": {LatestVersion: "1.2.3", LatestVersionKey: "1.2.3", LatestPublishedAt: "2026-01-02T00:00:00Z"},
					},
				}), nil
			}),
		},
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls-remote", "--json"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}

	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidateCLIEnvelope(env); err != nil {
		t.Fatalf("ValidateCLIEnvelope: %v", err)
	}
	if err := protocol.ValidateCLISchema("ls-remote-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(ls-remote): %v", err)
	}
	if env.Command != "ls-remote" || !env.OK {
		t.Fatalf("unexpected env: %+v", env)
	}
	packages, ok := env.Data["packages"].([]any)
	if !ok || len(packages) != 1 {
		t.Fatalf("unexpected packages: %#v", env.Data["packages"])
	}
}

func TestRunWithoutCommandPrintsNextStep(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), nil); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"INPUT_REQUIRED: command is required\n",
		"Next:\n",
		"  run: fontpub --help\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestRunLSRemoteHumanReadable(t *testing.T) {
	client := &MetadataClient{
		BaseURL:   "https://fontpub.org",
		UserAgent: "test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, protocol.RootIndex{
					SchemaVersion: "1",
					GeneratedAt:   "2026-01-02T00:00:00Z",
					Packages: map[string]protocol.RootIndexPackage{
						"example/family": {LatestVersion: "1.2.3", LatestVersionKey: "1.2.3", LatestPublishedAt: "2026-01-02T00:00:00Z"},
					},
				}), nil
			}),
		},
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls-remote"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Available packages:\n",
		"  - example/family  latest 1.2.3",
		"published 2026-01-02\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("ls-remote output missing %q\n%s", want, output)
		}
	}
}

func TestUnknownFlagPrintsHelpHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "--bogus"}); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"INPUT_REQUIRED: unknown flag\n",
		"  flag: --bogus\n",
		"Next:\n",
		"  run: fontpub ls --help\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestUnknownCommandPrintsHelpHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"list"}); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"INPUT_REQUIRED: unknown command\n",
		"  command: list\n",
		"Next:\n",
		"  run: fontpub --help\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestRunShowJSONLatestAndVersion(t *testing.T) {
	client := &MetadataClient{
		BaseURL:   "https://fontpub.org",
		UserAgent: "test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/v1/packages/example/family.json", "/v1/packages/example/family/versions/1.2.3.json":
					return jsonResponse(http.StatusOK, protocol.VersionedPackageDetail{
						SchemaVersion: "1",
						PackageID:     "example/family",
						DisplayName:   "Example Sans",
						Author:        "Example Studio",
						License:       "OFL-1.1",
						Version:       "1.2.3",
						VersionKey:    "1.2.3",
						PublishedAt:   "2026-01-02T00:00:00Z",
						GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
						ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
						Assets: []protocol.VersionedAsset{
							{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.example/dist/ExampleSans-Regular.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11},
						},
					}), nil
				default:
					return jsonResponse(http.StatusNotFound, protocol.ErrorEnvelope{Error: protocol.ErrorObject{Code: "PACKAGE_NOT_FOUND", Message: "not found", Details: map[string]any{}}}), nil
				}
			}),
		},
	}

	for _, args := range [][]string{
		{"show", "example/family", "--json"},
		{"show", "example/family", "--version", "v1.2.3", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		app := App{
			Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()},
			Client: client,
			Stdout: &stdout,
			Stderr: &stderr,
		}
		if code := app.Run(context.Background(), args); code != 0 {
			t.Fatalf("Run(%v) code=%d stderr=%s", args, code, stderr.String())
		}
		var env protocol.CLIEnvelope
		if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if env.Command != "show" || !env.OK {
			t.Fatalf("unexpected env: %+v", env)
		}
		if env.Data["package_id"] != "example/family" || env.Data["version_key"] != "1.2.3" {
			t.Fatalf("unexpected show data: %#v", env.Data)
		}
	}
}

func TestRunShowHumanReadable(t *testing.T) {
	client := &MetadataClient{
		BaseURL:   "https://fontpub.org",
		UserAgent: "test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, protocol.VersionedPackageDetail{
					SchemaVersion: "1",
					PackageID:     "example/family",
					DisplayName:   "Example Sans",
					Author:        "Example Studio",
					License:       "OFL-1.1",
					Version:       "1.2.3",
					VersionKey:    "1.2.3",
					PublishedAt:   "2026-01-02T00:00:00Z",
					GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
					ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
					Assets: []protocol.VersionedAsset{
						{Path: "dist/ExampleSans-Regular.otf", URL: "https://assets.example/regular.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11},
					},
				}), nil
			}),
		},
	}
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"show", "example/family"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Package: example/family\n",
		"Display name: Example Sans\n",
		"Version: 1.2.3 (key 1.2.3)\n",
		"Published: 2026-01-02T00:00:00Z\n",
		"GitHub: example/family @ 0123456789ab\n",
		"Manifest: https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json\n",
		"Assets:\n",
		"  - ExampleSans-Regular.otf [otf] path=dist/ExampleSans-Regular.otf style=normal weight=400 size=11\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("show output missing %q\n%s", want, output)
		}
	}
}

func TestRunLSJSON(t *testing.T) {
	dir := t.TempDir()
	lockfilePath := filepath.Join(dir, "fontpub.lock")
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "golden", "lockfile.json"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if err := os.WriteFile(lockfilePath, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: dir, BaseURL: "https://fontpub.org"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "--json"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}

	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidateStatusResult(env); err != nil {
		t.Fatalf("ValidateStatusResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("ls-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(ls): %v", err)
	}
}

func TestRunLSHumanReadable(t *testing.T) {
	dir := t.TempDir()
	lockfilePath := filepath.Join(dir, "fontpub.lock")
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "golden", "lockfile.json"))
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if err := os.WriteFile(lockfilePath, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: dir, BaseURL: "https://fontpub.org"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"example/family\n",
		"  installed versions: 1.2.3\n",
		"  active version: 1.2.3\n",
		"  activation dir: not set\n",
		"  activation status: not checked (pass --activation-dir or set FONTPUB_ACTIVATION_DIR)\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("ls output missing %q\n%s", want, output)
		}
	}
}

func TestRunLSHumanReadableWithActivationDir(t *testing.T) {
	dir := t.TempDir()
	activationDir := t.TempDir()
	localPath := filepath.Join(dir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("font-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	if err := os.Symlink(localPath, symlinkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: func() *string { v := "1.2.3"; return &v }(),
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), LocalPath: localPath, Active: true, SymlinkPath: &symlinkPath},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(dir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: dir, BaseURL: "https://fontpub.org", DefaultActivationDir: activationDir},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "example/family"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"example/family\n",
		"  activation dir: " + activationDir + "\n",
		"  activation status: active (1/1 assets linked)\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("ls output missing %q\n%s", want, output)
		}
	}
}

func TestRunLSHumanReadableBrokenActivation(t *testing.T) {
	dir := t.TempDir()
	activationDir := t.TempDir()
	localPath := filepath.Join(dir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(localPath, []byte("font-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	badTarget := filepath.Join(dir, "wrong.otf")
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	if err := os.Symlink(badTarget, symlinkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: func() *string { v := "1.2.3"; return &v }(),
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), LocalPath: localPath, Active: true, SymlinkPath: &symlinkPath},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(dir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: dir, BaseURL: "https://fontpub.org"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "example/family", "--activation-dir", activationDir}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "  activation status: broken (0/1 assets linked)\n") {
		t.Fatalf("unexpected ls output:\n%s", output)
	}
}

func TestRunLSPackageNotInstalled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir(), BaseURL: "https://fontpub.org"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "missing/repo", "--json"}); code == 0 {
		t.Fatalf("expected failure")
	}

	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if env.Error == nil || env.Error.Code != "NOT_INSTALLED" {
		t.Fatalf("unexpected error: %+v", env)
	}
}

func TestHelpOutput(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--help"}, want: "Usage:\n  fontpub <command> [options]"},
		{args: []string{"show", "--help"}, want: "Usage:\n  fontpub show <owner>/<repo> [--version <v>] [--json]"},
		{args: []string{"package", "--help"}, want: "Usage:\n  fontpub package <subcommand> [options]"},
		{args: []string{"package", "init", "--help"}, want: "Usage:\n  fontpub package init [PATH] [--write] [--dry-run] [--yes] [--json]"},
		{args: []string{"workflow", "init", "--help"}, want: "Usage:\n  fontpub workflow init [PATH] [--dry-run] [--yes] [--json]"},
		{args: []string{"ls", "--json", "--help"}, want: "Usage:\n  fontpub ls [<owner>/<repo>] [--activation-dir <path>] [--json]"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
			if code := app.Run(context.Background(), tc.args); code != 0 {
				t.Fatalf("Run(%v) code=%d stderr=%s", tc.args, code, stderr.String())
			}
			if got := stdout.String(); !strings.Contains(got, tc.want) {
				t.Fatalf("help output mismatch\nwant substring: %q\ngot: %s", tc.want, got)
			}
			if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
				t.Fatalf("help output must be human-readable, got JSON: %s", stdout.String())
			}
		})
	}
}

func TestHelpOutputIncludesDescriptionsAndExamples(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"--help"}); code != 0 {
		t.Fatalf("help code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"ls-remote  List published packages\n",
		"ls         Show installed versions and activation state\n",
		"Environment:\n",
		"FONTPUB_ACTIVATION_DIR   Default activation directory for activation commands\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("top-level help missing %q\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"package", "--help"}); code != 0 {
		t.Fatalf("package help code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "fontpub package preview /path/to/repo --package-id owner/repo --json\n") {
		t.Fatalf("package help missing example:\n%s", output)
	}
}

func TestLegacyListAndStatusCommandsAreRejected(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"status"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
			if code := app.Run(context.Background(), args); code == 0 {
				t.Fatalf("expected failure for %v", args)
			}
			output := stderr.String()
			if !strings.Contains(output, "INPUT_REQUIRED: unknown command\n") {
				t.Fatalf("unexpected stderr for %v:\n%s", args, output)
			}
			if !strings.Contains(output, "  run: fontpub --help\n") {
				t.Fatalf("missing help hint for %v:\n%s", args, output)
			}
		})
	}
}

func TestPackageAndWorkflowSubcommandHints(t *testing.T) {
	tests := []struct {
		args []string
		want []string
	}{
		{
			args: []string{"package"},
			want: []string{
				"INPUT_REQUIRED: package subcommand is required\n",
				"Next:\n",
				"  run: fontpub package --help\n",
			},
		},
		{
			args: []string{"workflow"},
			want: []string{
				"INPUT_REQUIRED: workflow subcommand is required\n",
				"Next:\n",
				"  run: fontpub workflow --help\n",
			},
		},
		{
			args: []string{"package", "bogus"},
			want: []string{
				"INPUT_REQUIRED: unknown package subcommand\n",
				"  subcommand: bogus\n",
				"Next:\n",
				"  run: fontpub package --help\n",
			},
		},
		{
			args: []string{"workflow", "bogus"},
			want: []string{
				"INPUT_REQUIRED: unknown workflow subcommand\n",
				"  subcommand: bogus\n",
				"Next:\n",
				"  run: fontpub workflow --help\n",
			},
		},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
			if code := app.Run(context.Background(), tc.args); code == 0 {
				t.Fatalf("expected failure for %v", tc.args)
			}
			output := stderr.String()
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Fatalf("stderr missing %q\n%s", want, output)
				}
			}
		})
	}
}

func TestInstallActivateVerifyRepairAndUninstall(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	assetBytes := []byte("regular-font-bytes")
	sum := sha256.Sum256(assetBytes)
	assetSHA := fmtHex(sum[:])
	client := fakeClient(map[string]responseSpec{
		"/v1/packages/example/family.json": {
			body: protocol.VersionedPackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.2.3",
				VersionKey:    "1.2.3",
				PublishedAt:   "2026-01-02T00:00:00Z",
				GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
				ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
				Assets: []protocol.VersionedAsset{
					{Path: "dist/ExampleSans-Regular.otf", URL: "https://assets.example/regular.otf", SHA256: assetSHA, Format: "otf", Style: "normal", Weight: 400, SizeBytes: int64(len(assetBytes))},
				},
			},
		},
		"https://assets.example/regular.otf": {raw: assetBytes},
	})
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: stateDir},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    func() time.Time { return now },
	}
	if code := app.Run(context.Background(), []string{"install", "example/family", "--activate", "--activation-dir", activationDir, "--json"}); code != 0 {
		t.Fatalf("install code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	localPath := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("os.Stat(localPath): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"verify", "example/family", "--activation-dir", activationDir, "--json"}); code != 0 {
		t.Fatalf("verify code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal verify: %v", err)
	}
	if err := protocol.ValidateVerifyResult(env); err != nil {
		t.Fatalf("ValidateVerifyResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("verify-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(verify): %v", err)
	}

	lock, ok, err := LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}.Load()
	if err != nil || !ok {
		t.Fatalf("Load lockfile ok=%v err=%v", ok, err)
	}
	symlinkPath := *lock.Packages["example/family"].InstalledVersions["1.2.3"].Assets[0].SymlinkPath
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatalf("os.Remove(symlinkPath): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"repair", "example/family", "--activation-dir", activationDir, "--json"}); code != 0 {
		t.Fatalf("repair code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal repair: %v", err)
	}
	if err := protocol.ValidateRepairResult(env); err != nil {
		t.Fatalf("ValidateRepairResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("repair-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(repair): %v", err)
	}
	if _, err := os.Lstat(symlinkPath); err != nil {
		t.Fatalf("os.Lstat(symlinkPath): %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"uninstall", "example/family", "--all", "--yes", "--json"}); code != 0 {
		t.Fatalf("uninstall code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected local asset removal, err=%v", err)
	}
}

func TestInstallHumanReadableSummaries(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	assetBytes := []byte("regular-font-bytes")
	sum := sha256.Sum256(assetBytes)
	assetSHA := fmtHex(sum[:])
	client := fakeClient(map[string]responseSpec{
		"/v1/packages/example/family.json": {
			body: protocol.VersionedPackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.2.3",
				VersionKey:    "1.2.3",
				PublishedAt:   "2026-01-02T00:00:00Z",
				GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
				ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
				Assets: []protocol.VersionedAsset{
					{Path: "dist/ExampleSans-Regular.otf", URL: "https://assets.example/regular.otf", SHA256: assetSHA, Format: "otf", Style: "normal", Weight: 400, SizeBytes: int64(len(assetBytes))},
				},
			},
		},
		"https://assets.example/regular.otf": {raw: assetBytes},
	})
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: stateDir},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) },
	}

	if code := app.Run(context.Background(), []string{"install", "example/family", "--activate", "--activation-dir", activationDir}); code != 0 {
		t.Fatalf("install code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Installed and activated example/family@1.2.3\n",
		"  assets: 1\n",
		"  install root: " + filepath.Join(stateDir, "packages", "example", "family", "1.2.3") + "\n",
		"  activation dir: " + activationDir + "\n",
		"  symlinks created: 1\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("install output missing %q\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"install", "example/family"}); code != 0 {
		t.Fatalf("second install code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "example/family@1.2.3 is already installed\n") {
		t.Fatalf("unexpected no-change output:\n%s", got)
	}
}

func TestUpdateInstallsLatestVersion(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	oldSHA := strings.Repeat("a", 64)
	oldLocal := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(oldLocal), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(oldLocal, []byte("old-font"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: oldSHA, LocalPath: oldLocal},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	newBytes := []byte("new-font")
	sum := sha256.Sum256(newBytes)
	newSHA := fmtHex(sum[:])
	client := fakeClient(map[string]responseSpec{
		"/v1/index.json": {
			body: protocol.RootIndex{
				SchemaVersion: "1",
				GeneratedAt:   "2026-01-03T00:00:00Z",
				Packages: map[string]protocol.RootIndexPackage{
					"example/family": {LatestVersion: "1.3.0", LatestVersionKey: "1.3", LatestPublishedAt: "2026-01-03T00:00:00Z"},
				},
			},
		},
		"/v1/packages/example/family/versions/1.3.json": {
			body: protocol.VersionedPackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.3.0",
				VersionKey:    "1.3",
				PublishedAt:   "2026-01-03T00:00:00Z",
				GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "89abcdef0123456789abcdef0123456789abcdef"},
				ManifestURL:   "https://raw.githubusercontent.com/example/family/89abcdef0123456789abcdef0123456789abcdef/fontpub.json",
				Assets: []protocol.VersionedAsset{
					{Path: "dist/ExampleSans-Regular.otf", URL: "https://assets.example/new-regular.otf", SHA256: newSHA, Format: "otf", Style: "normal", Weight: 400, SizeBytes: int64(len(newBytes))},
				},
			},
		},
		"https://assets.example/new-regular.otf": {raw: newBytes},
	})
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: stateDir, DefaultActivationDir: activationDir},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    func() time.Time { return time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) },
	}
	if code := app.Run(context.Background(), []string{"update", "example/family", "--activate", "--json"}); code != 0 {
		t.Fatalf("update code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	newLocal := filepath.Join(stateDir, "packages", "example", "family", "1.3", "dist", "ExampleSans-Regular.otf")
	if _, err := os.Stat(newLocal); err != nil {
		t.Fatalf("os.Stat(newLocal): %v", err)
	}
}

func TestNotInstalledErrorSuggestsInstall(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"ls", "example/family"}); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"NOT_INSTALLED: package is not installed\n",
		"  package_id: example/family\n",
		"Next:\n",
		"  run: fontpub install example/family\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestUpdateHumanReadableDryRun(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	oldSHA := strings.Repeat("a", 64)
	oldLocal := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(oldLocal), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(oldLocal, []byte("old-font"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: oldSHA, LocalPath: oldLocal},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	newBytes := []byte("new-font")
	sum := sha256.Sum256(newBytes)
	newSHA := fmtHex(sum[:])
	client := fakeClient(map[string]responseSpec{
		"/v1/index.json": {
			body: protocol.RootIndex{
				SchemaVersion: "1",
				GeneratedAt:   "2026-01-03T00:00:00Z",
				Packages: map[string]protocol.RootIndexPackage{
					"example/family": {LatestVersion: "1.3.0", LatestVersionKey: "1.3", LatestPublishedAt: "2026-01-03T00:00:00Z"},
				},
			},
		},
		"/v1/packages/example/family/versions/1.3.json": {
			body: protocol.VersionedPackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.3.0",
				VersionKey:    "1.3",
				PublishedAt:   "2026-01-03T00:00:00Z",
				GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "89abcdef0123456789abcdef0123456789abcdef"},
				ManifestURL:   "https://raw.githubusercontent.com/example/family/89abcdef0123456789abcdef0123456789abcdef/fontpub.json",
				Assets: []protocol.VersionedAsset{
					{Path: "dist/ExampleSans-Regular.otf", URL: "https://assets.example/new-regular.otf", SHA256: newSHA, Format: "otf", Style: "normal", Weight: 400, SizeBytes: int64(len(newBytes))},
				},
			},
		},
		"https://assets.example/new-regular.otf": {raw: newBytes},
	})
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{BaseURL: "https://fontpub.org", StateDir: stateDir, DefaultActivationDir: activationDir},
		Client: client,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"update", "example/family", "--activate", "--dry-run"}); code != 0 {
		t.Fatalf("update dry-run code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Update plan for 1 package(s)\n",
		"Packages:\n",
		"  - example/family@1.3\n",
		"  assets written: 1\n",
		"  activation dir: " + activationDir + "\n",
		"  symlinks created: 1\n",
		"Planned actions:\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("update output missing %q\n%s", want, output)
		}
	}
}

func TestVerifyFailureForMissingFile(t *testing.T) {
	stateDir := t.TempDir()
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), LocalPath: filepath.Join(stateDir, "missing.otf")},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: stateDir}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"verify", "--json"}); code == 0 {
		t.Fatalf("expected verify failure")
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal verify failure: %v", err)
	}
	if err := protocol.ValidateVerifyResult(env); err != nil {
		t.Fatalf("ValidateVerifyResult failure: %v", err)
	}
	if err := protocol.ValidateCLISchema("verify-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(verify failure): %v", err)
	}
}

func TestPackageValidateMissingManifestSuggestsInit(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"package", "validate", root}); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"LOCAL_FILE_MISSING: fontpub.json was not found\n",
		"  path: " + filepath.Join(root, "fontpub.json") + "\n",
		"Next:\n",
		"  run: fontpub package init " + root + " --write\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestActivationDirErrorSuggestsFlagOrEnv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"activate", "example/family"}); code == 0 {
		t.Fatalf("expected failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"INPUT_REQUIRED: activation directory is required\n",
		"  flag: --activation-dir\n",
		"Next:\n",
		"  pass --activation-dir <path> or set FONTPUB_ACTIVATION_DIR\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr missing %q\n%s", want, output)
		}
	}
}

func TestVerifyHumanReadableSuccess(t *testing.T) {
	stateDir := t.TempDir()
	localPath := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	body := []byte("font-bytes")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	sum := sha256.Sum256(body)
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: fmtHex(sum[:]), LocalPath: localPath},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: stateDir}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"verify"}); code != 0 {
		t.Fatalf("verify code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Verification results:\n",
		"  example/family: ok\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("verify output missing %q\n%s", want, output)
		}
	}
}

func TestVerifyHumanReadableFailure(t *testing.T) {
	stateDir := t.TempDir()
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{Path: "dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), LocalPath: filepath.Join(stateDir, "missing.otf")},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: stateDir}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"verify"}); code == 0 {
		t.Fatalf("expected verify failure")
	}
	errOutput := stderr.String()
	for _, want := range []string{
		"verification failed\n",
		"Details:\n",
		"  example/family: failed\n",
		"installed asset file is missing (dist/ExampleSans-Regular.otf)\n",
		"      local_path: " + filepath.Join(stateDir, "missing.otf") + "\n",
	} {
		if !strings.Contains(errOutput, want) {
			t.Fatalf("verify stderr missing %q\n%s", want, errOutput)
		}
	}
}

func TestPackageInitJSONUsesExistingManifestFields(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "ExampleSans-Regular.otf"), []byte("font-bytes"), 0o644); err != nil {
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

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidatePackageInitResult(env); err != nil {
		t.Fatalf("ValidatePackageInitResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("package-init-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(package init): %v", err)
	}
}

func TestPackageInitHumanReadableSummary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "OTTO", "Embedded Family", "Bold Italic", 700, true)
	if err := os.WriteFile(filepath.Join(root, "dist", "Misleading-Regular.otf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Repository: " + root,
		"Discovered assets:\n",
		"Misleading-Regular.otf [otf] style=italic (embedded metadata) weight=700 (embedded metadata)",
		"family=Embedded Family (embedded metadata)",
		"Manifest fields:\n",
		"name: Embedded Family (embedded metadata)",
		"author: Example Studio (user input)",
		"version: 1.2.3 (user input)",
		"Unresolved fields: none",
		"Candidate fontpub.json:\n",
		`"name":"Embedded Family"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package init output missing %q\n%s", want, output)
		}
	}
}

func TestPackageInitWriteHumanReadable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "OTTO", "Embedded Family", "Bold Italic", 700, true)
	if err := os.WriteFile(filepath.Join(root, "dist", "Misleading-Regular.otf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--write", "--yes"}); code != 0 {
		t.Fatalf("package init write code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Wrote fontpub.json\n",
		"  path: " + filepath.Join(root, "fontpub.json") + "\n",
		"  files discovered: 1\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package init write output missing %q\n%s", want, output)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "fontpub.json"))
	if err != nil {
		t.Fatalf("os.ReadFile manifest: %v", err)
	}
	if !strings.Contains(string(body), `"name":"Embedded Family"`) {
		t.Fatalf("manifest was not rewritten with inferred name:\n%s", string(body))
	}
}

func TestPackageInitJSONPrefersEmbeddedMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "OTTO", "Embedded Family", "Bold Italic", 700, true)
	if err := os.WriteFile(filepath.Join(root, "dist", "Misleading-Regular.otf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidatePackageInitResult(env); err != nil {
		t.Fatalf("ValidatePackageInitResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("package-init-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(package init): %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["name"] != "Embedded Family" {
		t.Fatalf("unexpected manifest name: %#v", manifestData["name"])
	}
	files, ok := manifestData["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("unexpected files: %#v", manifestData["files"])
	}
	fileData, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected file data: %#v", files[0])
	}
	if fileData["style"] != "italic" || int(fileData["weight"].(float64)) != 700 {
		t.Fatalf("unexpected inferred file data: %#v", fileData)
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		t.Fatalf("unexpected inferences: %#v", env.Data["inferences"])
	}
	sources := map[string]string{}
	for _, raw := range inferences {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected inference record: %#v", raw)
		}
		field, _ := record["field"].(string)
		source, _ := record["source"].(string)
		sources[field] = source
	}
	if sources["files[0].style"] != "embedded_metadata" || sources["files[0].weight"] != "embedded_metadata" || sources["name"] != "embedded_metadata" {
		t.Fatalf("unexpected inference sources: %#v", sources)
	}
}

func TestPackageInitJSONResolvesNameAcrossEmbeddedAndWOFF2Heuristics(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts", "static"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "OTTO", "Zx Gamut", "Bold", 700, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "static", "ZxGamut-Bold.otf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile otf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fonts", "static", "ZxGamut-Regular.woff2"), []byte("woff2-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile woff2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidatePackageInitResult(env); err != nil {
		t.Fatalf("ValidatePackageInitResult: %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["name"] != "Zx Gamut" {
		t.Fatalf("unexpected manifest name: %#v", manifestData["name"])
	}
	unresolved, ok := env.Data["unresolved_fields"].([]any)
	if !ok {
		t.Fatalf("unexpected unresolved fields: %#v", env.Data["unresolved_fields"])
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved fields, got %#v", unresolved)
	}
}

func TestPackageInitJSONGroupsSameStemFormats(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "0x Proto", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "0xProto-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fonts", "0xProto-Regular.woff2"), []byte("woff2-bytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile woff2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["name"] != "0x Proto" {
		t.Fatalf("unexpected manifest name: %#v", manifestData["name"])
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		t.Fatalf("unexpected inferences: %#v", env.Data["inferences"])
	}
	sources := map[string]string{}
	for _, raw := range inferences {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected inference record: %#v", raw)
		}
		field, _ := record["field"].(string)
		source, _ := record["source"].(string)
		sources[field] = source
	}
	if sources["files[1].style"] != "group_embedded_metadata" || sources["files[1].weight"] != "group_embedded_metadata" {
		t.Fatalf("unexpected grouped inference sources: %#v", sources)
	}
}

func TestPackageInitJSONInfersAuthorFromREADME(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Example Sans\n\nCopyright (c) 2026 [Example Studio](https://example.test)\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile readme: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"version":"1.2.3","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["author"] != "Example Studio" {
		t.Fatalf("unexpected author: %#v", manifestData["author"])
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		t.Fatalf("unexpected inferences: %#v", env.Data["inferences"])
	}
	sources := map[string]string{}
	for _, raw := range inferences {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected inference record: %#v", raw)
		}
		field, _ := record["field"].(string)
		source, _ := record["source"].(string)
		sources[field] = source
	}
	if sources["author"] != "repository_readme" {
		t.Fatalf("unexpected author source: %#v", sources)
	}
}

func TestPackageInitJSONInfersVersionFromChangelog(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte("## 1.002\n\n- First release\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile changelog: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["version"] != "1.002" {
		t.Fatalf("unexpected version: %#v", manifestData["version"])
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		t.Fatalf("unexpected inferences: %#v", env.Data["inferences"])
	}
	sources := map[string]string{}
	for _, raw := range inferences {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected inference record: %#v", raw)
		}
		field, _ := record["field"].(string)
		source, _ := record["source"].(string)
		sources[field] = source
	}
	if sources["version"] != "repository_changelog" {
		t.Fatalf("unexpected version source: %#v", sources)
	}
}

func TestPackageInitJSONInfersVersionFromGitTag(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Example User")
	runGit(t, root, "config", "user.email", "example@example.test")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "tag", "v1.001")
	runGit(t, root, "tag", "1.002")
	runGit(t, root, "tag", "backup/1.002-pre-fontpub")

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	manifestData, ok := env.Data["manifest"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected manifest data: %#v", env.Data["manifest"])
	}
	if manifestData["version"] != "1.002" {
		t.Fatalf("unexpected version: %#v", manifestData["version"])
	}
	inferences, ok := env.Data["inferences"].([]any)
	if !ok {
		t.Fatalf("unexpected inferences: %#v", env.Data["inferences"])
	}
	sources := map[string]string{}
	for _, raw := range inferences {
		record, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("unexpected inference record: %#v", raw)
		}
		field, _ := record["field"].(string)
		source, _ := record["source"].(string)
		sources[field] = source
	}
	if sources["version"] != "repository_tag" {
		t.Fatalf("unexpected version source: %#v", sources)
	}
}

func TestPackageInitJSONReportsResolvedVersionConflict(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"author":"Example Studio","license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte("## 1.002\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile changelog: %v", err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Example User")
	runGit(t, root, "config", "user.email", "example@example.test")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	runGit(t, root, "tag", "1.001")

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "init", root, "--json"}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	conflicts, ok := env.Data["conflicts"].([]any)
	if !ok {
		t.Fatalf("unexpected conflicts: %#v", env.Data["conflicts"])
	}
	if len(conflicts) != 1 {
		t.Fatalf("unexpected conflicts length: %#v", conflicts)
	}
	conflict, ok := conflicts[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected conflict: %#v", conflicts[0])
	}
	if conflict["field"] != "version" || conflict["resolved"] != true || conflict["chosen_value"] != "1.002" {
		t.Fatalf("unexpected conflict payload: %#v", conflict)
	}
}

func TestPackageInitHumanReadableReportsConflicts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("Copyright (c) 2026 [Example Studio](https://example.test)\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile readme: %v", err)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Example User")
	runGit(t, root, "config", "user.email", "example@example.test")
	runGit(t, root, "remote", "add", "origin", "https://github.com/example/family.git")

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("1.002\n"),
		IsTTY:  func() bool { return true },
	}
	if code := app.Run(context.Background(), []string{"package", "init", root}); code != 0 {
		t.Fatalf("package init code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Conflicts:\n",
		"  author (resolved)\n",
		"    chosen: Example Studio\n",
		"    - Example Studio (repository README)\n",
		"    - example (repository owner)\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package init output missing %q\n%s", want, output)
		}
	}
}

func TestPackageInitFailsImmediatelyWithoutTTY(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "fonts"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBody := buildTestSFNT(t, "\x00\x01\x00\x00", "Example Sans", "Regular", 400, false)
	if err := os.WriteFile(filepath.Join(root, "fonts", "ExampleSans-Regular.ttf"), fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile ttf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fontpub.json"), []byte(`{"license":"OFL-1.1","files":[]}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile manifest: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir()},
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  strings.NewReader("ignored\n"),
		IsTTY:  func() bool { return false },
	}
	if code := app.Run(context.Background(), []string{"package", "init", root}); code == 0 {
		t.Fatalf("expected package init failure")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("expected no prompt output, got:\n%s", got)
	}
	output := stderr.String()
	for _, want := range []string{
		"INPUT_REQUIRED: required manifest fields could not be inferred\n",
		"  unresolved_fields: author, version\n",
		"Next:\n",
		"  rerun interactively or edit fontpub.json to fill the missing fields\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package init stderr missing %q\n%s", want, output)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestPackagePreviewJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBytes := []byte("preview-font")
	if err := os.WriteFile(filepath.Join(root, "dist", "ExampleSans-Regular.otf"), fontBytes, 0o644); err != nil {
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

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "preview", root, "--package-id", "Example/Family", "--json"}); code != 0 {
		t.Fatalf("package preview code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidatePackagePreviewResult(env); err != nil {
		t.Fatalf("ValidatePackagePreviewResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("package-preview-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(package preview): %v", err)
	}
	if env.Data["package_id"] != "example/family" {
		t.Fatalf("unexpected package_id: %#v", env.Data["package_id"])
	}
}

func TestPackagePreviewHumanReadable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	fontBytes := []byte("preview-font")
	if err := os.WriteFile(filepath.Join(root, "dist", "ExampleSans-Regular.otf"), fontBytes, 0o644); err != nil {
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

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "preview", root, "--package-id", "Example/Family"}); code != 0 {
		t.Fatalf("package preview code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Package preview\n",
		"  package id: example/family\n",
		"  display name: Example Sans\n",
		"  version: 1.2.3 (key 1.2.3)\n",
		"  assets: 1\n",
		"  root: " + root + "\n",
		"Assets:\n",
		"  - dist/ExampleSans-Regular.otf [otf] style=normal weight=400 size=12\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package preview output missing %q\n%s", want, output)
		}
	}
}

func TestPackageValidateAndCheck(t *testing.T) {
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

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "validate", root, "--json"}); code != 0 {
		t.Fatalf("package validate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"package", "check", root, "--tag", "v1.2.3", "--json"}); code != 0 {
		t.Fatalf("package check code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"package", "check", root, "--tag", "v2.0.0", "--json"}); code == 0 {
		t.Fatalf("expected package check failure")
	}
}

func TestPackageValidateAndCheckHumanReadable(t *testing.T) {
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

	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "validate", root}); code != 0 {
		t.Fatalf("package validate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Manifest is valid\n",
		"  path: " + filepath.Join(root, "fontpub.json") + "\n",
		"  root: " + root + "\n",
		"  files checked: 1\n",
		"  version: 1.2.3\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package validate output missing %q\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"package", "check", root, "--tag", "v1.2.3"}); code != 0 {
		t.Fatalf("package check code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output = stdout.String()
	for _, want := range []string{
		"Package is ready for publication\n",
		"  root: " + root + "\n",
		"  manifest: " + filepath.Join(root, "fontpub.json") + "\n",
		"  files checked: 1\n",
		"  version: 1.2.3\n",
		"  tag: v1.2.3\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package check output missing %q\n%s", want, output)
		}
	}
}

func TestWorkflowInitDryRunAndWrite(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"workflow", "init", root, "--dry-run", "--json"}); code != 0 {
		t.Fatalf("workflow init dry-run code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal dry-run: %v", err)
	}
	if env.Command != "workflow init" || !env.OK {
		t.Fatalf("unexpected env: %+v", env)
	}
	data, ok := env.Data["planned_actions"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("unexpected planned_actions: %#v", env.Data["planned_actions"])
	}
	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"workflow", "init", root, "--yes"}); code != 0 {
		t.Fatalf("workflow init write code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	workflowPath := filepath.Join(root, ".github", "workflows", "fontpub.yml")
	if _, err := os.Stat(workflowPath); err != nil {
		t.Fatalf("os.Stat workflow: %v", err)
	}
	body, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("os.ReadFile workflow: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`workflow_dispatch:`,
		`- "[0-9]*"`,
		`fetch-depth: 0`,
		`persist-credentials: false`,
		`git rev-parse --verify "${{ steps.ref.outputs.ref }}^{commit}"`,
		`grep -Eq '^[vV]?(0|[1-9][0-9]*)(\.[0-9]+)*$'`,
		`ACTIONS_ID_TOKEN_REQUEST_URL`,
		`https://fontpub.org/v1/update`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated workflow missing %q\n%s", want, text)
		}
	}
}

func TestWorkflowInitHumanReadableDryRunAndWrite(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{BaseURL: "https://fontpub.org", StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"workflow", "init", root, "--dry-run"}); code != 0 {
		t.Fatalf("workflow init dry-run code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Workflow write plan\n",
		"  path: " + filepath.Join(root, ".github", "workflows", "fontpub.yml") + "\n",
		"Planned actions:\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("workflow dry-run output missing %q\n%s", want, output)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"workflow", "init", root, "--yes"}); code != 0 {
		t.Fatalf("workflow init write code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "Wrote workflow\n") || !strings.Contains(output, "  path: "+filepath.Join(root, ".github", "workflows", "fontpub.yml")+"\n") {
		t.Fatalf("unexpected workflow write output:\n%s", output)
	}
}

func TestRepairHumanReadableDryRun(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	localPath := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	body := []byte("font-bytes")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	sum := sha256.Sum256(body)
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: func() *string { v := "1.2.3"; return &v }(),
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{
								Path:        "dist/ExampleSans-Regular.otf",
								SHA256:      fmtHex(sum[:]),
								LocalPath:   localPath,
								Active:      true,
								SymlinkPath: &symlinkPath,
							},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: stateDir, DefaultActivationDir: activationDir},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"repair", "example/family", "--dry-run", "--activation-dir", activationDir}); code != 0 {
		t.Fatalf("repair code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Repair results:\n",
		"  example/family: repaired\n",
		"  symlinks created: 1\n",
		"  symlinks removed: 0\n",
		"Planned actions:\n",
		"  - create symlink [example/family@1.2.3] dist/ExampleSans-Regular.otf\n",
		"  - write lockfile [example/family]\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("repair output missing %q\n%s", want, output)
		}
	}
}

func TestRepairHumanReadableFailureIncludesDetails(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: func() *string { v := "1.2.3"; return &v }(),
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{
								Path:        "dist/ExampleSans-Regular.otf",
								SHA256:      strings.Repeat("a", 64),
								LocalPath:   filepath.Join(stateDir, "missing.otf"),
								Active:      true,
								SymlinkPath: &symlinkPath,
							},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: stateDir, DefaultActivationDir: activationDir},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"repair", "example/family", "--activation-dir", activationDir}); code == 0 {
		t.Fatalf("expected repair failure")
	}
	output := stderr.String()
	for _, want := range []string{
		"repair failed\n",
		"Details:\n",
		"  example/family: failed\n",
		"installed asset file is missing (dist/ExampleSans-Regular.otf)\n",
		"      local_path: " + filepath.Join(stateDir, "missing.otf") + "\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("repair stderr missing %q\n%s", want, output)
		}
	}
}

func TestDeactivateAndUninstallHumanReadable(t *testing.T) {
	stateDir := t.TempDir()
	activationDir := t.TempDir()
	localPath := filepath.Join(stateDir, "packages", "example", "family", "1.2.3", "dist", "ExampleSans-Regular.otf")
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	body := []byte("font-bytes")
	if err := os.WriteFile(localPath, body, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	sum := sha256.Sum256(body)
	symlinkPath := filepath.Join(activationDir, "example--family--ExampleSans-Regular.otf")
	if err := os.Symlink(localPath, symlinkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}
	lock := protocol.Lockfile{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.LockedPackage{
			"example/family": {
				ActiveVersionKey: func() *string { v := "1.2.3"; return &v }(),
				InstalledVersions: map[string]protocol.InstalledVersion{
					"1.2.3": {
						Version:     "1.2.3",
						VersionKey:  "1.2.3",
						InstalledAt: "2026-01-02T00:00:00Z",
						Assets: []protocol.LockedAsset{
							{
								Path:        "dist/ExampleSans-Regular.otf",
								SHA256:      fmtHex(sum[:]),
								LocalPath:   localPath,
								Active:      true,
								SymlinkPath: &symlinkPath,
							},
						},
					},
				},
			},
		},
	}
	if err := (LockfileStore{Path: filepath.Join(stateDir, "fontpub.lock")}).Save(lock); err != nil {
		t.Fatalf("Save lockfile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: stateDir},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"deactivate", "example/family"}); code != 0 {
		t.Fatalf("deactivate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if output := stdout.String(); !strings.Contains(output, "Deactivated example/family\n") || !strings.Contains(output, "  symlinks removed: 1\n") {
		t.Fatalf("unexpected deactivate output:\n%s", output)
	}

	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"uninstall", "example/family", "--all", "--yes"}); code != 0 {
		t.Fatalf("uninstall code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Uninstalled example/family@1.2.3\n",
		"  versions: 1.2.3\n",
		"  assets removed: 1\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("uninstall output missing %q\n%s", want, output)
		}
	}
}

func TestPackageInspectJSON(t *testing.T) {
	root := t.TempDir()
	fontPath := filepath.Join(root, "ExampleSans-BoldItalic.otf")
	if err := os.WriteFile(fontPath, []byte("font"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "inspect", fontPath, "--json"}); code != 0 {
		t.Fatalf("package inspect code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal inspect: %v", err)
	}
	if env.Data["style"] != "italic" || int(env.Data["weight"].(float64)) != 700 {
		t.Fatalf("unexpected inspect data: %#v", env.Data)
	}
}

func TestPackageInspectHumanReadable(t *testing.T) {
	root := t.TempDir()
	fontPath := filepath.Join(root, "Misleading-Regular.otf")
	fontBody := buildTestSFNT(t, "OTTO", "Embedded Family", "Bold Italic", 700, true)
	if err := os.WriteFile(fontPath, fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "inspect", fontPath}); code != 0 {
		t.Fatalf("package inspect code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Path: " + fontPath,
		"Format: otf",
		"Family: Embedded Family (embedded metadata)",
		"Style: italic (embedded metadata)",
		"Weight: 700 (embedded metadata)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("package inspect output missing %q\n%s", want, output)
		}
	}
}

func TestPackageInspectJSONPrefersEmbeddedMetadata(t *testing.T) {
	root := t.TempDir()
	fontPath := filepath.Join(root, "Misleading-Regular.otf")
	fontBody := buildTestSFNT(t, "OTTO", "Embedded Family", "Bold Italic", 700, true)
	if err := os.WriteFile(fontPath, fontBody, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	var stdout, stderr bytes.Buffer
	app := App{Config: Config{StateDir: t.TempDir()}, Stdout: &stdout, Stderr: &stderr}
	if code := app.Run(context.Background(), []string{"package", "inspect", fontPath, "--json"}); code != 0 {
		t.Fatalf("package inspect code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal inspect: %v", err)
	}
	if env.Data["name"] != "Embedded Family" || env.Data["style"] != "italic" || int(env.Data["weight"].(float64)) != 700 {
		t.Fatalf("unexpected inspect data: %#v", env.Data)
	}
}

type responseSpec struct {
	body any
	raw  []byte
}

func fakeClient(routes map[string]responseSpec) *MetadataClient {
	return &MetadataClient{
		BaseURL:   "https://fontpub.org",
		UserAgent: "test",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				key := req.URL.Path
				if req.URL.Host != "fontpub.org" {
					key = req.URL.String()
				}
				spec, ok := routes[key]
				if !ok {
					return jsonResponse(http.StatusNotFound, protocol.ErrorEnvelope{Error: protocol.ErrorObject{Code: "PACKAGE_NOT_FOUND", Message: "not found", Details: map[string]any{}}}), nil
				}
				if spec.raw != nil {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
						Body:       io.NopCloser(strings.NewReader(string(spec.raw))),
					}, nil
				}
				return jsonResponse(http.StatusOK, spec.body), nil
			}),
		},
	}
}

func fmtHex(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(data))),
	}
}
