// Package index handles fetching and parsing the Fontpub index.
package index

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// DefaultBaseURL is the default base URL for the Fontpub API.
	DefaultBaseURL = "https://api.fontpub.org"
)

// PackageSummary represents a package entry in the root index.
// This contains only summary information for version checking.
type PackageSummary struct {
	LatestVersion string `json:"latest_version"`
	LastUpdated   string `json:"last_updated"`
}

// Index represents the root index (/v1/index.json).
type Index struct {
	Packages map[string]*PackageSummary `json:"packages"`
}

// Asset represents a font file asset in the package detail.
type Asset struct {
	Path   string `json:"path"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Style  string `json:"style,omitempty"`
	Weight int    `json:"weight,omitempty"`
	Format string `json:"format,omitempty"`
}

// PackageDetail represents the full package information (/v1/packages/{owner}/{repo}.json).
type PackageDetail struct {
	Name      string  `json:"name"`
	Version   string  `json:"version"`
	GitHubSHA string  `json:"github_sha"`
	Assets    []Asset `json:"assets"`
}

// Client is an HTTP client for fetching the index and package details.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new index client with default settings.
func NewClient() *Client {
	return &Client{
		BaseURL:    DefaultBaseURL,
		HTTPClient: http.DefaultClient,
	}
}

// NewClientWithBaseURL creates a new index client with a custom base URL.
func NewClientWithBaseURL(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
		HTTPClient: http.DefaultClient,
	}
}

// FetchIndex downloads and parses the root index.
func (c *Client) FetchIndex() (*Index, error) {
	url := c.BaseURL + "/v1/index.json"

	resp, err := c.HTTPClient.Get(url)
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
		index.Packages = make(map[string]*PackageSummary)
	}

	return &index, nil
}

// FetchPackageDetail downloads and parses the package detail.
// packageName should be in the format "owner/repo".
func (c *Client) FetchPackageDetail(packageName string) (*PackageDetail, error) {
	url := fmt.Sprintf("%s/v1/packages/%s.json", c.BaseURL, packageName)

	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package detail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("package not found: %s", packageName)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("package detail fetch failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read package detail body: %w", err)
	}

	var detail PackageDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse package detail: %w", err)
	}

	return &detail, nil
}

// GetPackage returns summary information about a specific package from the index.
func (idx *Index) GetPackage(name string) (*PackageSummary, error) {
	pkg := idx.Packages[name]
	if pkg == nil {
		return nil, fmt.Errorf("package not found: %s", name)
	}
	return pkg, nil
}

// ParseIndexFromBytes parses a root index from raw JSON bytes.
func ParseIndexFromBytes(data []byte) (*Index, error) {
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}
	if index.Packages == nil {
		index.Packages = make(map[string]*PackageSummary)
	}
	return &index, nil
}

// ParsePackageDetailFromBytes parses a package detail from raw JSON bytes.
func ParsePackageDetailFromBytes(data []byte) (*PackageDetail, error) {
	var detail PackageDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse package detail: %w", err)
	}
	return &detail, nil
}
