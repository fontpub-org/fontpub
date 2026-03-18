package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestRunListJSON(t *testing.T) {
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
	if code := app.Run(context.Background(), []string{"list", "--json"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}

	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidateCLIEnvelope(env); err != nil {
		t.Fatalf("ValidateCLIEnvelope: %v", err)
	}
	if err := protocol.ValidateCLISchema("list-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(list): %v", err)
	}
	if env.Command != "list" || !env.OK {
		t.Fatalf("unexpected env: %+v", env)
	}
	packages, ok := env.Data["packages"].([]any)
	if !ok || len(packages) != 1 {
		t.Fatalf("unexpected packages: %#v", env.Data["packages"])
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

func TestRunStatusJSON(t *testing.T) {
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
	if code := app.Run(context.Background(), []string{"status", "--json"}); code != 0 {
		t.Fatalf("Run() code=%d stderr=%s", code, stderr.String())
	}

	var env protocol.CLIEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := protocol.ValidateStatusResult(env); err != nil {
		t.Fatalf("ValidateStatusResult: %v", err)
	}
	if err := protocol.ValidateCLISchema("status-result.schema.json", env); err != nil {
		t.Fatalf("ValidateCLISchema(status): %v", err)
	}
}

func TestRunStatusPackageNotInstalled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := App{
		Config: Config{StateDir: t.TempDir(), BaseURL: "https://fontpub.org"},
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if code := app.Run(context.Background(), []string{"status", "missing/repo", "--json"}); code == 0 {
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
		{args: []string{"status", "--json", "--help"}, want: "Usage:\n  fontpub status [<owner>/<repo>] [--activation-dir <path>] [--json]"},
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
	stdout.Reset()
	stderr.Reset()
	if code := app.Run(context.Background(), []string{"workflow", "init", root, "--yes"}); code != 0 {
		t.Fatalf("workflow init write code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "fontpub.yml")); err != nil {
		t.Fatalf("os.Stat workflow: %v", err)
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
