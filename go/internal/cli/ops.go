package cli

type Finding struct {
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Subject  string         `json:"subject"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details"`
}

type PackageCheckResult struct {
	PackageID string    `json:"package_id"`
	OK        bool      `json:"ok"`
	Findings  []Finding `json:"findings"`
}

type PlannedAction struct {
	Type       string `json:"type"`
	PackageID  string `json:"package_id"`
	VersionKey string `json:"version_key,omitempty"`
	Path       string `json:"path,omitempty"`
}
