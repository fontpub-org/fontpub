package artifacts

import (
	"context"
	"testing"

	"github.com/ma/fontpub/go/internal/indexer/derive"
	"github.com/ma/fontpub/go/internal/protocol"
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
