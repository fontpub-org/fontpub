// Package version implements Numeric Dot versioning logic for Fontpub.
package version

import (
	"errors"
	"strconv"
	"strings"
)

var (
	ErrInvalidVersion = errors.New("invalid version format: must contain only digits and dots")
	ErrEmptyVersion   = errors.New("version string cannot be empty")
)

// normalizeVersion removes the leading 'v' or 'V' prefix if present.
func normalizeVersion(v string) string {
	if len(v) > 0 && (v[0] == 'v' || v[0] == 'V') {
		return v[1:]
	}
	return v
}

// parseSegments splits a version string by dots and converts each segment to an integer.
func parseSegments(v string) ([]int, error) {
	if v == "" {
		return nil, ErrEmptyVersion
	}

	parts := strings.Split(v, ".")
	segments := make([]int, len(parts))

	for i, part := range parts {
		if part == "" {
			return nil, ErrInvalidVersion
		}
		// Check for non-digit characters
		for _, c := range part {
			if c < '0' || c > '9' {
				return nil, ErrInvalidVersion
			}
		}
		num, err := strconv.Atoi(part)
		if err != nil {
			return nil, ErrInvalidVersion
		}
		if num < 0 {
			return nil, ErrInvalidVersion
		}
		segments[i] = num
	}

	return segments, nil
}

// IsValid checks if a version string is valid according to Numeric Dot format.
func IsValid(v string) bool {
	normalized := normalizeVersion(v)
	_, err := parseSegments(normalized)
	return err == nil
}

// Compare compares two version strings using Numeric Dot algorithm.
// Returns:
//
//	-1 if v1 < v2
//	 0 if v1 == v2
//	 1 if v1 > v2
//
// Returns an error if either version string is invalid.
func Compare(v1, v2 string) (int, error) {
	// Normalize: remove leading 'v' or 'V'
	v1 = normalizeVersion(v1)
	v2 = normalizeVersion(v2)

	// Parse segments
	seg1, err := parseSegments(v1)
	if err != nil {
		return 0, err
	}
	seg2, err := parseSegments(v2)
	if err != nil {
		return 0, err
	}

	// Pad shorter slice with zeros
	maxLen := len(seg1)
	if len(seg2) > maxLen {
		maxLen = len(seg2)
	}

	for len(seg1) < maxLen {
		seg1 = append(seg1, 0)
	}
	for len(seg2) < maxLen {
		seg2 = append(seg2, 0)
	}

	// Compare segment by segment from left to right
	for i := 0; i < maxLen; i++ {
		if seg1[i] < seg2[i] {
			return -1, nil
		}
		if seg1[i] > seg2[i] {
			return 1, nil
		}
	}

	return 0, nil
}

// IsNewer returns true if v1 is newer (greater) than v2.
func IsNewer(v1, v2 string) (bool, error) {
	result, err := Compare(v1, v2)
	if err != nil {
		return false, err
	}
	return result > 0, nil
}
