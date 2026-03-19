package cli

import (
	"encoding/json"
	"strings"
)

type CLIError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *CLIError) Error() string {
	return e.Code + ": " + e.Message
}

func asCLIError(err error) *CLIError {
	var cliErr *CLIError
	if err != nil && errorAs(err, &cliErr) {
		return cliErr
	}
	return &CLIError{Code: "INTERNAL_ERROR", Message: err.Error(), Details: map[string]any{}}
}

func structToMap(value any) (map[string]any, *CLIError) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not serialize output", Details: map[string]any{}}
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not build JSON output", Details: map[string]any{}}
	}
	return out, nil
}

func normalizePackageID(packageID string) string {
	return strings.ToLower(packageID)
}

func ensureDetails(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func errorAs(err error, target **CLIError) bool {
	cliErr, ok := err.(*CLIError)
	if !ok {
		return false
	}
	*target = cliErr
	return true
}
