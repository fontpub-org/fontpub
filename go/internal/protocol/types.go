package protocol

type Manifest struct {
	Name    string         `json:"name"`
	Author  string         `json:"author"`
	Version string         `json:"version"`
	License string         `json:"license"`
	Files   []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	Style  string `json:"style"`
	Weight int    `json:"weight"`
}

type RootIndex struct {
	SchemaVersion string                      `json:"schema_version"`
	GeneratedAt   string                      `json:"generated_at"`
	Packages      map[string]RootIndexPackage `json:"packages"`
}

type RootIndexPackage struct {
	LatestVersion     string `json:"latest_version"`
	LatestVersionKey  string `json:"latest_version_key"`
	LatestPublishedAt string `json:"latest_published_at"`
}

type PackageVersionsIndex struct {
	SchemaVersion    string                 `json:"schema_version"`
	PackageID        string                 `json:"package_id"`
	LatestVersion    string                 `json:"latest_version"`
	LatestVersionKey string                 `json:"latest_version_key"`
	Versions         []PackageVersionRecord `json:"versions"`
}

type PackageVersionRecord struct {
	Version     string `json:"version"`
	VersionKey  string `json:"version_key"`
	PublishedAt string `json:"published_at"`
	URL         string `json:"url"`
}

type VersionedPackageDetail struct {
	SchemaVersion string           `json:"schema_version"`
	PackageID     string           `json:"package_id"`
	DisplayName   string           `json:"display_name"`
	Author        string           `json:"author"`
	License       string           `json:"license"`
	Version       string           `json:"version"`
	VersionKey    string           `json:"version_key"`
	PublishedAt   string           `json:"published_at"`
	GitHub        GitHubRef        `json:"github"`
	ManifestURL   string           `json:"manifest_url"`
	Assets        []VersionedAsset `json:"assets"`
}

type GitHubRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
}

type VersionedAsset struct {
	Path      string `json:"path"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Format    string `json:"format"`
	Style     string `json:"style"`
	Weight    int    `json:"weight"`
	SizeBytes int64  `json:"size_bytes"`
}

type CandidatePackageDetail struct {
	SchemaVersion string           `json:"schema_version"`
	PackageID     string           `json:"package_id"`
	DisplayName   string           `json:"display_name"`
	Author        string           `json:"author"`
	License       string           `json:"license"`
	Version       string           `json:"version"`
	VersionKey    string           `json:"version_key"`
	Source        CandidateSource  `json:"source"`
	Assets        []CandidateAsset `json:"assets"`
}

type CandidateSource struct {
	Kind     string `json:"kind"`
	RootPath string `json:"root_path"`
}

type CandidateAsset struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Format    string `json:"format"`
	Style     string `json:"style"`
	Weight    int    `json:"weight"`
	SizeBytes int64  `json:"size_bytes"`
}

type Lockfile struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at"`
	Packages      map[string]LockedPackage `json:"packages"`
}

type LockedPackage struct {
	InstalledVersions map[string]InstalledVersion `json:"installed_versions"`
	ActiveVersionKey  *string                     `json:"active_version_key,omitempty"`
}

type InstalledVersion struct {
	Version     string        `json:"version"`
	VersionKey  string        `json:"version_key"`
	InstalledAt string        `json:"installed_at"`
	Assets      []LockedAsset `json:"assets"`
}

type LockedAsset struct {
	Path        string  `json:"path"`
	SHA256      string  `json:"sha256"`
	LocalPath   string  `json:"local_path"`
	Active      bool    `json:"active"`
	SymlinkPath *string `json:"symlink_path,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorObject `json:"error"`
}

type ErrorObject struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type OIDCClaims struct {
	Sub             string `json:"sub"`
	Repository      string `json:"repository"`
	RepositoryID    string `json:"repository_id"`
	RepositoryOwner string `json:"repository_owner"`
	SHA             string `json:"sha"`
	Ref             string `json:"ref"`
	WorkflowRef     string `json:"workflow_ref"`
	WorkflowSHA     string `json:"workflow_sha"`
	JTI             string `json:"jti"`
	EventName       string `json:"event_name"`
}

type CLIEnvelope struct {
	SchemaVersion string         `json:"schema_version"`
	OK            bool           `json:"ok"`
	Command       string         `json:"command"`
	Data          map[string]any `json:"data,omitempty"`
	Error         *ErrorObject   `json:"error,omitempty"`
}
