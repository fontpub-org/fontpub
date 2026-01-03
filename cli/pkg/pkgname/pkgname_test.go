package pkgname

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		username string
		fontname string
		wantErr  bool
	}{
		{"alice/myfont", "alice", "myfont", false},
		{"user123/font-name", "user123", "font-name", false},
		{"org/my.font.name", "org", "my.font.name", false},
		{" alice / myfont ", "alice", "myfont", false}, // trimmed

		// Invalid cases
		{"", "", "", true},
		{"noslash", "", "", true},
		{"/fontname", "", "", true},
		{"username/", "", "", true},
		{"a/b/c", "a", "b/c", false}, // only splits on first /
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			pkg, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if pkg.Username != tt.username {
					t.Errorf("Username = %q, want %q", pkg.Username, tt.username)
				}
				if pkg.Fontname != tt.fontname {
					t.Errorf("Fontname = %q, want %q", pkg.Fontname, tt.fontname)
				}
			}
		})
	}
}

func TestString(t *testing.T) {
	pkg := &Package{Username: "alice", Fontname: "myfont"}
	if got := pkg.String(); got != "alice/myfont" {
		t.Errorf("String() = %q, want %q", got, "alice/myfont")
	}
}

