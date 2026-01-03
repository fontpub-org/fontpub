// Package pkgname handles parsing and validation of package names.
package pkgname

import (
	"errors"
	"strings"
)

var (
	ErrInvalidFormat = errors.New("invalid package name format: expected 'username/fontname'")
	ErrEmptyUsername = errors.New("username cannot be empty")
	ErrEmptyFontname = errors.New("fontname cannot be empty")
)

// Package represents a parsed package name.
type Package struct {
	Username string
	Fontname string
}

// Parse parses a package name in the format "username/fontname".
func Parse(name string) (*Package, error) {
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	username := strings.TrimSpace(parts[0])
	fontname := strings.TrimSpace(parts[1])

	if username == "" {
		return nil, ErrEmptyUsername
	}
	if fontname == "" {
		return nil, ErrEmptyFontname
	}

	return &Package{
		Username: username,
		Fontname: fontname,
	}, nil
}

// String returns the full package name.
func (p *Package) String() string {
	return p.Username + "/" + p.Fontname
}
