package updateapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ma/fontpub/go/internal/indexer/artifacts"
	"github.com/ma/fontpub/go/internal/protocol"
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}

type PublishingProcessor struct {
	ValidationProcessor
	ArtifactStore artifacts.Store
	Clock         Clock
}

func (p PublishingProcessor) Process(ctx context.Context, req UpdateRequest, claims protocol.OIDCClaims) (int, any) {
	result, errObj, status := p.Validate(ctx, req, claims)
	if errObj != nil {
		return status, protocol.ErrorEnvelope{Error: *errObj}
	}
	response, errObj, status := p.Publish(ctx, req, result)
	if errObj != nil {
		return status, protocol.ErrorEnvelope{Error: *errObj}
	}
	return http.StatusOK, response
}

func (p PublishingProcessor) Publish(ctx context.Context, req UpdateRequest, validated ValidationResult) (map[string]any, *protocol.ErrorObject, int) {
	if p.ArtifactStore == nil {
		return nil, internalError("artifact store is not configured"), http.StatusInternalServerError
	}
	clock := p.Clock
	if clock == nil {
		clock = RealClock{}
	}

	existing, found, err := p.ArtifactStore.GetVersionedPackageDetail(ctx, validated.PackageID, validated.VersionKey)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}

	var detail protocol.VersionedPackageDetail
	var detailBytes []byte
	var detailETag string
	now := clock.Now().UTC().Format(time.RFC3339)

	if found {
		candidate := buildVersionedPackageDetail(validated, req, existing.PublishedAt)
		if !protocol.ImmutableEqual(existing, candidate) {
			return nil, errorObject("IMMUTABLE_VERSION", "published version differs in immutable fields", nil), http.StatusConflict
		}
		detail = existing
		if detailBytes, err = protocol.MarshalCanonical(detail); err != nil {
			return nil, internalError(err.Error()), http.StatusInternalServerError
		}
		detailETag = computeETag(detailBytes)
	} else {
		detail = buildVersionedPackageDetail(validated, req, now)
		if detailBytes, err = protocol.MarshalCanonical(detail); err != nil {
			return nil, internalError(err.Error()), http.StatusInternalServerError
		}
		detailETag = computeETag(detailBytes)
		if err := p.ArtifactStore.PutVersionedPackageDetail(ctx, detail, detailBytes, detailETag); err != nil {
			return nil, indexConflict(err), http.StatusServiceUnavailable
		}
	}

	packageDetails, err := p.ArtifactStore.ListPackageVersionedPackageDetails(ctx, validated.PackageID)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndex, latestDetail, err := buildPackageVersionsIndex(validated.PackageID, packageDetails)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndexBytes, err := protocol.MarshalCanonical(packageIndex)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndexETag := computeETag(packageIndexBytes)
	if err := p.ArtifactStore.PutPackageVersionsIndex(ctx, validated.PackageID, packageIndex, packageIndexBytes, packageIndexETag); err != nil {
		return nil, indexConflict(err), http.StatusServiceUnavailable
	}

	latestAliasUpdated := latestDetail.VersionKey == detail.VersionKey
	latestAliasETag := ""
	if latestAliasUpdated {
		if err := p.ArtifactStore.PutLatestAlias(ctx, validated.PackageID, detailBytes, detailETag); err != nil {
			return nil, indexConflict(err), http.StatusServiceUnavailable
		}
		latestAliasETag = detailETag
	}

	allDetails, err := p.ArtifactStore.ListAllVersionedPackageDetails(ctx)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndex, err := buildRootIndex(allDetails)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndexBytes, err := protocol.MarshalCanonical(rootIndex)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndexETag := computeETag(rootIndexBytes)
	if err := p.ArtifactStore.PutRootIndex(ctx, rootIndex, rootIndexBytes, rootIndexETag); err != nil {
		return nil, indexConflict(err), http.StatusServiceUnavailable
	}

	response := map[string]any{
		"status":      "ok",
		"package_id":  detail.PackageID,
		"version":     detail.Version,
		"version_key": detail.VersionKey,
		"github_sha":  detail.GitHub.SHA,
		"artifacts": map[string]any{
			"versioned_package_detail": map[string]any{
				"path": artifacts.VersionedPackageDetailPath(detail.PackageID, detail.VersionKey),
				"etag": detailETag,
			},
			"package_versions_index": map[string]any{
				"path": artifacts.PackageVersionsIndexPath(detail.PackageID),
				"etag": packageIndexETag,
			},
			"latest_package_alias": map[string]any{
				"path":    artifacts.LatestAliasPath(detail.PackageID),
				"updated": latestAliasUpdated,
			},
			"root_index": map[string]any{
				"path": artifacts.RootIndexPath(),
				"etag": rootIndexETag,
			},
		},
	}
	if latestAliasUpdated {
		response["artifacts"].(map[string]any)["latest_package_alias"].(map[string]any)["etag"] = latestAliasETag
	}
	return response, nil, http.StatusOK
}

func buildVersionedPackageDetail(validated ValidationResult, req UpdateRequest, publishedAt string) protocol.VersionedPackageDetail {
	assets := make([]protocol.VersionedAsset, 0, len(validated.Assets))
	for _, asset := range validated.Assets {
		assets = append(assets, protocol.VersionedAsset{
			Path:      asset.Path,
			URL:       asset.URL,
			SHA256:    asset.SHA256,
			Format:    asset.Format,
			Style:     asset.Style,
			Weight:    asset.Weight,
			SizeBytes: asset.SizeBytes,
		})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	return protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     validated.PackageID,
		DisplayName:   validated.Manifest.Name,
		Author:        validated.Manifest.Author,
		License:       validated.Manifest.License,
		Version:       validated.Version,
		VersionKey:    validated.VersionKey,
		PublishedAt:   publishedAt,
		GitHub: protocol.GitHubRef{
			Owner: splitOwner(validated.PackageID),
			Repo:  splitRepo(validated.PackageID),
			SHA:   req.SHA,
		},
		ManifestURL: validated.ManifestURL,
		Assets:      assets,
	}
}

func buildPackageVersionsIndex(packageID string, details []protocol.VersionedPackageDetail) (protocol.PackageVersionsIndex, protocol.VersionedPackageDetail, error) {
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
			URL:         artifacts.VersionedPackageDetailPath(packageID, detail.VersionKey),
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

func buildRootIndex(details []protocol.VersionedPackageDetail) (protocol.RootIndex, error) {
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

func computeETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func indexConflict(err error) *protocol.ErrorObject {
	return errorObject("INDEX_CONFLICT", "could not preserve derived document consistency", map[string]any{
		"reason": err.Error(),
	})
}

func splitOwner(packageID string) string {
	parts := strings.SplitN(packageID, "/", 2)
	return parts[0]
}

func splitRepo(packageID string) string {
	parts := strings.SplitN(packageID, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}
