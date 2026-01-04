package index

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseIndexFromBytes(t *testing.T) {
	data := []byte(`{
		"packages": {
			"alice/myfont": {
				"latest_version": "1.0.0",
				"last_updated": "2026-01-03T22:25:00Z"
			}
		}
	}`)

	idx, err := ParseIndexFromBytes(data)
	if err != nil {
		t.Fatalf("ParseIndexFromBytes() error = %v", err)
	}

	pkg, err := idx.GetPackage("alice/myfont")
	if err != nil {
		t.Fatalf("GetPackage() error = %v", err)
	}

	if pkg.LatestVersion != "1.0.0" {
		t.Errorf("LatestVersion = %q, want %q", pkg.LatestVersion, "1.0.0")
	}
	if pkg.LastUpdated != "2026-01-03T22:25:00Z" {
		t.Errorf("LastUpdated = %q, want %q", pkg.LastUpdated, "2026-01-03T22:25:00Z")
	}
}

func TestParsePackageDetailFromBytes(t *testing.T) {
	data := []byte(`{
		"name": "0xProto",
		"version": "2.500",
		"github_sha": "a1b2c3d4e5f6",
		"assets": [
			{
				"path": "fonts/0xProto-Regular.otf",
				"url": "https://raw.githubusercontent.com/0xType/0xProto/a1b2c3d4e5f6/fonts/0xProto-Regular.otf",
				"sha256": "fc9e27c9fe581378c2acccc2fb9e9500c00044cecb5de8e002648e2509b9dbcc",
				"style": "regular",
				"weight": 400,
				"format": "otf"
			}
		]
	}`)

	detail, err := ParsePackageDetailFromBytes(data)
	if err != nil {
		t.Fatalf("ParsePackageDetailFromBytes() error = %v", err)
	}

	if detail.Name != "0xProto" {
		t.Errorf("Name = %q, want %q", detail.Name, "0xProto")
	}
	if detail.Version != "2.500" {
		t.Errorf("Version = %q, want %q", detail.Version, "2.500")
	}
	if detail.GitHubSHA != "a1b2c3d4e5f6" {
		t.Errorf("GitHubSHA = %q, want %q", detail.GitHubSHA, "a1b2c3d4e5f6")
	}
	if len(detail.Assets) != 1 {
		t.Fatalf("Assets count = %d, want 1", len(detail.Assets))
	}

	asset := detail.Assets[0]
	if asset.Path != "fonts/0xProto-Regular.otf" {
		t.Errorf("Asset Path = %q, want %q", asset.Path, "fonts/0xProto-Regular.otf")
	}
	if asset.SHA256 != "fc9e27c9fe581378c2acccc2fb9e9500c00044cecb5de8e002648e2509b9dbcc" {
		t.Errorf("Asset SHA256 mismatch")
	}
	if asset.Style != "regular" {
		t.Errorf("Asset Style = %q, want %q", asset.Style, "regular")
	}
	if asset.Weight != 400 {
		t.Errorf("Asset Weight = %d, want 400", asset.Weight)
	}
}

func TestGetPackageNotFound(t *testing.T) {
	idx, _ := ParseIndexFromBytes([]byte(`{"packages": {}}`))

	_, err := idx.GetPackage("nonexistent")
	if err == nil {
		t.Error("GetPackage() should return error for nonexistent package")
	}
}

func TestFetchIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/index.json" {
			t.Errorf("Expected path /v1/index.json, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"packages": {
				"test/font": {
					"latest_version": "2.0.0",
					"last_updated": "2026-01-01T00:00:00Z"
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	idx, err := client.FetchIndex()
	if err != nil {
		t.Fatalf("FetchIndex() error = %v", err)
	}

	pkg, err := idx.GetPackage("test/font")
	if err != nil {
		t.Fatalf("GetPackage() error = %v", err)
	}

	if pkg.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", pkg.LatestVersion, "2.0.0")
	}
}

func TestFetchPackageDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/packages/alice/myfont.json" {
			t.Errorf("Expected path /v1/packages/alice/myfont.json, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name": "myfont",
			"version": "1.0.0",
			"github_sha": "abc123",
			"assets": [
				{
					"path": "Font-Regular.otf",
					"url": "https://example.com/Font-Regular.otf",
					"sha256": "def456"
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	detail, err := client.FetchPackageDetail("alice/myfont")
	if err != nil {
		t.Fatalf("FetchPackageDetail() error = %v", err)
	}

	if detail.Name != "myfont" {
		t.Errorf("Name = %q, want %q", detail.Name, "myfont")
	}
	if detail.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", detail.Version, "1.0.0")
	}
	if len(detail.Assets) != 1 {
		t.Fatalf("Assets count = %d, want 1", len(detail.Assets))
	}
}

func TestFetchPackageDetailNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchPackageDetail("nonexistent/package")
	if err == nil {
		t.Error("FetchPackageDetail() should return error for 404")
	}
}

func TestFetchIndexHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchIndex()
	if err == nil {
		t.Error("FetchIndex() should return error for HTTP 500")
	}
}

func TestFetchIndexInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL(server.URL)
	_, err := client.FetchIndex()
	if err == nil {
		t.Error("FetchIndex() should return error for invalid JSON")
	}
}
