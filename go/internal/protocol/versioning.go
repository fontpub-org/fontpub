package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

type ParsedVersion struct {
	Literal  string
	Segments []int
	Key      string
}

func ParseVersion(input string) (ParsedVersion, error) {
	if input == "" {
		return ParsedVersion{}, fmt.Errorf("VERSION_INVALID: empty version")
	}
	trimmed := strings.TrimPrefix(strings.TrimPrefix(input, "v"), "V")
	parts := strings.Split(trimmed, ".")
	if len(parts) == 0 {
		return ParsedVersion{}, fmt.Errorf("VERSION_INVALID: no segments")
	}
	segments := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return ParsedVersion{}, fmt.Errorf("VERSION_INVALID: empty segment")
		}
		if len(part) > 1 && part[0] == '0' {
			return ParsedVersion{}, fmt.Errorf("VERSION_INVALID: leading zero")
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return ParsedVersion{}, fmt.Errorf("VERSION_INVALID: non numeric segment")
		}
		segments = append(segments, n)
	}
	keySegments := append([]int(nil), segments...)
	for len(keySegments) > 1 && keySegments[len(keySegments)-1] == 0 {
		keySegments = keySegments[:len(keySegments)-1]
	}
	return ParsedVersion{
		Literal:  input,
		Segments: segments,
		Key:      joinVersionSegments(keySegments),
	}, nil
}

func NormalizeVersionKey(input string) (string, error) {
	parsed, err := ParseVersion(input)
	if err != nil {
		return "", err
	}
	return parsed.Key, nil
}

func CompareVersions(left, right string) (int, error) {
	lv, err := ParseVersion(left)
	if err != nil {
		return 0, err
	}
	rv, err := ParseVersion(right)
	if err != nil {
		return 0, err
	}
	maxLen := len(lv.Segments)
	if len(rv.Segments) > maxLen {
		maxLen = len(rv.Segments)
	}
	for i := 0; i < maxLen; i++ {
		l := 0
		r := 0
		if i < len(lv.Segments) {
			l = lv.Segments[i]
		}
		if i < len(rv.Segments) {
			r = rv.Segments[i]
		}
		if l < r {
			return -1, nil
		}
		if l > r {
			return 1, nil
		}
	}
	return 0, nil
}

func joinVersionSegments(segments []int) string {
	parts := make([]string, len(segments))
	for i, segment := range segments {
		parts[i] = strconv.Itoa(segment)
	}
	return strings.Join(parts, ".")
}
