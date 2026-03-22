package updateapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/indexer/deriveddocs"
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
	clock := p.publishClock()

	detail, detailBytes, detailETag, errObj, status := p.loadOrCreateVersionedDetail(ctx, req, validated, clock.Now().UTC().Format(time.RFC3339))
	if errObj != nil {
		return nil, errObj, status
	}

	packageWrite, errObj, status := p.writePackageDerivedDocs(ctx, validated.PackageID)
	if errObj != nil {
		return nil, errObj, status
	}
	rootIndexETag, errObj, status := p.writeRootDerivedDocs(ctx)
	if errObj != nil {
		return nil, errObj, status
	}

	return buildPublishResponse(detail, detailBytes, detailETag, packageWrite, rootIndexETag), nil, http.StatusOK
}

func (p PublishingProcessor) publishClock() Clock {
	if p.Clock != nil {
		return p.Clock
	}
	return RealClock{}
}

func (p PublishingProcessor) loadOrCreateVersionedDetail(ctx context.Context, req UpdateRequest, validated ValidationResult, now string) (protocol.VersionedPackageDetail, []byte, string, *protocol.ErrorObject, int) {
	existing, found, err := p.ArtifactStore.GetVersionedPackageDetail(ctx, validated.PackageID, validated.VersionKey)
	if err != nil {
		return protocol.VersionedPackageDetail{}, nil, "", internalError(err.Error()), http.StatusInternalServerError
	}

	if found {
		candidate := buildVersionedPackageDetail(validated, req, existing.PublishedAt)
		if !protocol.ImmutableEqual(existing, candidate) {
			return protocol.VersionedPackageDetail{}, nil, "", errorObject("IMMUTABLE_VERSION", "published version differs in immutable fields", nil), http.StatusConflict
		}
		detailBytes, err := protocol.MarshalCanonical(existing)
		if err != nil {
			return protocol.VersionedPackageDetail{}, nil, "", internalError(err.Error()), http.StatusInternalServerError
		}
		return existing, detailBytes, derive.ComputeETag(detailBytes), nil, http.StatusOK
	}

	detail := buildVersionedPackageDetail(validated, req, now)
	detailBytes, err := protocol.MarshalCanonical(detail)
	if err != nil {
		return protocol.VersionedPackageDetail{}, nil, "", internalError(err.Error()), http.StatusInternalServerError
	}
	detailETag := derive.ComputeETag(detailBytes)
	if err := p.ArtifactStore.PutVersionedPackageDetail(ctx, detail, detailBytes, detailETag); err != nil {
		return protocol.VersionedPackageDetail{}, nil, "", indexConflict(err), http.StatusServiceUnavailable
	}
	return detail, detailBytes, detailETag, nil, http.StatusOK
}

func (p PublishingProcessor) writePackageDerivedDocs(ctx context.Context, packageID string) (deriveddocs.PackageWriteResult, *protocol.ErrorObject, int) {
	packageDetails, err := p.ArtifactStore.ListPackageVersionedPackageDetails(ctx, packageID)
	if err != nil {
		return deriveddocs.PackageWriteResult{}, internalError(err.Error()), http.StatusInternalServerError
	}
	packageWrite, err := deriveddocs.WritePackage(ctx, p.ArtifactStore, packageID, packageDetails)
	if err != nil {
		return deriveddocs.PackageWriteResult{}, indexConflict(err), http.StatusServiceUnavailable
	}
	return packageWrite, nil, http.StatusOK
}

func (p PublishingProcessor) writeRootDerivedDocs(ctx context.Context) (string, *protocol.ErrorObject, int) {
	allDetails, err := p.ArtifactStore.ListAllVersionedPackageDetails(ctx)
	if err != nil {
		return "", internalError(err.Error()), http.StatusInternalServerError
	}
	rootIndexETag, err := deriveddocs.WriteRoot(ctx, p.ArtifactStore, allDetails)
	if err != nil {
		return "", indexConflict(err), http.StatusServiceUnavailable
	}
	return rootIndexETag, nil, http.StatusOK
}

func buildPublishResponse(detail protocol.VersionedPackageDetail, detailBytes []byte, detailETag string, packageWrite deriveddocs.PackageWriteResult, rootIndexETag string) map[string]any {
	_ = detailBytes
	latestAliasUpdated := packageWrite.LatestDetail.VersionKey == detail.VersionKey
	latestAlias := map[string]any{
		"path":    artifacts.LatestAliasPath(detail.PackageID),
		"updated": latestAliasUpdated,
	}
	if latestAliasUpdated {
		latestAlias["etag"] = packageWrite.LatestAliasETag
	}

	return map[string]any{
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
				"etag": packageWrite.PackageIndexETag,
			},
			"latest_package_alias": latestAlias,
			"root_index": map[string]any{
				"path": artifacts.RootIndexPath(),
				"etag": rootIndexETag,
			},
		},
	}
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
