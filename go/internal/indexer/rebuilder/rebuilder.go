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
	return r.run(ctx, "")
}

func (r Rebuilder) RebuildPackage(ctx context.Context, packageID string) (Result, error) {
	if r.Store == nil {
		return Result{}, fmt.Errorf("artifact store is not configured")
	}
	return r.run(ctx, packageID)
}

func (r Rebuilder) run(ctx context.Context, packageID string) (Result, error) {
	if r.Store == nil {
		return Result{}, fmt.Errorf("artifact store is not configured")
	}

	allDetails, err := r.Store.ListAllVersionedPackageDetails(ctx)
	if err != nil {
		return Result{}, err
	}
	grouped := groupPackageDetails(allDetails)

	packageIDs, versionCount, err := rebuildScope(grouped, packageID)
	if err != nil {
		return Result{}, err
	}
	for _, currentPackageID := range packageIDs {
		if err := r.writePackageDerived(ctx, currentPackageID, grouped[currentPackageID]); err != nil {
			return Result{}, err
		}
	}
	if err := r.writeRootIndex(ctx, allDetails); err != nil {
		return Result{}, err
	}
	return Result{Packages: len(packageIDs), Versions: versionCount}, nil
}

func (r Rebuilder) writePackageDerived(ctx context.Context, packageID string, details []protocol.VersionedPackageDetail) error {
	_, err := deriveddocs.WritePackage(ctx, r.Store, packageID, details)
	return err
}

func (r Rebuilder) writeRootIndex(ctx context.Context, allDetails []protocol.VersionedPackageDetail) error {
	_, err := deriveddocs.WriteRoot(ctx, r.Store, allDetails)
	return err
}

func groupPackageDetails(details []protocol.VersionedPackageDetail) map[string][]protocol.VersionedPackageDetail {
	grouped := make(map[string][]protocol.VersionedPackageDetail)
	for _, detail := range details {
		grouped[detail.PackageID] = append(grouped[detail.PackageID], detail)
	}
	return grouped
}

func rebuildScope(grouped map[string][]protocol.VersionedPackageDetail, packageID string) ([]string, int, error) {
	if packageID != "" {
		details := grouped[packageID]
		if len(details) == 0 {
			return nil, 0, fmt.Errorf("package not found: %s", packageID)
		}
		return []string{packageID}, len(details), nil
	}

	packageIDs := make([]string, 0, len(grouped))
	versionCount := 0
	for currentPackageID, details := range grouped {
		packageIDs = append(packageIDs, currentPackageID)
		versionCount += len(details)
	}
	sort.Strings(packageIDs)
	return packageIDs, versionCount, nil
}
