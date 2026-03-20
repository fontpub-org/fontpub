package rebuilder

import (
	"context"
	"fmt"
	"sort"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/deriveddocs"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
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
	_, err := deriveddocs.WritePackage(ctx, r.Store, packageID, details)
	return err
}

func (r Rebuilder) writeRootIndex(ctx context.Context, allDetails []protocol.VersionedPackageDetail) error {
	_, err := deriveddocs.WriteRoot(ctx, r.Store, allDetails)
	return err
}
