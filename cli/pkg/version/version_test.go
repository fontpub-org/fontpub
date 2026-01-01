package version

import (
	"testing"
)

func TestIsValid(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		// Valid versions
		{"1.0", true},
		{"1.0.0", true},
		{"1.500", true},
		{"v1.0", true},
		{"V1.0", true},
		{"0.0.1", true},
		{"10.20.30", true},
		{"1", true},

		// Invalid versions
		{"", false},
		{"v", false},
		{"1.0.0-alpha", false},
		{"1.0.0+build", false},
		{"1.0.0a", false},
		{"a.b.c", false},
		{"1..0", false},
		{".1.0", false},
		{"1.0.", false},
		{"-1.0", false},
		{"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0", true}, // Long but valid
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := IsValid(tt.version)
			if got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		v1      string
		v2      string
		want    int
		wantErr bool
	}{
		// Spec examples
		{"1.500", "1.5", 1, false},   // 1.500 > 1.5.0
		{"2.0.1", "2.0", 1, false},   // 2.0.1 > 2.0.0
		{"v1.2", "1.2", 0, false},    // v1.2 == 1.2

		// Equal versions
		{"1.0.0", "1.0.0", 0, false},
		{"1.0", "1.0.0", 0, false},
		{"1", "1.0.0", 0, false},
		{"V1.0", "v1.0", 0, false},

		// Less than
		{"1.0", "1.1", -1, false},
		{"1.0", "2.0", -1, false},
		{"1.9", "1.10", -1, false},
		{"1.0.0", "1.0.1", -1, false},

		// Greater than
		{"2.0", "1.0", 1, false},
		{"1.1", "1.0", 1, false},
		{"1.10", "1.9", 1, false},
		{"1.0.1", "1.0.0", 1, false},

		// Edge cases with zeros
		{"0.0.0", "0.0.0", 0, false},
		{"0.0.1", "0.0.0", 1, false},
		{"0.1.0", "0.0.1", 1, false},

		// Different segment counts
		{"1.2.3.4", "1.2.3", 1, false},
		{"1.2.3", "1.2.3.4", -1, false},
		{"1.2.3.0", "1.2.3", 0, false},

		// Invalid versions should error
		{"1.0.0-alpha", "1.0.0", 0, true},
		{"1.0.0", "invalid", 0, true},
		{"", "1.0.0", 0, true},
		{"1.0.0", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got, err := Compare(tt.v1, tt.v2)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compare(%q, %q) error = %v, wantErr %v", tt.v1, tt.v2, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Compare(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		v1      string
		v2      string
		want    bool
		wantErr bool
	}{
		{"2.0", "1.0", true, false},
		{"1.0", "2.0", false, false},
		{"1.0", "1.0", false, false},
		{"1.500", "1.5", true, false},
		{"invalid", "1.0", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_newer_than_"+tt.v2, func(t *testing.T) {
			got, err := IsNewer(tt.v1, tt.v2)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsNewer(%q, %q) error = %v, wantErr %v", tt.v1, tt.v2, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

