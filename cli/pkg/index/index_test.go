package index

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFromBytes(t *testing.T) {
	data := []byte(`{
		"packages": {
			"alice/myfont": {
				"latest_version": "1.0.0",
				"sha256": "abc123",
				"manifest_url": "https://example.com/manifest.json",
				"assets": {
					"Regular.otf": {
						"url": "https://example.com/Regular.otf",
						"sha256": "def456"
					}
				}
			}
		}
	}`)

	idx, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("ParseFromBytes() error = %v", err)
	}

	pkg, err := idx.GetPackage("alice/myfont")
	if err != nil {
		t.Fatalf("GetPackage() error = %v", err)
	}

	if pkg.LatestVersion != "1.0.0" {
		t.Errorf("LatestVersion = %q, want %q", pkg.LatestVersion, "1.0.0")
	}
	if pkg.SHA256 != "abc123" {
		t.Errorf("SHA256 = %q, want %q", pkg.SHA256, "abc123")
	}
	if pkg.ManifestURL != "https://example.com/manifest.json" {
		t.Errorf("ManifestURL = %q, want %q", pkg.ManifestURL, "https://example.com/manifest.json")
	}

	asset, ok := pkg.Assets["Regular.otf"]
	if !ok {
		t.Fatal("Asset Regular.otf not found")
	}
	if asset.URL != "https://example.com/Regular.otf" {
		t.Errorf("Asset URL = %q, want %q", asset.URL, "https://example.com/Regular.otf")
	}
	if asset.SHA256 != "def456" {
		t.Errorf("Asset SHA256 = %q, want %q", asset.SHA256, "def456")
	}
}

func TestGetPackageNotFound(t *testing.T) {
	idx, _ := ParseFromBytes([]byte(`{"packages": {}}`))

	_, err := idx.GetPackage("nonexistent")
	if err == nil {
		t.Error("GetPackage() should return error for nonexistent package")
	}
}

func TestFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"packages": {
				"test/font": {
					"latest_version": "2.0.0",
					"sha256": "xyz789",
					"manifest_url": "https://test.com/manifest.json",
					"assets": {}
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewClientWithURL(server.URL)
	idx, err := client.Fetch()
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	pkg, err := idx.GetPackage("test/font")
	if err != nil {
		t.Fatalf("GetPackage() error = %v", err)
	}

	if pkg.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", pkg.LatestVersion, "2.0.0")
	}
}

func TestFetchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithURL(server.URL)
	_, err := client.Fetch()
	if err == nil {
		t.Error("Fetch() should return error for HTTP 500")
	}
}

func TestFetchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	client := NewClientWithURL(server.URL)
	_, err := client.Fetch()
	if err == nil {
		t.Error("Fetch() should return error for invalid JSON")
	}
}
