package artifacts

import (
	"context"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestMemoryStoreVersionedAndDocuments(t *testing.T) {
	store := NewMemoryStore()
	detail := protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     "example/family",
		Version:       "1.2.3",
		VersionKey:    "1.2.3",
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, []byte(`{"ok":true}`), `"etag-1"`); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}
	got, ok, err := store.GetVersionedPackageDetail(context.Background(), "example/family", "1.2.3")
	if err != nil || !ok || got.VersionKey != "1.2.3" {
		t.Fatalf("unexpected get result: %+v ok=%v err=%v", got, ok, err)
	}
	doc, ok, err := store.GetDocument(context.Background(), VersionedPackageDetailPath("example/family", "1.2.3"))
	if err != nil || !ok || doc.ETag != `"etag-1"` {
		t.Fatalf("unexpected doc result: %+v ok=%v err=%v", doc, ok, err)
	}
}

func TestMemoryStoreFailNextWrite(t *testing.T) {
	store := NewMemoryStore()
	path := RootIndexPath()
	store.FailNextWrite(path, 1)
	if err := store.PutRootIndex(context.Background(), protocol.RootIndex{}, []byte(`{}`), `"etag"`); err == nil {
		t.Fatalf("expected fail next write")
	}
	if err := store.PutRootIndex(context.Background(), protocol.RootIndex{}, []byte(`{}`), `"etag"`); err != nil {
		t.Fatalf("unexpected second write failure: %v", err)
	}
}

func TestMemoryStoreListsPackageVersionsAndDerivedDocuments(t *testing.T) {
	store := NewMemoryStore()
	for _, detail := range []protocol.VersionedPackageDetail{
		{SchemaVersion: "1", PackageID: "example/family", Version: "1.10.0", VersionKey: "1.10.0"},
		{SchemaVersion: "1", PackageID: "example/family", Version: "1.2.3", VersionKey: "1.2.3"},
		{SchemaVersion: "1", PackageID: "example/serif", Version: "2.0.0", VersionKey: "2.0.0"},
	} {
		if err := store.PutVersionedPackageDetail(context.Background(), detail, []byte(`{"ok":true}`), `"etag"`); err != nil {
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
	all, err := store.ListAllVersionedPackageDetails(context.Background())
	if err != nil {
		t.Fatalf("ListAllVersionedPackageDetails: %v", err)
	}
	if len(all) != 3 || all[0].PackageID != "example/family" || all[2].PackageID != "example/serif" {
		t.Fatalf("unexpected all list: %+v", all)
	}
	if err := store.PutPackageVersionsIndex(context.Background(), "example/family", protocol.PackageVersionsIndex{}, []byte(`{"index":true}`), `"etag-index"`); err != nil {
		t.Fatalf("PutPackageVersionsIndex: %v", err)
	}
	if err := store.PutLatestAlias(context.Background(), "example/family", []byte(`{"latest":true}`), `"etag-latest"`); err != nil {
		t.Fatalf("PutLatestAlias: %v", err)
	}
	indexDoc, ok, err := store.GetDocument(context.Background(), PackageVersionsIndexPath("example/family"))
	if err != nil || !ok || indexDoc.ETag != `"etag-index"` {
		t.Fatalf("unexpected index doc: %+v ok=%v err=%v", indexDoc, ok, err)
	}
	latestDoc, ok, err := store.GetDocument(context.Background(), LatestAliasPath("example/family"))
	if err != nil || !ok || latestDoc.ETag != `"etag-latest"` {
		t.Fatalf("unexpected latest doc: %+v ok=%v err=%v", latestDoc, ok, err)
	}
}

func TestArtifactPathHelpers(t *testing.T) {
	if got := PackageVersionsIndexPath("example/family"); got != "/v1/packages/example/family/index.json" {
		t.Fatalf("unexpected package index path: %s", got)
	}
	if got := LatestAliasPath("example/family"); got != "/v1/packages/example/family.json" {
		t.Fatalf("unexpected latest alias path: %s", got)
	}
}
