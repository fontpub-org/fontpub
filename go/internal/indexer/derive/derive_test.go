package derive

import (
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func TestComputeETag(t *testing.T) {
	if got := ComputeETag([]byte("hello")); got != `"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"` {
		t.Fatalf("unexpected etag: %s", got)
	}
}

func TestBuildPackageVersionsIndexSortsByVersionAndPublishedAt(t *testing.T) {
	index, latest, err := BuildPackageVersionsIndex("example/family", []protocol.VersionedPackageDetail{
		testVersionedDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
		testVersionedDetail("example/family", "1.10.0", "2026-01-01T00:00:00Z"),
		testVersionedDetail("example/family", "1.10.0", "2026-01-03T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("BuildPackageVersionsIndex: %v", err)
	}
	if latest.VersionKey != "1.10.0" || latest.PublishedAt != "2026-01-03T00:00:00Z" {
		t.Fatalf("unexpected latest detail: %+v", latest)
	}
	if len(index.Versions) != 3 {
		t.Fatalf("unexpected versions: %+v", index.Versions)
	}
	if index.Versions[0].VersionKey != "1.10.0" || index.Versions[0].PublishedAt != "2026-01-03T00:00:00Z" {
		t.Fatalf("unexpected first version record: %+v", index.Versions[0])
	}
	if index.Versions[2].VersionKey != "1.2.3" {
		t.Fatalf("unexpected last version record: %+v", index.Versions[2])
	}
}

func TestBuildRootIndexChoosesLatestPerPackage(t *testing.T) {
	root, err := BuildRootIndex([]protocol.VersionedPackageDetail{
		testVersionedDetail("example/family", "1.2.3", "2026-01-02T00:00:00Z"),
		testVersionedDetail("example/family", "1.10.0", "2026-01-01T00:00:00Z"),
		testVersionedDetail("example/serif", "2.0.0", "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("BuildRootIndex: %v", err)
	}
	if root.GeneratedAt != "2026-01-04T00:00:00Z" {
		t.Fatalf("unexpected generated_at: %s", root.GeneratedAt)
	}
	family := root.Packages["example/family"]
	if family.LatestVersion != "1.10.0" || family.LatestVersionKey != "1.10.0" {
		t.Fatalf("unexpected family package: %+v", family)
	}
}

func TestVersionedPackageDetailPath(t *testing.T) {
	if got := versionedPackageDetailPath("example/family", "1.2.3"); got != "/v1/packages/example/family/versions/1.2.3.json" {
		t.Fatalf("unexpected path: %s", got)
	}
}

func testVersionedDetail(packageID, version, publishedAt string) protocol.VersionedPackageDetail {
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
