package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var protocolRoot = filepath.Join("..", "..", "..", "protocol")

func readFixture(t *testing.T, name string, target any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(protocolRoot, "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
}

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(protocolRoot, "golden", name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return []byte(strings.TrimSuffix(string(data), "\n"))
}

func TestSchemasAreValidJSON(t *testing.T) {
	err := filepath.Walk(filepath.Join(protocolRoot, "schemas"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Errorf("schema %s is not valid JSON: %v", path, err)
			return nil
		}
		if _, ok := decoded["$id"]; !ok {
			t.Errorf("schema %s missing $id", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk schemas: %v", err)
	}
}

func TestVersionFixtures(t *testing.T) {
	var fixture struct {
		Valid []struct {
			Input      string `json:"input"`
			VersionKey string `json:"version_key"`
		} `json:"valid"`
		Invalid    []string `json:"invalid"`
		Precedence []struct {
			Left  string `json:"left"`
			Right string `json:"right"`
			Cmp   int    `json:"cmp"`
		} `json:"precedence"`
	}
	readFixture(t, "versioning.json", &fixture)
	for _, tc := range fixture.Valid {
		got, err := NormalizeVersionKey(tc.Input)
		if err != nil {
			t.Fatalf("NormalizeVersionKey(%q): %v", tc.Input, err)
		}
		if got != tc.VersionKey {
			t.Fatalf("NormalizeVersionKey(%q)=%q want %q", tc.Input, got, tc.VersionKey)
		}
	}
	for _, input := range fixture.Invalid {
		if _, err := NormalizeVersionKey(input); err == nil {
			t.Fatalf("NormalizeVersionKey(%q) unexpectedly succeeded", input)
		}
	}
	for _, tc := range fixture.Precedence {
		got, err := CompareVersions(tc.Left, tc.Right)
		if err != nil {
			t.Fatalf("CompareVersions(%q,%q): %v", tc.Left, tc.Right, err)
		}
		if got != tc.Cmp {
			t.Fatalf("CompareVersions(%q,%q)=%d want %d", tc.Left, tc.Right, got, tc.Cmp)
		}
	}
}

func TestPathFixtures(t *testing.T) {
	var fixture struct {
		Valid   []string `json:"valid"`
		Invalid []struct {
			Path  string `json:"path"`
			Error string `json:"error"`
		} `json:"invalid"`
	}
	readFixture(t, "paths.json", &fixture)
	for _, path := range fixture.Valid {
		if err := ValidateAssetPath(path); err != nil {
			t.Fatalf("ValidateAssetPath(%q): %v", path, err)
		}
	}
	for _, tc := range fixture.Invalid {
		err := ValidateAssetPath(tc.Path)
		if err == nil || !strings.Contains(err.Error(), tc.Error) {
			t.Fatalf("ValidateAssetPath(%q)=%v want %s", tc.Path, err, tc.Error)
		}
	}
}

func TestManifestFixtures(t *testing.T) {
	var fixture struct {
		Valid []struct {
			Name     string   `json:"name"`
			Manifest Manifest `json:"manifest"`
		} `json:"valid"`
		Invalid []struct {
			Name     string   `json:"name"`
			Manifest Manifest `json:"manifest"`
			Error    string   `json:"error"`
		} `json:"invalid"`
	}
	readFixture(t, "manifests.json", &fixture)
	for _, tc := range fixture.Valid {
		if err := ValidateManifest(tc.Manifest); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
	}
	for _, tc := range fixture.Invalid {
		err := ValidateManifest(tc.Manifest)
		if err == nil || !strings.Contains(err.Error(), tc.Error) {
			t.Fatalf("%s: got %v want error containing %s", tc.Name, err, tc.Error)
		}
	}
}

func TestOIDCFixtures(t *testing.T) {
	var fixture struct {
		Valid []struct {
			Name   string     `json:"name"`
			Claims OIDCClaims `json:"claims"`
		} `json:"valid"`
		Invalid []struct {
			Name   string     `json:"name"`
			Claims OIDCClaims `json:"claims"`
			Error  string     `json:"error"`
		} `json:"invalid"`
	}
	readFixture(t, "oidc-claims.json", &fixture)
	for _, tc := range fixture.Valid {
		if err := ValidateOIDCClaims(tc.Claims); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
	}
	for _, tc := range fixture.Invalid {
		err := ValidateOIDCClaims(tc.Claims)
		if err == nil || !strings.Contains(err.Error(), tc.Error) {
			t.Fatalf("%s: got %v want error containing %s", tc.Name, err, tc.Error)
		}
	}
}

func TestImmutabilityFixtures(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name      string                 `json:"name"`
			Equal     bool                   `json:"equal"`
			Existing  VersionedPackageDetail `json:"existing"`
			Candidate VersionedPackageDetail `json:"candidate"`
		} `json:"cases"`
	}
	readFixture(t, "immutability.json", &fixture)
	for _, tc := range fixture.Cases {
		got := ImmutableEqual(tc.Existing, tc.Candidate)
		if got != tc.Equal {
			t.Fatalf("%s: ImmutableEqual=%v want %v", tc.Name, got, tc.Equal)
		}
	}
}

func TestCLIJSONFixtures(t *testing.T) {
	var fixture map[string]struct {
		Valid CLIEnvelope `json:"valid"`
	}
	readFixture(t, "cli-json.json", &fixture)
	for name, record := range fixture {
		schemaFile := schemaFileNameForCLICommand(record.Valid.Command)
		if schemaFile == "" {
			t.Fatalf("%s: missing schema mapping for command %q", name, record.Valid.Command)
		}
		if err := ValidateCLISchema(schemaFile, record.Valid); err != nil {
			t.Fatalf("%s schema validation: %v", name, err)
		}
	}
	if err := ValidateCLISchema("envelope.schema.json", fixture["list"].Valid); err != nil {
		t.Fatalf("list envelope: %v", err)
	}
	if err := ValidateCLISchema("envelope.schema.json", fixture["verify_failure"].Valid); err != nil {
		t.Fatalf("verify_failure envelope: %v", err)
	}
	if err := ValidateCLISchema("envelope.schema.json", fixture["package_preview"].Valid); err != nil {
		t.Fatalf("package_preview envelope: %v", err)
	}
	if err := ValidateCLISchema("envelope.schema.json", fixture["repair_failure"].Valid); err != nil {
		t.Fatalf("repair_failure envelope: %v", err)
	}
	if err := ValidateCLIEnvelope(fixture["list"].Valid); err != nil {
		t.Fatalf("list envelope validator: %v", err)
	}
	if err := ValidateStatusResult(fixture["status"].Valid); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := ValidateVerifyResult(fixture["verify_failure"].Valid); err != nil {
		t.Fatalf("verify_failure: %v", err)
	}
	if err := ValidatePackageInitResult(fixture["package_init"].Valid); err != nil {
		t.Fatalf("package_init: %v", err)
	}
	if err := ValidatePackagePreviewResult(fixture["package_preview"].Valid); err != nil {
		t.Fatalf("package_preview: %v", err)
	}
	if err := ValidateRepairResult(fixture["repair_failure"].Valid); err != nil {
		t.Fatalf("repair_failure: %v", err)
	}
}

func TestCanonicalGoldenOutputs(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		golden string
	}{
		{
			name: "root-index",
			value: RootIndex{
				SchemaVersion: "1",
				GeneratedAt:   "2026-01-02T00:00:00Z",
				Packages: map[string]RootIndexPackage{
					"example/family": {
						LatestVersion:     "1.2.3",
						LatestVersionKey:  "1.2.3",
						LatestPublishedAt: "2026-01-02T00:00:00Z",
					},
				},
			},
			golden: "root-index.json",
		},
		{
			name: "package-versions-index",
			value: PackageVersionsIndex{
				SchemaVersion:    "1",
				PackageID:        "example/family",
				LatestVersion:    "1.2.3",
				LatestVersionKey: "1.2.3",
				Versions: []PackageVersionRecord{
					{Version: "1.2.3", VersionKey: "1.2.3", PublishedAt: "2026-01-02T00:00:00Z", URL: "/v1/packages/example/family/versions/1.2.3.json"},
					{Version: "1.2.2", VersionKey: "1.2.2", PublishedAt: "2026-01-01T00:00:00Z", URL: "/v1/packages/example/family/versions/1.2.2.json"},
				},
			},
			golden: "package-versions-index.json",
		},
		{
			name: "versioned-package-detail",
			value: VersionedPackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.2.3",
				VersionKey:    "1.2.3",
				PublishedAt:   "2026-01-02T00:00:00Z",
				GitHub:        GitHubRef{Owner: "example", Repo: "family", SHA: "0123456789abcdef0123456789abcdef01234567"},
				ManifestURL:   "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/fontpub.json",
				Assets: []VersionedAsset{
					{Path: "dist/ExampleSans-Regular.otf", URL: "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), Format: "otf", Style: "normal", Weight: 400, SizeBytes: 1000},
					{Path: "dist/ExampleSans-Italic.otf", URL: "https://raw.githubusercontent.com/example/family/0123456789abcdef0123456789abcdef01234567/dist/ExampleSans-Italic.otf", SHA256: strings.Repeat("b", 64), Format: "otf", Style: "italic", Weight: 400, SizeBytes: 1001},
				},
			},
			golden: "versioned-package-detail.json",
		},
		{
			name: "candidate-package-detail",
			value: CandidatePackageDetail{
				SchemaVersion: "1",
				PackageID:     "example/family",
				DisplayName:   "Example Sans",
				Author:        "Example Studio",
				License:       "OFL-1.1",
				Version:       "1.2.3",
				VersionKey:    "1.2.3",
				Source:        CandidateSource{Kind: "local_repository", RootPath: "/Users/example/family"},
				Assets: []CandidateAsset{
					{Path: "dist/ExampleSans-Regular.otf", SHA256: strings.Repeat("a", 64), Format: "otf", Style: "normal", Weight: 400, SizeBytes: 1000},
				},
			},
			golden: "candidate-package-detail.json",
		},
		{
			name: "lockfile",
			value: func() Lockfile {
				active := "1.2.3"
				symlink := "/Users/example/Fonts/example--family--ExampleSans-Regular.otf"
				return Lockfile{
					SchemaVersion: "1",
					GeneratedAt:   "2026-01-02T00:00:00Z",
					Packages: map[string]LockedPackage{
						"example/family": {
							ActiveVersionKey: &active,
							InstalledVersions: map[string]InstalledVersion{
								"1.2.3": {
									Version:     "1.2.3",
									VersionKey:  "1.2.3",
									InstalledAt: "2026-01-02T00:00:00Z",
									Assets: []LockedAsset{
										{
											Path:        "dist/ExampleSans-Regular.otf",
											SHA256:      strings.Repeat("a", 64),
											LocalPath:   "/Users/example/.fontpub/packages/example/family/1.2.3/dist/ExampleSans-Regular.otf",
											Active:      true,
											SymlinkPath: &symlink,
										},
									},
								},
							},
						},
					},
				}
			}(),
			golden: "lockfile.json",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MarshalCanonical(tc.value)
			if err != nil {
				t.Fatalf("MarshalCanonical: %v", err)
			}
			want := readGolden(t, tc.golden)
			if string(got) != string(want) {
				t.Fatalf("golden mismatch\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
