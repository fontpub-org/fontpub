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
