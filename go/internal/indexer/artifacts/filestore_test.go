package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestFileStoreRoundTrip(t *testing.T) {
	store := NewFileStore(t.TempDir())
	detail := protocol.VersionedPackageDetail{
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
	}
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	got, ok, err := store.GetVersionedPackageDetail(context.Background(), "example/family", "1.2.3")
	if err != nil || !ok {
		t.Fatalf("GetVersionedPackageDetail: ok=%v err=%v", ok, err)
	}
	if got.PackageID != detail.PackageID || got.VersionKey != detail.VersionKey {
		t.Fatalf("unexpected detail: %+v", got)
	}

	list, err := store.ListAllVersionedPackageDetails(context.Background())
	if err != nil {
		t.Fatalf("ListAllVersionedPackageDetails: %v", err)
	}
	if len(list) != 1 || list[0].VersionKey != "1.2.3" {
		t.Fatalf("unexpected list: %+v", list)
	}

	doc, ok, err := store.GetDocument(context.Background(), VersionedPackageDetailPath("example/family", "1.2.3"))
	if err != nil || !ok {
		t.Fatalf("GetDocument: ok=%v err=%v", ok, err)
	}
	if doc.ETag != derive.ComputeETag(body) {
		t.Fatalf("unexpected etag: %s", doc.ETag)
	}
}

func TestFileStoreListAllWithNonCleanRoot(t *testing.T) {
	root := t.TempDir() + "/./nested/.."
	store := NewFileStore(root)
	detail := protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     "0xtype/gamut",
		DisplayName:   "Zx Gamut",
		Author:        "0xType",
		License:       "OFL-1.1",
		Version:       "1.002",
		VersionKey:    "1.002",
		PublishedAt:   "2026-03-19T00:00:00Z",
		GitHub:        protocol.GitHubRef{Owner: "0xtype", Repo: "gamut", SHA: "0123456789abcdef0123456789abcdef01234567"},
		ManifestURL:   "https://raw.githubusercontent.com/0xtype/gamut/0123456789abcdef0123456789abcdef01234567/fontpub.json",
		Assets:        []protocol.VersionedAsset{{Path: "fonts/static/ZxGamut-Bold.otf", URL: "https://raw.githubusercontent.com/0xtype/gamut/0123456789abcdef0123456789abcdef01234567/fonts/static/ZxGamut-Bold.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 700, SizeBytes: 11}},
	}
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	list, err := store.ListAllVersionedPackageDetails(context.Background())
	if err != nil {
		t.Fatalf("ListAllVersionedPackageDetails: %v", err)
	}
	if len(list) != 1 || list[0].PackageID != "0xtype/gamut" {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestFileStoreListsPackageVersionsAndDerivedDocuments(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	for _, detail := range []protocol.VersionedPackageDetail{
		{
			SchemaVersion: "1",
			PackageID:     "example/family",
			DisplayName:   "Example Sans",
			Author:        "Example Studio",
			License:       "OFL-1.1",
			Version:       "1.10.0",
			VersionKey:    "1.10.0",
			PublishedAt:   "2026-01-03T00:00:00Z",
		},
		{
			SchemaVersion: "1",
			PackageID:     "example/family",
			DisplayName:   "Example Sans",
			Author:        "Example Studio",
			License:       "OFL-1.1",
			Version:       "1.2.3",
			VersionKey:    "1.2.3",
			PublishedAt:   "2026-01-02T00:00:00Z",
		},
	} {
		body, err := protocol.MarshalCanonical(detail)
		if err != nil {
			t.Fatalf("MarshalCanonical: %v", err)
		}
		if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
			t.Fatalf("PutVersionedPackageDetail: %v", err)
		}
	}
	list, err := store.ListPackageVersionedPackageDetails(context.Background(), "example/family")
	if err != nil {
		t.Fatalf("ListPackageVersionedPackageDetails: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}
	if err := store.PutPackageVersionsIndex(context.Background(), "example/family", protocol.PackageVersionsIndex{}, []byte(`{"index":true}`), `"etag"`); err != nil {
		t.Fatalf("PutPackageVersionsIndex: %v", err)
	}
	if err := store.PutLatestAlias(context.Background(), "example/family", []byte(`{"latest":true}`), `"etag"`); err != nil {
		t.Fatalf("PutLatestAlias: %v", err)
	}
	if err := store.PutRootIndex(context.Background(), protocol.RootIndex{}, []byte(`{"root":true}`), `"etag"`); err != nil {
		t.Fatalf("PutRootIndex: %v", err)
	}
	for _, path := range []string{
		PackageVersionsIndexPath("example/family"),
		LatestAliasPath("example/family"),
		RootIndexPath(),
	} {
		if _, ok, err := store.GetDocument(context.Background(), path); err != nil || !ok {
			t.Fatalf("GetDocument(%s): ok=%v err=%v", path, ok, err)
		}
	}
}

func TestFileStoreGetDocumentHandlesNotFoundAndErrors(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	if _, ok, err := store.GetDocument(context.Background(), RootIndexPath()); err != nil || ok {
		t.Fatalf("expected missing document, ok=%v err=%v", ok, err)
	}
	target := filepath.Join(root, "v1", "index.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, ok, err := store.GetDocument(context.Background(), RootIndexPath()); err == nil || ok {
		t.Fatalf("expected read error, ok=%v err=%v", ok, err)
	}
}
