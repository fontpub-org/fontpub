package updateapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type fixedClock struct {
	t time.Time
}

func (c fixedClock) Now() time.Time { return c.t }

type failingArtifactStore struct {
	base                *artifacts.MemoryStore
	getVersionedErr     error
	listPackageErr      error
	listAllErr          error
	putVersionedErr     error
	putPackageIndexErr  error
	putLatestAliasErr   error
	putRootIndexErr     error
	getVersionedFound   bool
	getVersionedDetail  protocol.VersionedPackageDetail
	overrideListPackage []protocol.VersionedPackageDetail
	overrideListAll     []protocol.VersionedPackageDetail
}

func (s failingArtifactStore) GetVersionedPackageDetail(ctx context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error) {
	if s.getVersionedErr != nil {
		return protocol.VersionedPackageDetail{}, false, s.getVersionedErr
	}
	if s.getVersionedFound {
		return s.getVersionedDetail, true, nil
	}
	return s.base.GetVersionedPackageDetail(ctx, packageID, versionKey)
}

func (s failingArtifactStore) PutVersionedPackageDetail(ctx context.Context, detail protocol.VersionedPackageDetail, body []byte, etag string) error {
	if s.putVersionedErr != nil {
		return s.putVersionedErr
	}
	return s.base.PutVersionedPackageDetail(ctx, detail, body, etag)
}

func (s failingArtifactStore) ListPackageVersionedPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, error) {
	if s.listPackageErr != nil {
		return nil, s.listPackageErr
	}
	if s.overrideListPackage != nil {
		return s.overrideListPackage, nil
	}
	return s.base.ListPackageVersionedPackageDetails(ctx, packageID)
}

func (s failingArtifactStore) ListAllVersionedPackageDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error) {
	if s.listAllErr != nil {
		return nil, s.listAllErr
	}
	if s.overrideListAll != nil {
		return s.overrideListAll, nil
	}
	return s.base.ListAllVersionedPackageDetails(ctx)
}

func (s failingArtifactStore) PutPackageVersionsIndex(ctx context.Context, packageID string, index protocol.PackageVersionsIndex, body []byte, etag string) error {
	if s.putPackageIndexErr != nil {
		return s.putPackageIndexErr
	}
	return s.base.PutPackageVersionsIndex(ctx, packageID, index, body, etag)
}

func (s failingArtifactStore) PutLatestAlias(ctx context.Context, packageID string, body []byte, etag string) error {
	if s.putLatestAliasErr != nil {
		return s.putLatestAliasErr
	}
	return s.base.PutLatestAlias(ctx, packageID, body, etag)
}

func (s failingArtifactStore) PutRootIndex(ctx context.Context, index protocol.RootIndex, body []byte, etag string) error {
	if s.putRootIndexErr != nil {
		return s.putRootIndexErr
	}
	return s.base.PutRootIndex(ctx, index, body, etag)
}

func (s failingArtifactStore) GetDocument(ctx context.Context, path string) (artifacts.Document, bool, error) {
	return s.base.GetDocument(ctx, path)
}

func TestPublishingProcessorSuccess(t *testing.T) {
	processor := newPublishingProcessor(t)
	req, claims := validRequestAndClaims()

	status, body := processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%#v", status, body)
	}
	resp := body.(map[string]any)
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}

	doc, ok, err := processor.ArtifactStore.GetDocument(context.Background(), artifacts.VersionedPackageDetailPath("example/family", "1.2.3"))
	if err != nil || !ok || doc.ETag == "" {
		t.Fatalf("versioned doc missing: %+v ok=%v err=%v", doc, ok, err)
	}
	latest, ok, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.LatestAliasPath("example/family"))
	if !ok || string(latest.Body) != string(doc.Body) {
		t.Fatalf("latest alias must be byte-identical")
	}
}

func TestPublishingProcessorIdempotentReplay(t *testing.T) {
	processor := newPublishingProcessor(t)
	req, claims := validRequestAndClaims()

	status, body := processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("first publish failed: %v %#v", status, body)
	}
	doc1, _, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.VersionedPackageDetailPath("example/family", "1.2.3"))

	claims.JTI = "jwt-id-2"
	status, body = processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("second publish failed: %v %#v", status, body)
	}
	doc2, _, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.VersionedPackageDetailPath("example/family", "1.2.3"))
	if string(doc1.Body) != string(doc2.Body) || doc1.ETag != doc2.ETag {
		t.Fatalf("idempotent replay changed authoritative artifact")
	}
}

func TestPublishingProcessorImmutableConflict(t *testing.T) {
	processor := newPublishingProcessor(t)
	req, claims := validRequestAndClaims()

	status, _ := processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("first publish failed")
	}

	claims.JTI = "jwt-id-2"
	processor.Fetcher = fakeFetcher{
		results: map[string]githubraw.Result{
			mustManifestURL(t, req): {
				Body: []byte(`{"name":"Example Sans","author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":700}]}`),
				Size: 158,
			},
			mustAssetURL(t, req, "dist/ExampleSans-Regular.otf"): {Body: []byte("asset-bytes"), Size: 11},
		},
		errors: map[string]error{},
	}
	status, body := processor.Process(context.Background(), req, claims)
	if status != http.StatusConflict {
		t.Fatalf("status=%d body=%#v", status, body)
	}
	env := body.(protocol.ErrorEnvelope)
	if env.Error.Code != "IMMUTABLE_VERSION" {
		t.Fatalf("unexpected error code: %s", env.Error.Code)
	}
}

func TestPublishingProcessorRetryRepairsDerivedDocuments(t *testing.T) {
	processor := newPublishingProcessor(t)
	req, claims := validRequestAndClaims()
	memStore := processor.ArtifactStore.(*artifacts.MemoryStore)
	memStore.FailNextWrite(artifacts.PackageVersionsIndexPath("example/family"), 1)

	status, body := processor.Process(context.Background(), req, claims)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d body=%#v", status, body)
	}
	if _, ok, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.VersionedPackageDetailPath("example/family", "1.2.3")); !ok {
		t.Fatalf("authoritative artifact should exist after partial failure")
	}

	claims.JTI = "jwt-id-2"
	status, body = processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("retry should repair derived docs: %d %#v", status, body)
	}
	if _, ok, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.PackageVersionsIndexPath("example/family")); !ok {
		t.Fatalf("package index missing after retry")
	}
	if _, ok, _ := processor.ArtifactStore.GetDocument(context.Background(), artifacts.RootIndexPath()); !ok {
		t.Fatalf("root index missing after retry")
	}
}

func TestPublishingProcessorFileStoreBuildsRootIndex(t *testing.T) {
	store := artifacts.NewFileStore(t.TempDir())
	req := UpdateRequest{
		Repository: "0xtype/gamut",
		SHA:        "2b4873d8275347fe609253d6da1cf9c5a21ec3b9",
		Ref:        "refs/tags/1.002",
	}
	claims := protocol.OIDCClaims{
		Sub:             "repo:0xtype/gamut:ref:refs/tags/1.002",
		Repository:      req.Repository,
		RepositoryID:    "123456789",
		RepositoryOwner: "0xtype",
		SHA:             req.SHA,
		Ref:             req.Ref,
		WorkflowRef:     "0xtype/gamut/.github/workflows/fontpub.yml@refs/heads/main",
		WorkflowSHA:     "89abcdef0123456789abcdef0123456789abcdef",
		JTI:             "jwt-id-1",
		EventName:       "push",
	}
	manifest := protocol.Manifest{
		Name:    "Zx Gamut",
		Author:  "0xType",
		Version: "1.002",
		License: "OFL-1.1",
		Files: []protocol.ManifestFile{
			{Path: "fonts/static/ZxGamut-Bold.otf", Style: "normal", Weight: 700},
		},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	processor := PublishingProcessor{
		ValidationProcessor: ValidationProcessor{
			State: state.NewMemoryStore(),
			Fetcher: fakeFetcher{
				results: map[string]githubraw.Result{
					mustManifestURL(t, req):                               {Body: manifestBytes, Size: int64(len(manifestBytes))},
					mustAssetURL(t, req, "fonts/static/ZxGamut-Bold.otf"): {Body: []byte("asset-bytes"), Size: 11},
				},
				errors: map[string]error{},
			},
		},
		ArtifactStore: store,
		Clock:         fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}

	status, body := processor.Process(context.Background(), req, claims)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%#v", status, body)
	}

	rootDoc, ok, err := store.GetDocument(context.Background(), artifacts.RootIndexPath())
	if err != nil || !ok {
		t.Fatalf("GetDocument(root): ok=%v err=%v", ok, err)
	}
	var root protocol.RootIndex
	if err := json.Unmarshal(rootDoc.Body, &root); err != nil {
		t.Fatalf("json.Unmarshal(root): %v", err)
	}
	pkg, ok := root.Packages["0xtype/gamut"]
	if !ok {
		t.Fatalf("root index missing package: %+v", root)
	}
	if pkg.LatestVersion != "1.002" || pkg.LatestVersionKey != "1.002" {
		t.Fatalf("unexpected root package entry: %+v", pkg)
	}
}

func TestPublishingProcessorPublishFailuresAndLatestAliasBranch(t *testing.T) {
	req, claims := validRequestAndClaims()
	manifest := []byte(`{"name":"Example Sans","author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":400}]}`)
	tests := []struct {
		name       string
		processor  PublishingProcessor
		wantCode   string
		wantStatus int
		checkBody  func(t *testing.T, body any)
	}{
		{
			name:       "missing artifact store",
			processor:  PublishingProcessor{},
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name: "get versioned detail error",
			processor: PublishingProcessor{
				ValidationProcessor: newPublishingProcessor(t).ValidationProcessor,
				ArtifactStore:       failingArtifactStore{base: artifacts.NewMemoryStore(), getVersionedErr: errors.New("boom")},
				Clock:               fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name: "authoritative write conflict",
			processor: PublishingProcessor{
				ValidationProcessor: newPublishingProcessor(t).ValidationProcessor,
				ArtifactStore:       failingArtifactStore{base: artifacts.NewMemoryStore(), putVersionedErr: errors.New("boom")},
				Clock:               fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
			wantCode:   "INDEX_CONFLICT",
			wantStatus: 503,
		},
		{
			name: "list package error",
			processor: PublishingProcessor{
				ValidationProcessor: newPublishingProcessor(t).ValidationProcessor,
				ArtifactStore:       failingArtifactStore{base: artifacts.NewMemoryStore(), listPackageErr: errors.New("boom")},
				Clock:               fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name: "root index write failure after package index success",
			processor: PublishingProcessor{
				ValidationProcessor: newPublishingProcessor(t).ValidationProcessor,
				ArtifactStore:       failingArtifactStore{base: artifacts.NewMemoryStore(), putRootIndexErr: errors.New("boom")},
				Clock:               fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
			wantCode:   "INDEX_CONFLICT",
			wantStatus: 503,
		},
		{
			name: "latest alias not updated for older publish",
			processor: func() PublishingProcessor {
				base := artifacts.NewMemoryStore()
				newer := protocol.VersionedPackageDetail{
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
					Assets:        []protocol.VersionedAsset{{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.githubusercontent.com/example/family/89abcdef0123456789abcdef0123456789abcdef/dist/ExampleSans-Regular.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 400, SizeBytes: 11}},
				}
				body, err := protocol.MarshalCanonical(newer)
				if err != nil {
					t.Fatalf("MarshalCanonical: %v", err)
				}
				if err := base.PutVersionedPackageDetail(context.Background(), newer, body, "etag"); err != nil {
					t.Fatalf("PutVersionedPackageDetail: %v", err)
				}
				return PublishingProcessor{
					ValidationProcessor: ValidationProcessor{
						State: state.NewMemoryStore(),
						Fetcher: fakeFetcher{
							results: map[string]githubraw.Result{
								mustManifestURL(t, req):                              {Body: manifest, Size: int64(len(manifest))},
								mustAssetURL(t, req, "dist/ExampleSans-Regular.otf"): {Body: []byte("asset-bytes"), Size: 11},
							},
						},
					},
					ArtifactStore: base,
					Clock:         fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
				}
			}(),
			wantStatus: 200,
			checkBody: func(t *testing.T, body any) {
				t.Helper()
				resp := body.(map[string]any)
				latest := resp["artifacts"].(map[string]any)["latest_package_alias"].(map[string]any)
				if latest["updated"] != false {
					t.Fatalf("expected latest alias to remain unchanged: %#v", latest)
				}
				if _, ok := latest["etag"]; ok {
					t.Fatalf("unexpected etag on unchanged latest alias: %#v", latest)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body := tc.processor.Process(context.Background(), req, claims)
			if status != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%#v", status, tc.wantStatus, body)
			}
			if tc.wantCode != "" {
				env := body.(protocol.ErrorEnvelope)
				if env.Error.Code != tc.wantCode {
					t.Fatalf("unexpected error code: %s", env.Error.Code)
				}
				return
			}
			if tc.checkBody != nil {
				tc.checkBody(t, body)
			}
		})
	}
}

func newPublishingProcessor(t *testing.T) PublishingProcessor {
	t.Helper()
	req, _ := validRequestAndClaims()
	manifest := protocol.Manifest{
		Name:    "Example Sans",
		Author:  "Example Studio",
		Version: "1.2.3",
		License: "OFL-1.1",
		Files: []protocol.ManifestFile{
			{Path: "dist/ExampleSans-Regular.otf", Style: "normal", Weight: 400},
		},
	}
	manifestBytes, _ := json.Marshal(manifest)
	return PublishingProcessor{
		ValidationProcessor: ValidationProcessor{
			State: state.NewMemoryStore(),
			Fetcher: fakeFetcher{
				results: map[string]githubraw.Result{
					mustManifestURL(t, req):                              {Body: manifestBytes, Size: int64(len(manifestBytes))},
					mustAssetURL(t, req, "dist/ExampleSans-Regular.otf"): {Body: []byte("asset-bytes"), Size: 11},
				},
				errors: map[string]error{},
			},
		},
		ArtifactStore: artifacts.NewMemoryStore(),
		Clock:         fixedClock{t: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
}

func validRequestAndClaims() (UpdateRequest, protocol.OIDCClaims) {
	req := UpdateRequest{
		Repository: "example/family",
		SHA:        "0123456789abcdef0123456789abcdef01234567",
		Ref:        "refs/tags/v1.2.3",
	}
	claims := protocol.OIDCClaims{
		Sub:             "repo:example/family:ref:refs/tags/v1.2.3",
		Repository:      "example/family",
		RepositoryID:    "123456789",
		RepositoryOwner: "example",
		SHA:             req.SHA,
		Ref:             req.Ref,
		WorkflowRef:     "example/family/.github/workflows/fontpub.yml@refs/heads/main",
		WorkflowSHA:     "89abcdef0123456789abcdef0123456789abcdef",
		JTI:             "jwt-id-1",
		EventName:       "push",
	}
	return req, claims
}

func mustManifestURL(t *testing.T, req UpdateRequest) string {
	t.Helper()
	url, err := githubraw.BuildManifestURL(req.Repository, req.SHA)
	if err != nil {
		t.Fatalf("BuildManifestURL: %v", err)
	}
	return url
}

func mustAssetURL(t *testing.T, req UpdateRequest, path string) string {
	t.Helper()
	url, err := githubraw.BuildAssetURL(req.Repository, req.SHA, path)
	if err != nil {
		t.Fatalf("BuildAssetURL: %v", err)
	}
	return url
}
