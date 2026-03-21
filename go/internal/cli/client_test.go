package cli

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
)

type fakeAssetFetcher struct {
	result githubraw.Result
	err    error
}

func (f fakeAssetFetcher) Fetch(context.Context, string, int64) (githubraw.Result, error) {
	return f.result, f.err
}

func TestMetadataClientDownloadUsesAssetFetcher(t *testing.T) {
	client := &MetadataClient{
		AssetFetcher: fakeAssetFetcher{
			result: githubraw.Result{Body: []byte("font-bytes"), Size: 10},
		},
	}
	body, err := client.Download(context.Background(), "https://raw.githubusercontent.com/example/family/sha/dist/ExampleSans.otf")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(body) != "font-bytes" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestMetadataClientDownloadMapsLocalNotFound(t *testing.T) {
	client := &MetadataClient{
		AssetFetcher: fakeAssetFetcher{err: githubraw.ErrNotFound},
	}
	_, err := client.Download(context.Background(), "https://raw.githubusercontent.com/example/family/sha/dist/ExampleSans.otf")
	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != "LOCAL_FILE_MISSING" {
		t.Fatalf("unexpected code: %s", cliErr.Code)
	}
}

func TestNewMetadataClientInstallsLocalRepoFetcher(t *testing.T) {
	client := NewMetadataClient(Config{
		BaseURL:      "http://127.0.0.1:18081",
		HTTPTimeout:  3,
		LocalRepoMap: map[string]string{"0xtype/gamut": "/Users/ma/0xType/Gamut"},
	})
	if _, ok := client.AssetFetcher.(githubraw.RoutingFetcher); !ok {
		t.Fatalf("expected RoutingFetcher, got %T", client.AssetFetcher)
	}
}

func TestMetadataClientGetRootIndexMapsHTTPStatusFallback(t *testing.T) {
	client := &MetadataClient{
		BaseURL: "https://fontpub.org",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
				}, nil
			}),
		},
	}
	_, err := client.GetRootIndex(context.Background())
	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != "INTERNAL_ERROR" || cliErr.Details["status"] != http.StatusBadGateway {
		t.Fatalf("unexpected error: %#v", cliErr)
	}
}

func TestMetadataClientGetRootIndexMapsInvalidJSON(t *testing.T) {
	client := &MetadataClient{
		BaseURL: "https://fontpub.org",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{not-json")),
				}, nil
			}),
		},
	}
	_, err := client.GetRootIndex(context.Background())
	cliErr, ok := err.(*CLIError)
	if !ok {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != "INTERNAL_ERROR" || cliErr.Message != "response JSON was invalid" {
		t.Fatalf("unexpected error: %#v", cliErr)
	}
}
