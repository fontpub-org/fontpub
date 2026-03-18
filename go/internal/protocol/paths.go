package protocol

import (
	"fmt"
	"strings"
	"unicode"
)

func ValidateAssetPath(path string) error {
	if path == "" {
		return fmt.Errorf("ASSET_PATH_INVALID: empty path")
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return fmt.Errorf("ASSET_PATH_INVALID: absolute or trailing slash")
	}
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("ASSET_PATH_INVALID: invalid segment")
		}
		if strings.TrimSpace(segment) != segment {
			return fmt.Errorf("ASSET_PATH_INVALID: whitespace")
		}
		if strings.Contains(segment, ":") {
			return fmt.Errorf("ASSET_PATH_INVALID: colon")
		}
		for _, r := range segment {
			if r == 0 || unicode.IsControl(r) {
				return fmt.Errorf("ASSET_PATH_INVALID: control char")
			}
		}
	}
	return nil
}

func FormatFromPath(path string) (string, error) {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".otf"):
		return "otf", nil
	case strings.HasSuffix(strings.ToLower(path), ".ttf"):
		return "ttf", nil
	case strings.HasSuffix(strings.ToLower(path), ".woff2"):
		return "woff2", nil
	default:
		return "", fmt.Errorf("ASSET_FORMAT_NOT_ALLOWED: unsupported extension")
	}
}
