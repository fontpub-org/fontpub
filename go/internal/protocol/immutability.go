package protocol

import "sort"

func ImmutableEqual(existing, candidate VersionedPackageDetail) bool {
	if existing.PackageID != candidate.PackageID ||
		existing.Version != candidate.Version ||
		existing.VersionKey != candidate.VersionKey ||
		existing.DisplayName != candidate.DisplayName ||
		existing.Author != candidate.Author ||
		existing.License != candidate.License ||
		existing.GitHub != candidate.GitHub ||
		existing.ManifestURL != candidate.ManifestURL {
		return false
	}

	left := append([]VersionedAsset(nil), existing.Assets...)
	right := append([]VersionedAsset(nil), candidate.Assets...)
	sort.Slice(left, func(i, j int) bool { return left[i].Path < left[j].Path })
	sort.Slice(right, func(i, j int) bool { return right[i].Path < right[j].Path })
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Path != right[i].Path ||
			left[i].URL != right[i].URL ||
			left[i].SHA256 != right[i].SHA256 ||
			left[i].Format != right[i].Format ||
			left[i].Style != right[i].Style ||
			left[i].Weight != right[i].Weight {
			return false
		}
	}
	return true
}
