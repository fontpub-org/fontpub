package rebuilder

import (
	"context"
	"fmt"
	"sort"

	"github.com/ma/fontpub/go/internal/indexer/artifacts"
	"github.com/ma/fontpub/go/internal/indexer/derive"
	"github.com/ma/fontpub/go/internal/protocol"
)

type Rebuilder struct {
	Store artifacts.Store
}

type Result struct {
	Packages int
	Versions int
}

func (r Rebuilder) RebuildAll(ctx context.Context) (Result, error) {
	if r.Store == nil {
		return Result{}, fmt.Errorf("artifact store is not configured")
	}
	allDetails, err := r.Store.ListAllVersionedPackageDetails(ctx)
	if err != nil {
		return Result{}, err
	}

	grouped := make(map[string][]protocol.VersionedPackageDetail)
	for _, detail := range allDetails {
		grouped[detail.PackageID] = append(grouped[detail.PackageID], detail)
	}
	packageIDs := make([]string, 0, len(grouped))
	for packageID := range grouped {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)

	for _, packageID := range packageIDs {
		if err := r.writePackageDerived(ctx, packageID, grouped[packageID]); err != nil {
			return Result{}, err
		}
	}
	if err := r.writeRootIndex(ctx, allDetails); err != nil {
		return Result{}, err
	}
	return Result{Packages: len(packageIDs), Versions: len(allDetails)}, nil
}

func (r Rebuilder) RebuildPackage(ctx context.Context, packageID string) (Result, error) {
	if r.Store == nil {
		return Result{}, fmt.Errorf("artifact store is not configured")
	}
	details, err := r.Store.ListPackageVersionedPackageDetails(ctx, packageID)
	if err != nil {
		return Result{}, err
	}
	if len(details) == 0 {
		return Result{}, fmt.Errorf("package not found: %s", packageID)
	}
	if err := r.writePackageDerived(ctx, packageID, details); err != nil {
		return Result{}, err
	}
	allDetails, err := r.Store.ListAllVersionedPackageDetails(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := r.writeRootIndex(ctx, allDetails); err != nil {
		return Result{}, err
	}
	return Result{Packages: 1, Versions: len(details)}, nil
}

func (r Rebuilder) writePackageDerived(ctx context.Context, packageID string, details []protocol.VersionedPackageDetail) error {
	packageIndex, latestDetail, err := derive.BuildPackageVersionsIndex(packageID, append([]protocol.VersionedPackageDetail(nil), details...))
	if err != nil {
		return err
	}
	packageIndexBytes, err := protocol.MarshalCanonical(packageIndex)
	if err != nil {
		return err
	}
	if err := r.Store.PutPackageVersionsIndex(ctx, packageID, packageIndex, packageIndexBytes, derive.ComputeETag(packageIndexBytes)); err != nil {
		return err
	}
	latestBytes, err := protocol.MarshalCanonical(latestDetail)
	if err != nil {
		return err
	}
	if err := r.Store.PutLatestAlias(ctx, packageID, latestBytes, derive.ComputeETag(latestBytes)); err != nil {
		return err
	}
	return nil
}

func (r Rebuilder) writeRootIndex(ctx context.Context, allDetails []protocol.VersionedPackageDetail) error {
	rootIndex, err := derive.BuildRootIndex(allDetails)
	if err != nil {
		return err
	}
	rootBytes, err := protocol.MarshalCanonical(rootIndex)
	if err != nil {
		return err
	}
	return r.Store.PutRootIndex(ctx, rootIndex, rootBytes, derive.ComputeETag(rootBytes))
}
