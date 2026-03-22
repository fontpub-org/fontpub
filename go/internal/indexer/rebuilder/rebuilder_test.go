package rebuilder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type failingStore struct {
	base               *artifacts.MemoryStore
	listAllErr         error
	putPackageIndexErr error
	putLatestAliasErr  error
	putRootIndexErr    error
}

func (s failingStore) GetVersionedPackageDetail(ctx context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error) {
	return s.base.GetVersionedPackageDetail(ctx, packageID, versionKey)
}

func (s failingStore) PutVersionedPackageDetail(ctx context.Context, detail protocol.VersionedPackageDetail, body []byte, etag string) error {
	return s.base.PutVersionedPackageDetail(ctx, detail, body, etag)
}

func (s failingStore) ListPackageVersionedPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, error) {
	return s.base.ListPackageVersionedPackageDetails(ctx, packageID)
}

func (s failingStore) ListAllVersionedPackageDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error) {
	if s.listAllErr != nil {
		return nil, s.listAllErr
	}
	return s.base.ListAllVersionedPackageDetails(ctx)
}

func (s failingStore) PutPackageVersionsIndex(ctx context.Context, packageID string, index protocol.PackageVersionsIndex, body []byte, etag string) error {
	if s.putPackageIndexErr != nil {
		return s.putPackageIndexErr
	}
	return s.base.PutPackageVersionsIndex(ctx, packageID, index, body, etag)
}

func (s failingStore) PutLatestAlias(ctx context.Context, packageID string, body []byte, etag string) error {
	if s.putLatestAliasErr != nil {
		return s.putLatestAliasErr
	}
	return s.base.PutLatestAlias(ctx, packageID, body, etag)
}

func (s failingStore) PutRootIndex(ctx context.Context, index protocol.RootIndex, body []byte, etag string) error {
	if s.putRootIndexErr != nil {
		return s.putRootIndexErr
	}
	return s.base.PutRootIndex(ctx, index, body, etag)
}

func (s failingStore) GetDocument(ctx context.Context, path string) (artifacts.Document, bool, error) {
	return s.base.GetDocument(ctx, path)
}

func TestRebuildAllMatchesGolden(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detailBytes := readGoldenFile(t, "indexer-publish-versioned-package-detail.json")
	var detail protocol.VersionedPackageDetail
	if err := json.Unmarshal(detailBytes, &detail); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, detailBytes, derive.ComputeETag(detailBytes)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	result, err := (Rebuilder{Store: store}).RebuildAll(context.Background())
	if err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	if result.Packages != 1 || result.Versions != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	assertGoldenDocument(t, store, artifacts.PackageVersionsIndexPath("example/family"), "indexer-publish-package-versions-index.json")
	assertGoldenDocument(t, store, artifacts.RootIndexPath(), "indexer-publish-root-index.json")

	latest, ok, err := store.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family"))
	if err != nil || !ok {
		t.Fatalf("GetDocument latest: ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(latest.Body, detailBytes) {
		t.Fatalf("latest alias must match authoritative document")
	}
}

func TestRebuildPackageRestoresDeletedDerivedDocuments(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detailBytes := readGoldenFile(t, "indexer-publish-versioned-package-detail.json")
	var detail protocol.VersionedPackageDetail
	if err := json.Unmarshal(detailBytes, &detail); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, detailBytes, derive.ComputeETag(detailBytes)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	rebuilder := Rebuilder{Store: store}
	if _, err := rebuilder.RebuildAll(context.Background()); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}
	store.DeleteDocument(artifacts.PackageVersionsIndexPath("example/family"))
	store.DeleteDocument(artifacts.LatestAliasPath("example/family"))
	store.DeleteDocument(artifacts.RootIndexPath())

	if _, err := rebuilder.RebuildPackage(context.Background(), "example/family"); err != nil {
		t.Fatalf("RebuildPackage: %v", err)
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family")); !ok {
		t.Fatalf("package index not restored")
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family")); !ok {
		t.Fatalf("latest alias not restored")
	}
	if _, ok, _ := store.GetDocument(context.Background(), artifacts.RootIndexPath()); !ok {
		t.Fatalf("root index not restored")
	}
}

func TestRebuildAllIsIdempotent(t *testing.T) {
	store := artifacts.NewMemoryStore()
	putVersionedDetail(t, store, protocol.VersionedPackageDetail{
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

	rebuilder := Rebuilder{Store: store}
	if _, err := rebuilder.RebuildAll(context.Background()); err != nil {
		t.Fatalf("first RebuildAll: %v", err)
	}
	firstPackage, _, _ := store.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family"))
	firstLatest, _, _ := store.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family"))
	firstRoot, _, _ := store.GetDocument(context.Background(), artifacts.RootIndexPath())

	if _, err := rebuilder.RebuildAll(context.Background()); err != nil {
		t.Fatalf("second RebuildAll: %v", err)
	}
	secondPackage, _, _ := store.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family"))
	secondLatest, _, _ := store.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family"))
	secondRoot, _, _ := store.GetDocument(context.Background(), artifacts.RootIndexPath())

	if !bytes.Equal(firstPackage.Body, secondPackage.Body) || firstPackage.ETag != secondPackage.ETag {
		t.Fatalf("package index changed across rerun")
	}
	if !bytes.Equal(firstLatest.Body, secondLatest.Body) || firstLatest.ETag != secondLatest.ETag {
		t.Fatalf("latest alias changed across rerun")
	}
	if !bytes.Equal(firstRoot.Body, secondRoot.Body) || firstRoot.ETag != secondRoot.ETag {
		t.Fatalf("root index changed across rerun")
	}
}

func TestRebuildAllChoosesLatestVersionByPrecedence(t *testing.T) {
	store := artifacts.NewMemoryStore()
	putVersionedDetail(t, store, protocol.VersionedPackageDetail{
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
	putVersionedDetail(t, store, protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     "example/family",
		DisplayName:   "Example Sans",
		Author:        "Example Studio",
		License:       "OFL-1.1",
		Version:       "1.10.0",
		VersionKey:    "1.10.0",
		PublishedAt:   "2026-01-03T00:00:00Z",
		GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "89abcdef0123456789abcdef0123456789abcdef"},
		ManifestURL:   "https://raw.githubusercontent.com/example/family/89abcdef0123456789abcdef0123456789abcdef/fontpub.json",
		Assets:        []protocol.VersionedAsset{{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.githubusercontent.com/example/family/89abcdef0123456789abcdef0123456789abcdef/dist/ExampleSans-Regular.otf", SHA256: "def", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11}},
	})

	if _, err := (Rebuilder{Store: store}).RebuildAll(context.Background()); err != nil {
		t.Fatalf("RebuildAll: %v", err)
	}

	doc, ok, err := store.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family"))
	if err != nil || !ok {
		t.Fatalf("GetDocument package index: ok=%v err=%v", ok, err)
	}
	var index protocol.PackageVersionsIndex
	if err := json.Unmarshal(doc.Body, &index); err != nil {
		t.Fatalf("json.Unmarshal package index: %v", err)
	}
	if index.LatestVersion != "1.10.0" || index.LatestVersionKey != "1.10.0" {
		t.Fatalf("unexpected latest version: %+v", index)
	}
}

func TestRebuilderFailurePathsAndScope(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		if _, err := (Rebuilder{}).RebuildAll(context.Background()); err == nil {
			t.Fatalf("expected nil store error")
		}
		if _, err := (Rebuilder{}).RebuildPackage(context.Background(), "example/family"); err == nil {
			t.Fatalf("expected nil store error")
		}
	})

	t.Run("package not found", func(t *testing.T) {
		store := artifacts.NewMemoryStore()
		if _, err := (Rebuilder{Store: store}).RebuildPackage(context.Background(), "missing/package"); err == nil {
			t.Fatalf("expected missing package error")
		}
	})

	t.Run("list all failure", func(t *testing.T) {
		store := failingStore{base: artifacts.NewMemoryStore(), listAllErr: errors.New("boom")}
		if _, err := (Rebuilder{Store: store}).RebuildAll(context.Background()); err == nil {
			t.Fatalf("expected list failure")
		}
	})

	t.Run("package write failure", func(t *testing.T) {
		base := artifacts.NewMemoryStore()
		putVersionedDetail(t, base, protocol.VersionedPackageDetail{
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
		store := failingStore{base: base, putPackageIndexErr: errors.New("boom")}
		if _, err := (Rebuilder{Store: store}).RebuildAll(context.Background()); err == nil {
			t.Fatalf("expected package write failure")
		}
	})

	t.Run("root write failure", func(t *testing.T) {
		base := artifacts.NewMemoryStore()
		putVersionedDetail(t, base, protocol.VersionedPackageDetail{
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
		store := failingStore{base: base, putRootIndexErr: errors.New("boom")}
		if _, err := (Rebuilder{Store: store}).RebuildAll(context.Background()); err == nil {
			t.Fatalf("expected root write failure")
		}
	})

	t.Run("multiple package scope", func(t *testing.T) {
		grouped := groupPackageDetails([]protocol.VersionedPackageDetail{
			scopedDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
			scopedDetail("example/serif", "2.0.0", "2026-01-03T00:00:00Z"),
			scopedDetail("example/serif", "2.1.0", "2026-01-04T00:00:00Z"),
		})
		packageIDs, versions, err := rebuildScope(grouped, "")
		if err != nil {
			t.Fatalf("rebuildScope: %v", err)
		}
		if len(packageIDs) != 2 || packageIDs[0] != "example/family" || packageIDs[1] != "example/serif" || versions != 3 {
			t.Fatalf("unexpected scope: packageIDs=%v versions=%d", packageIDs, versions)
		}
	})
}

func putVersionedDetail(t *testing.T, store *artifacts.MemoryStore, detail protocol.VersionedPackageDetail) {
	t.Helper()
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}
}

func assertGoldenDocument(t *testing.T, store *artifacts.MemoryStore, path, goldenName string) {
	t.Helper()
	doc, ok, err := store.GetDocument(context.Background(), path)
	if err != nil || !ok {
		t.Fatalf("GetDocument %s: ok=%v err=%v", path, ok, err)
	}
	golden := readGoldenFile(t, goldenName)
	if !bytes.Equal(doc.Body, golden) {
		t.Fatalf("golden mismatch for %s\ngot: %s\nwant: %s", path, doc.Body, golden)
	}
}

func scopedDetail(packageID, version, publishedAt string) protocol.VersionedPackageDetail {
	return protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     packageID,
		DisplayName:   "Example Sans",
		Author:        "Example Studio",
		License:       "OFL-1.1",
		Version:       version,
		VersionKey:    version,
		PublishedAt:   publishedAt,
		GitHub:        protocol.GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
		ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
		Assets:        []protocol.VersionedAsset{{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/dist/ExampleSans-Regular.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11}},
	}
}

func readGoldenFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "protocol", "golden", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%s): %v", name, err)
	}
	return bytes.TrimSuffix(data, []byte("\n"))
}
