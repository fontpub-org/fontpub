package updateapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestReadRootIndexSupportsConditionalGET(t *testing.T) {
	store := artifacts.NewMemoryStore()
	root := protocol.RootIndex{
		SchemaVersion: "1",
		GeneratedAt:   "2026-01-02T00:00:00Z",
		Packages: map[string]protocol.RootIndexPackage{
			"example/family": {
				LatestVersion:     "1.2.3",
				LatestVersionKey:  "1.2.3",
				LatestPublishedAt: "2026-01-02T00:00:00Z",
			},
		},
	}
	body, err := protocol.MarshalCanonical(root)
	if err != nil {
		t.Fatalf("MarshalCanonical(root): %v", err)
	}
	etag := derive.ComputeETag(body)
	if err := store.PutRootIndex(context.Background(), root, body, etag); err != nil {
		t.Fatalf("PutRootIndex: %v", err)
	}

	server := Server{ArtifactStore: store}
	req := httptest.NewRequest(http.MethodGet, "/v1/index.json", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("ETag"); got != etag {
		t.Fatalf("etag=%q want %q", got, etag)
	}
	if rr.Body.String() != string(body) {
		t.Fatalf("body=%s want %s", rr.Body.String(), string(body))
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/index.json", nil)
	req.Header.Set("If-None-Match", etag)
	rr = httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotModified {
		t.Fatalf("status=%d want %d", rr.Code, http.StatusNotModified)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("content-type=%q want %q", got, "application/json; charset=utf-8")
	}
	if got := rr.Header().Get("ETag"); got != etag {
		t.Fatalf("etag=%q want %q", got, etag)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestReadRootIndexFallsBackToDerivedDocument(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detail := testReadDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z")
	detailBody, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical(detail): %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, detailBody, derive.ComputeETag(detailBody)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/index.json", nil)
	rr := httptest.NewRecorder()
	Server{ArtifactStore: store}.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var root protocol.RootIndex
	if err := json.Unmarshal(rr.Body.Bytes(), &root); err != nil {
		t.Fatalf("json.Unmarshal(root): %v", err)
	}
	if root.GeneratedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("unexpected generated_at: %s", root.GeneratedAt)
	}
	if got := root.Packages["example/family"].LatestVersionKey; got != "1.2.3" {
		t.Fatalf("unexpected latest version key: %s", got)
	}
}

func TestReadRootIndexFallsBackToEmptyCanonicalDocument(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/index.json", nil)
	rr := httptest.NewRecorder()
	Server{ArtifactStore: artifacts.NewMemoryStore()}.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var root protocol.RootIndex
	if err := json.Unmarshal(rr.Body.Bytes(), &root); err != nil {
		t.Fatalf("json.Unmarshal(root): %v", err)
	}
	if root.GeneratedAt != "1970-01-01T00:00:00Z" || len(root.Packages) != 0 {
		t.Fatalf("unexpected root index: %+v", root)
	}
}

func TestReadPackageDocumentsFallBackToDerivedDocuments(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detail := testReadDetail("example/family", "1.10.0", "2026-01-03T00:00:00Z")
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical(detail): %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "latest alias", path: "/v1/packages/example/family.json"},
		{name: "package index", path: "/v1/packages/example/family/index.json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			Server{ArtifactStore: store}.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestReadPackageDocumentsReturnContentTypeOnConditionalGET(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detail := testReadDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z")
	detailBody, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical(detail): %v", err)
	}
	detailETag := derive.ComputeETag(detailBody)
	if err := store.PutVersionedPackageDetail(context.Background(), detail, detailBody, detailETag); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	index, latestDetail, err := derive.BuildPackageVersionsIndex(detail.PackageID, []protocol.VersionedPackageDetail{detail})
	if err != nil {
		t.Fatalf("BuildPackageVersionsIndex: %v", err)
	}
	indexBody, err := protocol.MarshalCanonical(index)
	if err != nil {
		t.Fatalf("MarshalCanonical(index): %v", err)
	}
	indexETag := derive.ComputeETag(indexBody)
	if err := store.PutPackageVersionsIndex(context.Background(), detail.PackageID, index, indexBody, indexETag); err != nil {
		t.Fatalf("PutPackageVersionsIndex: %v", err)
	}
	latestBody, err := protocol.MarshalCanonical(latestDetail)
	if err != nil {
		t.Fatalf("MarshalCanonical(latest): %v", err)
	}
	latestETag := derive.ComputeETag(latestBody)
	if err := store.PutLatestAlias(context.Background(), latestDetail.PackageID, latestBody, latestETag); err != nil {
		t.Fatalf("PutLatestAlias: %v", err)
	}

	tests := []struct {
		name string
		path string
		etag string
	}{
		{name: "latest alias", path: "/v1/packages/example/family.json", etag: latestETag},
		{name: "package index", path: "/v1/packages/example/family/index.json", etag: indexETag},
		{name: "version detail", path: "/v1/packages/example/family/versions/1.2.3.json", etag: detailETag},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("If-None-Match", tc.etag)
			rr := httptest.NewRecorder()
			Server{ArtifactStore: store}.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusNotModified {
				t.Fatalf("status=%d want %d", rr.Code, http.StatusNotModified)
			}
			if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("content-type=%q want %q", got, "application/json; charset=utf-8")
			}
			if got := rr.Header().Get("ETag"); got != tc.etag {
				t.Fatalf("etag=%q want %q", got, tc.etag)
			}
			if rr.Body.Len() != 0 {
				t.Fatalf("unexpected body: %s", rr.Body.String())
			}
		})
	}
}

func TestReadVersionedPackageDetailDistinguishesPackageAndVersionNotFound(t *testing.T) {
	store := artifacts.NewMemoryStore()
	detail := testReadDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z")
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical(detail): %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantCode string
	}{
		{name: "missing version", path: "/v1/packages/example/family/versions/9.9.9.json", wantCode: "VERSION_NOT_FOUND"},
		{name: "missing package", path: "/v1/packages/example/serif/versions/1.0.0.json", wantCode: "PACKAGE_NOT_FOUND"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			Server{ArtifactStore: store}.Handler().ServeHTTP(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			var env protocol.ErrorEnvelope
			if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
				t.Fatalf("json.Unmarshal(error): %v", err)
			}
			if env.Error.Code != tc.wantCode {
				t.Fatalf("code=%s want %s", env.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestHandleUpdateSetsRetryAfterForIndexConflict(t *testing.T) {
	server := Server{
		Verifier: fakeVerifier{claims: protocol.OIDCClaims{
			Sub:             "repo:example/family:ref:refs/tags/v1.2.3",
			Repository:      "example/family",
			RepositoryID:    "123456789",
			RepositoryOwner: "example",
			SHA:             "0123456789abcdef0123456789abcdef01234567",
			Ref:             "refs/tags/v1.2.3",
			WorkflowRef:     "example/family/.github/workflows/fontpub.yml@refs/heads/main",
			WorkflowSHA:     "89abcdef0123456789abcdef0123456789abcdef",
			JTI:             "jwt-id-1",
			EventName:       "push",
		}},
		Processor: &fakeProcessor{
			status: http.StatusServiceUnavailable,
			body: protocol.ErrorEnvelope{
				Error: protocol.ErrorObject{
					Code:    "INDEX_CONFLICT",
					Message: "could not preserve derived document consistency",
					Details: map[string]any{},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/update", strings.NewReader(`{"repository":"example/family","sha":"0123456789abcdef0123456789abcdef01234567","ref":"refs/tags/v1.2.3"}`))
	req.Header.Set("Authorization", "Bearer token-1")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("retry-after=%q want %q", got, "1")
	}
}

func testReadDetail(packageID, version, publishedAt string) protocol.VersionedPackageDetail {
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
