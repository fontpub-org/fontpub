package githubraw

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestBuildURLs(t *testing.T) {
	manifestURL, err := BuildManifestURL("Example/Family", "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatalf("BuildManifestURL: %v", err)
	}
	if manifestURL != "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json" {
		t.Fatalf("unexpected manifest URL: %s", manifestURL)
	}
	assetURL, err := BuildAssetURL("Example/Family", "0123456789abcdef0123456789abcdef01234567", "dist/Example Sans.otf")
	if err != nil {
		t.Fatalf("BuildAssetURL: %v", err)
	}
	if !strings.Contains(assetURL, "Example%20Sans.otf") {
		t.Fatalf("expected encoded asset URL, got %s", assetURL)
	}
}

func TestHTTPFetcher(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/ok":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("abc")),
				}, nil
			case "/large":
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader("abcdef")),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("missing")),
				}, nil
			}
		}),
	}

	fetcher := HTTPFetcher{Client: client}
	if _, err := fetcher.Fetch(context.Background(), "https://example.test/missing", 10); err != ErrNotFound {
		t.Fatalf("got %v want ErrNotFound", err)
	}
	if _, err := fetcher.Fetch(context.Background(), "https://example.test/large", 5); err != ErrTooLarge {
		t.Fatalf("got %v want ErrTooLarge", err)
	}
	result, err := fetcher.Fetch(context.Background(), "https://example.test/ok", 5)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(result.Body) != "abc" || result.Size != 3 {
		t.Fatalf("unexpected fetch result: %+v", result)
	}
}
