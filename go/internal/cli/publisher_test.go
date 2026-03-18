package cli

import "testing"

func TestInferFromFilenameNormalizesWOFF2FamilyName(t *testing.T) {
	tests := []struct {
		path       string
		wantStyle  string
		wantWeight int
		wantName   string
	}{
		{
			path:       "fonts/static/ZxGamut-Regular.woff2",
			wantStyle:  "normal",
			wantWeight: 400,
			wantName:   "Zx Gamut",
		},
		{
			path:       "fonts/static/ZxGamut-SemiBold.woff2",
			wantStyle:  "normal",
			wantWeight: 600,
			wantName:   "Zx Gamut",
		},
		{
			path:       "fonts/variable/ZxGamut[wght].woff2",
			wantStyle:  "normal",
			wantWeight: 400,
			wantName:   "Zx Gamut",
		},
		{
			path:       "fonts/0xProto-Regular.woff2",
			wantStyle:  "normal",
			wantWeight: 400,
			wantName:   "0xProto",
		},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			style, weight, name := inferFromFilename(tc.path)
			if style != tc.wantStyle || weight != tc.wantWeight || name != tc.wantName {
				t.Fatalf("inferFromFilename(%q)=(%q,%d,%q) want (%q,%d,%q)", tc.path, style, weight, name, tc.wantStyle, tc.wantWeight, tc.wantName)
			}
		})
	}
}
