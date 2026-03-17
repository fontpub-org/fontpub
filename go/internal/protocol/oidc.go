package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

var hex40Pattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func ValidateOIDCClaims(claims OIDCClaims) error {
	if claims.Sub == "" || claims.Repository == "" || claims.RepositoryID == "" || claims.RepositoryOwner == "" ||
		claims.SHA == "" || claims.Ref == "" || claims.WorkflowRef == "" || claims.WorkflowSHA == "" ||
		claims.JTI == "" || claims.EventName == "" {
		return fmt.Errorf("AUTH_CLAIMS_MISSING: required claim missing")
	}
	repository := strings.ToLower(claims.Repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("AUTH_CLAIMS_MISMATCH: invalid repository")
	}
	if claims.RepositoryOwner != parts[0] {
		return fmt.Errorf("AUTH_CLAIMS_MISMATCH: repository owner mismatch")
	}
	if !hex40Pattern.MatchString(claims.SHA) || !hex40Pattern.MatchString(claims.WorkflowSHA) {
		return fmt.Errorf("AUTH_CLAIMS_MISMATCH: invalid sha")
	}
	if !strings.HasPrefix(claims.Ref, "refs/tags/") {
		return fmt.Errorf("WORKFLOW_NOT_ALLOWED: ref must be tag")
	}
	tag := strings.TrimPrefix(claims.Ref, "refs/tags/")
	if _, err := ParseVersion(tag); err != nil {
		return fmt.Errorf("WORKFLOW_NOT_ALLOWED: invalid tag version")
	}
	const prefix = "/.github/workflows/fontpub.yml@"
	if !strings.HasPrefix(claims.WorkflowRef, repository+prefix) {
		return fmt.Errorf("WORKFLOW_NOT_ALLOWED: invalid workflow ref")
	}
	if strings.TrimPrefix(claims.WorkflowRef, repository+prefix) == "" {
		return fmt.Errorf("WORKFLOW_NOT_ALLOWED: empty workflow suffix")
	}
	if claims.EventName != "push" && claims.EventName != "workflow_dispatch" {
		return fmt.Errorf("WORKFLOW_NOT_ALLOWED: invalid event")
	}
	return nil
}
