package updateapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type fakeFetcher struct {
	results map[string]githubraw.Result
	errors  map[string]error
}

func (f fakeFetcher) Fetch(_ context.Context, url string, _ int64) (githubraw.Result, error) {
	if err, ok := f.errors[url]; ok {
		return githubraw.Result{}, err
	}
	if result, ok := f.results[url]; ok {
		return result, nil
	}
	return githubraw.Result{}, githubraw.ErrNotFound
}

type failingStateStore struct {
	replayErr    error
	ownershipErr error
}

func (s failingStateStore) CheckAndReserveJTI(context.Context, string) error {
	return s.replayErr
}

func (s failingStateStore) CheckOrBindPackage(context.Context, string, string) error {
	return s.ownershipErr
}

func TestValidationProcessorValidate(t *testing.T) {
	claims := protocol.OIDCClaims{
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
	}
	req := UpdateRequest{
		Repository: "example/family",
		SHA:        "0123456789abcdef0123456789abcdef01234567",
		Ref:        "refs/tags/v1.2.3",
	}
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
	manifestURL, _ := githubraw.BuildManifestURL(req.Repository, req.SHA)
	assetURL, _ := githubraw.BuildAssetURL(req.Repository, req.SHA, "dist/ExampleSans-Regular.otf")
	largeManifestBytes, largeAssetResults := buildLargePackageFixture(t, req.Repository, req.SHA)

	tests := []struct {
		name       string
		claims     protocol.OIDCClaims
		store      state.Store
		fetcher    fakeFetcher
		wantCode   string
		wantStatus int
	}{
		{
			name:   "valid",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				results: map[string]githubraw.Result{
					manifestURL: {Body: manifestBytes, Size: int64(len(manifestBytes))},
					assetURL:    {Body: []byte("asset-bytes"), Size: 11},
				},
				errors: map[string]error{},
			},
			wantStatus: 0,
		},
		{
			name:       "replay detected",
			claims:     claims,
			store:      seededReplayStore(t, "jwt-id-1"),
			fetcher:    fakeFetcher{},
			wantCode:   "AUTH_REPLAY_DETECTED",
			wantStatus: 401,
		},
		{
			name:       "ownership mismatch",
			claims:     claims,
			store:      seededOwnershipStore(t, "example/family", "different"),
			fetcher:    fakeFetcher{},
			wantCode:   "OWNERSHIP_MISMATCH",
			wantStatus: 403,
		},
		{
			name:   "manifest not found",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				errors: map[string]error{manifestURL: githubraw.ErrNotFound},
			},
			wantCode:   "UPSTREAM_NOT_FOUND",
			wantStatus: 404,
		},
		{
			name:   "manifest invalid json",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				results: map[string]githubraw.Result{manifestURL: {Body: []byte("{"), Size: 1}},
				errors:  map[string]error{},
			},
			wantCode:   "MANIFEST_INVALID_JSON",
			wantStatus: 422,
		},
		{
			name:   "tag version mismatch",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				results: map[string]githubraw.Result{
					manifestURL: {Body: []byte(`{"name":"Example Sans","author":"Example Studio","version":"1.2.4","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":400}]}`), Size: 158},
				},
				errors: map[string]error{},
			},
			wantCode:   "TAG_VERSION_MISMATCH",
			wantStatus: 422,
		},
		{
			name:   "asset too large",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				results: map[string]githubraw.Result{
					manifestURL: {Body: manifestBytes, Size: int64(len(manifestBytes))},
					assetURL:    {Body: []byte("small"), Size: AssetMaxBytes + 1},
				},
				errors: map[string]error{},
			},
			wantCode:   "ASSET_TOO_LARGE",
			wantStatus: 413,
		},
		{
			name:   "package too large",
			claims: claims,
			store:  state.NewMemoryStore(),
			fetcher: fakeFetcher{
				results: mergeResults(
					map[string]githubraw.Result{
						manifestURL: {Body: largeManifestBytes, Size: int64(len(largeManifestBytes))},
					},
					largeAssetResults,
				),
				errors: map[string]error{},
			},
			wantCode:   "PACKAGE_TOO_LARGE",
			wantStatus: 413,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ValidationProcessor{State: tc.store, Fetcher: tc.fetcher}
			result, errObj, status := p.Validate(context.Background(), req, tc.claims)
			if tc.wantCode == "" {
				if errObj != nil {
					t.Fatalf("unexpected error: %#v", errObj)
				}
				if result.VersionKey != "1.2.3" || len(result.Assets) != 1 {
					t.Fatalf("unexpected validation result: %+v", result)
				}
				return
			}
			if errObj == nil {
				t.Fatalf("expected error")
			}
			if errObj.Code != tc.wantCode || status != tc.wantStatus {
				t.Fatalf("got code=%s status=%d want code=%s status=%d", errObj.Code, status, tc.wantCode, tc.wantStatus)
			}
		})
	}
}

func TestValidationProcessorValidateInfrastructureAndMappingFailures(t *testing.T) {
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
	manifestURL, _ := githubraw.BuildManifestURL(req.Repository, req.SHA)
	assetURL, _ := githubraw.BuildAssetURL(req.Repository, req.SHA, "dist/ExampleSans-Regular.otf")
	validManifest := []byte(`{"name":"Example Sans","author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":400}]}`)

	tests := []struct {
		name       string
		processor  ValidationProcessor
		req        UpdateRequest
		wantCode   string
		wantStatus int
	}{
		{
			name:       "missing state store",
			processor:  ValidationProcessor{},
			req:        req,
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name:       "missing fetcher",
			processor:  ValidationProcessor{State: state.NewMemoryStore()},
			req:        req,
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name:       "state replay error",
			processor:  ValidationProcessor{State: failingStateStore{replayErr: errors.New("boom")}, Fetcher: fakeFetcher{}},
			req:        req,
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name:       "state ownership error",
			processor:  ValidationProcessor{State: failingStateStore{ownershipErr: errors.New("boom")}, Fetcher: fakeFetcher{}},
			req:        req,
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name:       "invalid repository",
			processor:  ValidationProcessor{State: state.NewMemoryStore(), Fetcher: fakeFetcher{}},
			req:        UpdateRequest{Repository: "example", SHA: req.SHA, Ref: req.Ref},
			wantCode:   "REQUEST_SCHEMA_INVALID",
			wantStatus: 400,
		},
		{
			name: "manifest too large",
			processor: ValidationProcessor{
				State:   state.NewMemoryStore(),
				Fetcher: fakeFetcher{errors: map[string]error{manifestURL: githubraw.ErrTooLarge}},
			},
			req:        req,
			wantCode:   "MANIFEST_TOO_LARGE",
			wantStatus: 413,
		},
		{
			name: "asset fetch generic failure",
			processor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{manifestURL: {Body: validManifest, Size: int64(len(validManifest))}},
					errors:  map[string]error{assetURL: errors.New("boom")},
				},
			},
			req:        req,
			wantCode:   "UPSTREAM_FETCH_FAILED",
			wantStatus: 502,
		},
		{
			name: "manifest schema invalid",
			processor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{
						manifestURL: {Body: []byte(`{"author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":400}]}`), Size: 150},
					},
				},
			},
			req:        req,
			wantCode:   "MANIFEST_SCHEMA_INVALID",
			wantStatus: 422,
		},
		{
			name: "asset path invalid",
			processor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{
						manifestURL: {Body: []byte(`{"name":"Example Sans","author":"Example Studio","version":"1.2.3","license":"OFL-1.1","files":[{"path":"../escape.otf","style":"normal","weight":400}]}`), Size: 150},
					},
				},
			},
			req:        req,
			wantCode:   "ASSET_PATH_INVALID",
			wantStatus: 422,
		},
		{
			name: "invalid tag version",
			processor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{manifestURL: {Body: validManifest, Size: int64(len(validManifest))}},
				},
			},
			req:        UpdateRequest{Repository: req.Repository, SHA: req.SHA, Ref: "refs/tags/not-a-version"},
			wantCode:   "TAG_VERSION_MISMATCH",
			wantStatus: 422,
		},
		{
			name: "invalid manifest version",
			processor: ValidationProcessor{
				State: state.NewMemoryStore(),
				Fetcher: fakeFetcher{
					results: map[string]githubraw.Result{
						manifestURL: {Body: []byte(`{"name":"Example Sans","author":"Example Studio","version":"not-a-version","license":"OFL-1.1","files":[{"path":"dist/ExampleSans-Regular.otf","style":"normal","weight":400}]}`), Size: 160},
					},
				},
			},
			req:        req,
			wantCode:   "VERSION_INVALID",
			wantStatus: 422,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, errObj, status := tc.processor.Validate(context.Background(), tc.req, claims)
			if errObj == nil {
				t.Fatalf("expected error")
			}
			if errObj.Code != tc.wantCode || status != tc.wantStatus {
				t.Fatalf("got code=%s status=%d want code=%s status=%d", errObj.Code, status, tc.wantCode, tc.wantStatus)
			}
		})
	}
}

func seededReplayStore(t *testing.T, jti string) state.Store {
	t.Helper()
	store := state.NewMemoryStore()
	if err := store.CheckAndReserveJTI(context.Background(), jti); err != nil {
		t.Fatalf("seed replay store: %v", err)
	}
	return store
}

func seededOwnershipStore(t *testing.T, packageID, repositoryID string) state.Store {
	t.Helper()
	store := state.NewMemoryStore()
	if err := store.CheckOrBindPackage(context.Background(), packageID, repositoryID); err != nil {
		t.Fatalf("seed ownership store: %v", err)
	}
	return store
}

func TestValidationProcessorProcess(t *testing.T) {
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
		SHA:             "0123456789abcdef0123456789abcdef01234567",
		Ref:             "refs/tags/v1.2.3",
		WorkflowRef:     "example/family/.github/workflows/fontpub.yml@refs/heads/main",
		WorkflowSHA:     "89abcdef0123456789abcdef0123456789abcdef",
		JTI:             "jwt-id-1",
		EventName:       "push",
	}
	manifestURL, _ := githubraw.BuildManifestURL(req.Repository, req.SHA)
	assetURL, _ := githubraw.BuildAssetURL(req.Repository, req.SHA, "dist/ExampleSans-Regular.otf")
	p := ValidationProcessor{
		State: state.NewMemoryStore(),
		Fetcher: fakeFetcher{
			results: map[string]githubraw.Result{
				manifestURL: {Body: manifestBytes, Size: int64(len(manifestBytes))},
				assetURL:    {Body: []byte("asset-bytes"), Size: 11},
			},
			errors: map[string]error{},
		},
	}
	status, body := p.Process(context.Background(), req, claims)
	if status != 501 {
		t.Fatalf("got status=%d want 501", status)
	}
	env, ok := body.(protocol.ErrorEnvelope)
	if !ok || env.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestMapFetchErrorFallback(t *testing.T) {
	errObj := mapFetchError(errors.New("boom"), true)
	if errObj.Code != "UPSTREAM_FETCH_FAILED" {
		t.Fatalf("unexpected code: %s", errObj.Code)
	}
}

func TestMapProtocolValidationErrorAndStatus(t *testing.T) {
	errObj := mapProtocolValidationError(errors.New("ASSET_FORMAT_NOT_ALLOWED: bad format"))
	if errObj.Code != "ASSET_FORMAT_NOT_ALLOWED" {
		t.Fatalf("unexpected code: %s", errObj.Code)
	}
	if got := mapValidationStatus(errors.New("LICENSE_NOT_ALLOWED: bad license")); got != 422 {
		t.Fatalf("unexpected validation status: %d", got)
	}
}

func buildLargePackageFixture(t *testing.T, repository, sha string) ([]byte, map[string]githubraw.Result) {
	t.Helper()
	manifest := protocol.Manifest{
		Name:    "Example Sans",
		Author:  "Example Studio",
		Version: "1.2.3",
		License: "OFL-1.1",
		Files:   make([]protocol.ManifestFile, 0, 41),
	}
	results := make(map[string]githubraw.Result, 41)
	for i := 0; i < 41; i++ {
		path := "dist/File" + string(rune('A'+i)) + ".otf"
		manifest.Files = append(manifest.Files, protocol.ManifestFile{Path: path, Style: "normal", Weight: 400})
		assetURL, err := githubraw.BuildAssetURL(repository, sha, path)
		if err != nil {
			t.Fatalf("BuildAssetURL(%s): %v", path, err)
		}
		results[assetURL] = githubraw.Result{Body: []byte("x"), Size: AssetMaxBytes}
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal large manifest: %v", err)
	}
	return body, results
}

func mergeResults(left, right map[string]githubraw.Result) map[string]githubraw.Result {
	out := map[string]githubraw.Result{}
	for key, value := range left {
		out[key] = value
	}
	for key, value := range right {
		out[key] = value
	}
	return out
}
