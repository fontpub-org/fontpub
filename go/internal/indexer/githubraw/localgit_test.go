package githubraw

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseLocalRepoMap(t *testing.T) {
	mapping, err := ParseLocalRepoMap("0xType/Gamut=/tmp/gamut, example/family = /tmp/family ")
	if err != nil {
		t.Fatalf("ParseLocalRepoMap: %v", err)
	}
	if got := mapping["0xtype/gamut"]; got != "/tmp/gamut" {
		t.Fatalf("unexpected gamut mapping: %q", got)
	}
	if got := mapping["example/family"]; got != "/tmp/family" {
		t.Fatalf("unexpected family mapping: %q", got)
	}
}

func TestRoutingFetcherFetchesFromLocalGit(t *testing.T) {
	repoRoot, sha := initGitRepoWithCommit(t, map[string]string{
		"fontpub.json":             `{"name":"Example Sans"}`,
		"dist/Example Sans.otf":    "font-bytes",
		"nested/Example%20Alt.otf": "alt-bytes",
	})
	fetcher := RoutingFetcher{
		LocalRepos: map[string]string{"example/family": repoRoot},
		Remote:     HTTPFetcher{},
	}

	manifestURL, err := BuildManifestURL("example/family", sha)
	if err != nil {
		t.Fatalf("BuildManifestURL: %v", err)
	}
	result, err := fetcher.Fetch(context.Background(), manifestURL, 1024)
	if err != nil {
		t.Fatalf("Fetch manifest: %v", err)
	}
	if string(result.Body) != `{"name":"Example Sans"}` {
		t.Fatalf("unexpected manifest body: %q", result.Body)
	}

	assetURL, err := BuildAssetURL("example/family", sha, "dist/Example Sans.otf")
	if err != nil {
		t.Fatalf("BuildAssetURL: %v", err)
	}
	result, err = fetcher.Fetch(context.Background(), assetURL, 1024)
	if err != nil {
		t.Fatalf("Fetch asset: %v", err)
	}
	if string(result.Body) != "font-bytes" {
		t.Fatalf("unexpected asset body: %q", result.Body)
	}
}

func TestRoutingFetcherReturnsNotFoundForMissingLocalObject(t *testing.T) {
	repoRoot, sha := initGitRepoWithCommit(t, map[string]string{"fontpub.json": `{}`})
	fetcher := RoutingFetcher{
		LocalRepos: map[string]string{"example/family": repoRoot},
	}
	assetURL, err := BuildAssetURL("example/family", sha, "dist/Missing.otf")
	if err != nil {
		t.Fatalf("BuildAssetURL: %v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), assetURL, 1024); err != ErrNotFound {
		t.Fatalf("got %v want ErrNotFound", err)
	}
}

func TestRoutingFetcherRejectsOversizedLocalObject(t *testing.T) {
	repoRoot, sha := initGitRepoWithCommit(t, map[string]string{"fontpub.json": `{}`})
	fetcher := RoutingFetcher{
		LocalRepos: map[string]string{"example/family": repoRoot},
	}
	manifestURL, err := BuildManifestURL("example/family", sha)
	if err != nil {
		t.Fatalf("BuildManifestURL: %v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), manifestURL, 1); err != ErrTooLarge {
		t.Fatalf("got %v want ErrTooLarge", err)
	}
}

func initGitRepoWithCommit(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Fontpub Test")
	runGit(t, root, "config", "user.email", "fontpub@example.test")
	for path, body := range files {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", fullPath, err)
		}
		if err := os.WriteFile(fullPath, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", fullPath, err)
		}
	}
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "fixture")
	return root, stringsTrim(runGitOutput(t, root, "rev-parse", "HEAD"))
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func runGitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func stringsTrim(value string) string {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}
