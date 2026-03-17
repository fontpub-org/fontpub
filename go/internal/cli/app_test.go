package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
