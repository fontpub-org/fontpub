package deriveddocs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestWritePackage(t *testing.T) {
	store := artifacts.NewMemoryStore()
	details := []protocol.VersionedPackageDetail{
		testDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
		testDetail("example/family", "1.10.0", "2026-01-03T00:00:00Z"),
	}

	result, err := WritePackage(context.Background(), store, "example/family", details)
	if err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	if result.PackageIndexETag == "" || result.LatestAliasETag == "" {
		t.Fatalf("expected etags, got %+v", result)
	}
	if result.LatestDetail.VersionKey != "1.10.0" {
		t.Fatalf("unexpected latest detail: %+v", result.LatestDetail)
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family")); !ok {
		t.Fatalf("package index not written")
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family")); !ok {
		t.Fatalf("latest alias not written")
	}
}

func TestWritePackagePropagatesStoreErrors(t *testing.T) {
	store := artifacts.NewMemoryStore()
	store.FailNextWrite(artifacts.PackageVersionsIndexPath("example/family"), 1)

	_, err := WritePackage(context.Background(), store, "example/family", []protocol.VersionedPackageDetail{
		testDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteRoot(t *testing.T) {
	store := artifacts.NewMemoryStore()
	details := []protocol.VersionedPackageDetail{
		testDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
		testDetail("example/serif", "2.0.0", "2026-01-04T00:00:00Z"),
	}

	etag, err := WriteRoot(context.Background(), store, details)
	if err != nil {
		t.Fatalf("WriteRoot: %v", err)
	}
	if etag == "" {
		t.Fatalf("expected root etag")
	}
	doc, ok, err := store.GetDocument(context.Background(), artifacts.RootIndexPath())
	if err != nil || !ok {
		t.Fatalf("GetDocument(root): ok=%v err=%v", ok, err)
	}
	var root protocol.RootIndex
	if err := json.Unmarshal(doc.Body, &root); err != nil {
		t.Fatalf("json.Unmarshal(root): %v", err)
	}
	if root.GeneratedAt != "2026-01-04T00:00:00Z" {
		t.Fatalf("unexpected root index: %+v", root)
	}
}

func testDetail(packageID, version, publishedAt string) protocol.VersionedPackageDetail {
	return protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     packageID,
		DisplayName:   "Example Sans",
		Author:        "Example Studio",
		License:       "OFL-1.1",
		Version:       version,
		VersionKey:    version,
		PublishedAt:   publishedAt,
		GitHub: protocol.GitHubRef{
			Owner: "example",
			Repo:  "family",
			SHA:   "0123456789abcdef0123456789abcdef01234567",
		},
		ManifestURL: "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
		Assets: []protocol.VersionedAsset{
			{
				Path:      "dist/ExampleSans-Regular.otf",
				URL:       "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/dist/ExampleSans-Regular.otf",
				SHA256:    "abc",
				Format:    "otf",
				Style:     "normal",
				Weight:    400,
				SizeBytes: 11,
			},
		},
	}
}
