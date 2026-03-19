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

func TestApplyStemGroupingPrefersEmbeddedMetadataAcrossFormats(t *testing.T) {
	assets := []inspection{
		{
			Path:         "fonts/0xProto-Regular.otf",
			Format:       "otf",
			Style:        "normal",
			Weight:       400,
			Name:         "0x Proto",
			styleSource:  "embedded_metadata",
			weightSource: "embedded_metadata",
			nameSource:   "embedded_metadata",
		},
		{
			Path:         "fonts/0xProto-Regular.woff2",
			Format:       "woff2",
			Style:        "normal",
			Weight:       400,
			Name:         "0xProto",
			styleSource:  "filename_heuristic",
			weightSource: "filename_heuristic",
			nameSource:   "filename_heuristic",
		},
	}

	grouped := applyStemGrouping(assets)
	if grouped[1].Name != "0x Proto" {
		t.Fatalf("unexpected grouped name: %q", grouped[1].Name)
	}
	if grouped[1].nameSource != "group_embedded_metadata" {
		t.Fatalf("unexpected grouped source: %q", grouped[1].nameSource)
	}
}
