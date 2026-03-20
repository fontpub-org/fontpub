package rebuilder

import (
	"context"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestParseOptionsUsesEnvironmentDefaults(t *testing.T) {
	options, err := ParseOptions(nil, func(key string) string {
		switch key {
		case "FONTPUB_ARTIFACTS_DIR":
			return "/tmp/artifacts"
		case "FONTPUB_ARTIFACTS_BACKEND":
			return "file"
		default:
			return ""
		}
	}, 0)
	if err != nil {
		t.Fatalf("ParseOptions: %v", err)
	}
	if options.ArtifactsDir != "/tmp/artifacts" || options.Backend != "file" {
		t.Fatalf("unexpected options: %+v", options)
	}
}

func TestLoadConfigBuildsStoreFromFlags(t *testing.T) {
	artifactsDir := t.TempDir()
	cfg, err := LoadConfig(context.Background(), []string{"--artifacts-backend", "file", "--artifacts-dir", artifactsDir}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.Store.(*artifacts.FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", cfg.Store)
	}
}

func TestLoadConfigRequiresArtifactsDirForFileBackend(t *testing.T) {
	_, err := LoadConfig(context.Background(), nil, func(string) string { return "" })
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAppRunRebuildsSelectedPackage(t *testing.T) {
	store := artifacts.NewMemoryStore()
	putTestVersionedDetail(t, store, protocol.VersionedPackageDetail{
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
		Assets:        []protocol.VersionedAsset{{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/dist/ExampleSans-Regular.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11}},
	})

	result, err := App{Rebuilder: Rebuilder{Store: store}}.Run(context.Background(), "example/family")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Packages != 1 || result.Versions != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.RootIndexPath()); !ok {
		t.Fatalf("root index not written")
	}
}

func putTestVersionedDetail(t *testing.T, store *artifacts.MemoryStore, detail protocol.VersionedPackageDetail) {
	t.Helper()
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}
}
