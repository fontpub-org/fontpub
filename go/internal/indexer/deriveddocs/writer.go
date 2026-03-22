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
	packageIndexETag, err := writeCanonicalDocument(ctx, func(body []byte, etag string) error {
		return store.PutPackageVersionsIndex(ctx, packageID, packageIndex, body, etag)
	}, packageIndex)
	if err != nil {
		return PackageWriteResult{}, err
	}
	latestETag, err := writeCanonicalDocument(ctx, func(body []byte, etag string) error {
		return store.PutLatestAlias(ctx, packageID, body, etag)
	}, latestDetail)
	if err != nil {
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
	return writeCanonicalDocument(ctx, func(body []byte, etag string) error {
		return store.PutRootIndex(ctx, rootIndex, body, etag)
	}, rootIndex)
}

func marshalCanonical(value any) ([]byte, string, error) {
	body, err := protocol.MarshalCanonical(value)
	if err != nil {
		return nil, "", err
	}
	return body, derive.ComputeETag(body), nil
}

func writeCanonicalDocument(ctx context.Context, write func(body []byte, etag string) error, value any) (string, error) {
	body, etag, err := marshalCanonical(value)
	if err != nil {
		return "", err
	}
	if err := write(body, etag); err != nil {
		return "", err
	}
	return etag, nil
}
