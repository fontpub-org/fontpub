package deriveddocs

import (
	"context"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type PackageWriteResult struct {
	PackageIndexETag string
	LatestAliasETag  string
	LatestDetail     protocol.VersionedPackageDetail
}

func WritePackage(ctx context.Context, store artifacts.Store, packageID string, details []protocol.VersionedPackageDetail) (PackageWriteResult, error) {
	packageIndex, latestDetail, err := derive.BuildPackageVersionsIndex(packageID, append([]protocol.VersionedPackageDetail(nil), details...))
	if err != nil {
		return PackageWriteResult{}, err
	}
	packageIndexBytes, packageIndexETag, err := marshalCanonical(packageIndex)
	if err != nil {
		return PackageWriteResult{}, err
	}
	if err := store.PutPackageVersionsIndex(ctx, packageID, packageIndex, packageIndexBytes, packageIndexETag); err != nil {
		return PackageWriteResult{}, err
	}

	latestBytes, latestETag, err := marshalCanonical(latestDetail)
	if err != nil {
		return PackageWriteResult{}, err
	}
	if err := store.PutLatestAlias(ctx, packageID, latestBytes, latestETag); err != nil {
		return PackageWriteResult{}, err
	}

	return PackageWriteResult{
		PackageIndexETag: packageIndexETag,
		LatestAliasETag:  latestETag,
		LatestDetail:     latestDetail,
	}, nil
}

func WriteRoot(ctx context.Context, store artifacts.Store, details []protocol.VersionedPackageDetail) (string, error) {
	rootIndex, err := derive.BuildRootIndex(details)
	if err != nil {
		return "", err
	}
	rootBytes, rootETag, err := marshalCanonical(rootIndex)
	if err != nil {
		return "", err
	}
	if err := store.PutRootIndex(ctx, rootIndex, rootBytes, rootETag); err != nil {
		return "", err
	}
	return rootETag, nil
}

func marshalCanonical(value any) ([]byte, string, error) {
	body, err := protocol.MarshalCanonical(value)
	if err != nil {
		return nil, "", err
	}
	return body, derive.ComputeETag(body), nil
}
