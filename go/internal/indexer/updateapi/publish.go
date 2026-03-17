package updateapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
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
		detailETag = derive.ComputeETag(detailBytes)
	} else {
		detail = buildVersionedPackageDetail(validated, req, now)
		if detailBytes, err = protocol.MarshalCanonical(detail); err != nil {
			return nil, internalError(err.Error()), http.StatusInternalServerError
		}
		detailETag = derive.ComputeETag(detailBytes)
		if err := p.ArtifactStore.PutVersionedPackageDetail(ctx, detail, detailBytes, detailETag); err != nil {
			return nil, indexConflict(err), http.StatusServiceUnavailable
		}
	}

	packageDetails, err := p.ArtifactStore.ListPackageVersionedPackageDetails(ctx, validated.PackageID)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndex, latestDetail, err := derive.BuildPackageVersionsIndex(validated.PackageID, packageDetails)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndexBytes, err := protocol.MarshalCanonical(packageIndex)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	packageIndexETag := derive.ComputeETag(packageIndexBytes)
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
	rootIndex, err := derive.BuildRootIndex(allDetails)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndexBytes, err := protocol.MarshalCanonical(rootIndex)
	if err != nil {
		return nil, internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndexETag := derive.ComputeETag(rootIndexBytes)
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
