package derive

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/ma/fontpub/go/internal/protocol"
)

func ComputeETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func BuildPackageVersionsIndex(packageID string, details []protocol.VersionedPackageDetail) (protocol.PackageVersionsIndex, protocol.VersionedPackageDetail, error) {
	sort.Slice(details, func(i, j int) bool {
		cmp, _ := protocol.CompareVersions(details[i].Version, details[j].Version)
		if cmp == 0 {
			return details[i].PublishedAt > details[j].PublishedAt
		}
		return cmp > 0
	})
	versions := make([]protocol.PackageVersionRecord, 0, len(details))
	for _, detail := range details {
		versions = append(versions, protocol.PackageVersionRecord{
			Version:     detail.Version,
			VersionKey:  detail.VersionKey,
			PublishedAt: detail.PublishedAt,
			URL:         versionedPackageDetailPath(packageID, detail.VersionKey),
		})
	}
	latest := details[0]
	return protocol.PackageVersionsIndex{
		SchemaVersion:    "1",
		PackageID:        packageID,
		LatestVersion:    latest.Version,
		LatestVersionKey: latest.VersionKey,
		Versions:         versions,
	}, latest, nil
}

func BuildRootIndex(details []protocol.VersionedPackageDetail) (protocol.RootIndex, error) {
	latestByPackage := map[string]protocol.VersionedPackageDetail{}
	for _, detail := range details {
		current, exists := latestByPackage[detail.PackageID]
		if !exists {
			latestByPackage[detail.PackageID] = detail
			continue
		}
		cmp, err := protocol.CompareVersions(detail.Version, current.Version)
		if err != nil {
			return protocol.RootIndex{}, err
		}
		if cmp > 0 || (cmp == 0 && detail.PublishedAt > current.PublishedAt) {
			latestByPackage[detail.PackageID] = detail
		}
	}

	packages := make(map[string]protocol.RootIndexPackage, len(latestByPackage))
	generatedAt := "1970-01-01T00:00:00Z"
	for packageID, detail := range latestByPackage {
		packages[packageID] = protocol.RootIndexPackage{
			LatestVersion:     detail.Version,
			LatestVersionKey:  detail.VersionKey,
			LatestPublishedAt: detail.PublishedAt,
		}
		if detail.PublishedAt > generatedAt {
			generatedAt = detail.PublishedAt
		}
	}
	return protocol.RootIndex{
		SchemaVersion: "1",
		GeneratedAt:   generatedAt,
		Packages:      packages,
	}, nil
}

func versionedPackageDetailPath(packageID, versionKey string) string {
	return strings.Join([]string{"", "v1", "packages", packageID, "versions", versionKey + ".json"}, "/")
}
