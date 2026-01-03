// Package index handles fetching and parsing the Fontpub global index.
package index

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// DefaultIndexURL is the default URL for the Fontpub index.
	DefaultIndexURL = "https://api.fontpub.org/index.json"
)

// Asset represents a font file asset in the index.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

// PackageInfo represents a package entry in the global index.
type PackageInfo struct {
	LatestVersion string           `json:"latest_version"`
	SHA256        string           `json:"sha256"`
	ManifestURL   string           `json:"manifest_url"`
	Assets        map[string]Asset `json:"assets"`
}

// Index represents the global Fontpub index.
type Index struct {
	Packages map[string]*PackageInfo `json:"packages"`
}

// Client is an HTTP client for fetching the index.
type Client struct {
	IndexURL   string
	HTTPClient *http.Client
}

// NewClient creates a new index client with default settings.
func NewClient() *Client {
	return &Client{
		IndexURL:   DefaultIndexURL,
		HTTPClient: http.DefaultClient,
	}
}

// NewClientWithURL creates a new index client with a custom URL.
func NewClientWithURL(url string) *Client {
	return &Client{
		IndexURL:   url,
		HTTPClient: http.DefaultClient,
	}
}

// Fetch downloads and parses the global index.
func (c *Client) Fetch() (*Index, error) {
	resp, err := c.HTTPClient.Get(c.IndexURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index fetch failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read index body: %w", err)
	}

	var index Index
	if err := json.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	if index.Packages == nil {
		index.Packages = make(map[string]*PackageInfo)
	}

	return &index, nil
}

// GetPackage returns information about a specific package.
func (idx *Index) GetPackage(name string) (*PackageInfo, error) {
	pkg := idx.Packages[name]
	if pkg == nil {
		return nil, fmt.Errorf("package not found: %s", name)
	}
	return pkg, nil
}

// ParseFromBytes parses an index from raw JSON bytes.
// Useful for testing or loading from cache.
func ParseFromBytes(data []byte) (*Index, error) {
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}
	if index.Packages == nil {
		index.Packages = make(map[string]*PackageInfo)
	}
	return &index, nil
}
