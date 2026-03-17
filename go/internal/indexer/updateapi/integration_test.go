package updateapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestHandlerPublishMatchesGolden(t *testing.T) {
	req, claims := validRequestAndClaims()
	manifest := protocol.Manifest{
		Name:    "Example Sans",
		Author:  "Example Studio",
		Version: "1.2.3",
		License: "OFL-1.1",
		Files: []protocol.ManifestFile{
			{Path: "dist/ExampleSans-Regular.otf", Style: "normal", Weight: 400},
			{Path: "dist/ExampleSans-Italic.otf", Style: "italic", Weight: 400},
		},
	}
	manifestBytes, _ := json.Marshal(manifest)
	store := artifacts.NewMemoryStore()
	server := Server{
		Verifier: fakeVerifier{claims: claims},
		Processor: PublishingProcessor{
			ValidationProcessor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{
						mustManifestURL(t, req): {Body: manifestBytes, Size: int64(len(manifestBytes))},
						mustAssetURL(t, req, "dist/ExampleSans-Regular.otf"): {
							Body: []byte("regular-bytes"), Size: 13,
						},
						mustAssetURL(t, req, "dist/ExampleSans-Italic.otf"): {
							Body: []byte("italic-bytes"), Size: 12,
						},
					},
					errors: map[string]error{},
				},
			},
			ArtifactStore: store,
			Clock:         fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		},
	}

	body := `{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567","ref":"refs/tags/v1.2.3"}`
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, httpReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	assertGoldenDocument(t, store, artifacts.VersionedPackageDetailPath("example/family", "1.2.3"), "indexer-publish-versioned-package-detail.json")
	assertGoldenDocument(t, store, artifacts.PackageVersionsIndexPath("example/family"), "indexer-publish-package-versions-index.json")
	assertGoldenDocument(t, store, artifacts.RootIndexPath(), "indexer-publish-root-index.json")
}

func assertGoldenDocument(t *testing.T, store *artifacts.MemoryStore, path, goldenName string) {
	t.Helper()
	doc, ok, err := store.GetDocument(context.Background(), path)
	if err != nil || !ok {
		t.Fatalf("missing document path=%s err=%v", path, err)
	}
	golden := readGoldenFile(t, goldenName)
	if !bytes.Equal(doc.Body, golden) {
		t.Fatalf("golden mismatch for %s\n got: %s\nwant: %s", path, doc.Body, golden)
	}
}

func readGoldenFile(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "protocol", "golden", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return bytes.TrimSuffix(data, []byte("\n"))
}
