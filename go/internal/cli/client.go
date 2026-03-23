package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

const assetDownloadMaxBytes = 50 * 1024 * 1024

type MetadataClient struct {
	BaseURL      string
	UserAgent    string
	HTTPClient   *http.Client
	AssetFetcher githubraw.Fetcher
}

func NewMetadataClient(cfg Config) *MetadataClient {
	httpClient := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}
	var assetFetcher githubraw.Fetcher
	if len(cfg.LocalRepoMap) > 0 {
		assetFetcher = githubraw.RoutingFetcher{
			LocalRepos: cfg.LocalRepoMap,
			Remote:     githubraw.HTTPFetcher{Client: httpClient},
		}
	}
	return &MetadataClient{
		BaseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		UserAgent:    cfg.UserAgent,
		AssetFetcher: assetFetcher,
		HTTPClient:   httpClient,
	}
}

func (c *MetadataClient) GetRootIndex(ctx context.Context) (protocol.RootIndex, error) {
	var out protocol.RootIndex
	if err := c.getJSON(ctx, "/v1/index.json", &out); err != nil {
		return protocol.RootIndex{}, err
	}
	return out, nil
}

func (c *MetadataClient) FetchRootIndex(ctx context.Context, ifNoneMatch string) (protocol.RootIndex, []byte, string, bool, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint, err := url.Parse(c.BaseURL + "/v1/index.json")
	if err != nil {
		return protocol.RootIndex{}, nil, "", false, &CLIError{Code: "INTERNAL_ERROR", Message: "invalid base URL", Details: map[string]any{"path": "/v1/index.json"}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return protocol.RootIndex{}, nil, "", false, &CLIError{Code: "INTERNAL_ERROR", Message: "could not create request", Details: map[string]any{"path": "/v1/index.json"}}
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return protocol.RootIndex{}, nil, "", false, &CLIError{Code: "INTERNAL_ERROR", Message: "request failed", Details: map[string]any{"path": "/v1/index.json", "reason": err.Error()}}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return protocol.RootIndex{}, nil, "", false, &CLIError{Code: "INTERNAL_ERROR", Message: "could not read response", Details: map[string]any{"path": "/v1/index.json"}}
	}
	etag := resp.Header.Get("ETag")
	if resp.StatusCode == http.StatusNotModified {
		if etag == "" {
			etag = ifNoneMatch
		}
		return protocol.RootIndex{}, nil, etag, true, nil
	}
	if resp.StatusCode >= 400 {
		var env protocol.ErrorEnvelope
		if err := json.Unmarshal(body, &env); err == nil && env.Error.Code != "" {
			return protocol.RootIndex{}, nil, "", false, &CLIError{Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details}
		}
		return protocol.RootIndex{}, nil, "", false, &CLIError{
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("request failed with status %d", resp.StatusCode),
			Details: map[string]any{"path": "/v1/index.json", "status": resp.StatusCode},
		}
	}
	var out protocol.RootIndex
	if err := json.Unmarshal(body, &out); err != nil {
		return protocol.RootIndex{}, nil, "", false, &CLIError{Code: "INTERNAL_ERROR", Message: "response JSON was invalid", Details: map[string]any{"path": "/v1/index.json"}}
	}
	return out, body, etag, false, nil
}

func (c *MetadataClient) GetLatestPackageDetail(ctx context.Context, packageID string) (protocol.VersionedPackageDetail, error) {
	var out protocol.VersionedPackageDetail
	if err := c.getJSON(ctx, "/v1/packages/"+normalizePackageID(packageID)+".json", &out); err != nil {
		return protocol.VersionedPackageDetail{}, err
	}
	return out, nil
}

func (c *MetadataClient) GetPackageDetailVersion(ctx context.Context, packageID, version string) (protocol.VersionedPackageDetail, error) {
	versionKey, err := protocol.NormalizeVersionKey(version)
	if err != nil {
		return protocol.VersionedPackageDetail{}, &CLIError{
			Code:    "VERSION_INVALID",
			Message: "invalid version",
			Details: map[string]any{"version": version},
		}
	}
	var out protocol.VersionedPackageDetail
	path := "/v1/packages/" + normalizePackageID(packageID) + "/versions/" + versionKey + ".json"
	if err := c.getJSON(ctx, path, &out); err != nil {
		return protocol.VersionedPackageDetail{}, err
	}
	return out, nil
}

func (c *MetadataClient) getJSON(ctx context.Context, path string, dest any) error {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "invalid base URL", Details: map[string]any{"path": path}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "could not create request", Details: map[string]any{"path": path}}
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "request failed", Details: map[string]any{"path": path, "reason": err.Error()}}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "could not read response", Details: map[string]any{"path": path}}
	}
	if resp.StatusCode >= 400 {
		var env protocol.ErrorEnvelope
		if err := json.Unmarshal(body, &env); err == nil && env.Error.Code != "" {
			return &CLIError{Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details}
		}
		return &CLIError{
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("request failed with status %d", resp.StatusCode),
			Details: map[string]any{"path": path, "status": resp.StatusCode},
		}
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return &CLIError{Code: "INTERNAL_ERROR", Message: "response JSON was invalid", Details: map[string]any{"path": path}}
	}
	return nil
}

func (c *MetadataClient) Download(ctx context.Context, rawURL string) ([]byte, error) {
	if c.AssetFetcher != nil {
		result, err := c.AssetFetcher.Fetch(ctx, rawURL, assetDownloadMaxBytes)
		if err == nil {
			return result.Body, nil
		}
		switch err {
		case githubraw.ErrNotFound:
			return nil, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "download source was not found", Details: map[string]any{"url": rawURL}}
		case githubraw.ErrTooLarge:
			return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "download exceeds maximum asset size", Details: map[string]any{"url": rawURL}}
		}
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not create request", Details: map[string]any{"url": rawURL}}
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "download failed", Details: map[string]any{"url": rawURL, "reason": err.Error()}}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not read download", Details: map[string]any{"url": rawURL}}
	}
	if resp.StatusCode >= 400 {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: fmt.Sprintf("download failed with status %d", resp.StatusCode), Details: map[string]any{"url": rawURL, "status": resp.StatusCode}}
	}
	return body, nil
}
