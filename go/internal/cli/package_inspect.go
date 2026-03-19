package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func scanFontAssets(root string) ([]inspection, error) {
	out := make([]inspection, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		format, err := protocol.FormatFromPath(rel)
		if err != nil {
			return nil
		}
		info, err := inspectFontFile(path, rel, format)
		if err != nil {
			return err
		}
		out = append(out, info)
		return nil
	})
	if err != nil {
		return nil, &CLIError{Code: "INTERNAL_ERROR", Message: "could not scan repository", Details: map[string]any{"root_path": root, "reason": err.Error()}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func inspectFontPath(path, root string) (inspection, error) {
	rel := path
	fullPath := path
	if !filepath.IsAbs(path) {
		rel = filepath.ToSlash(path)
		fullPath = filepath.Join(root, filepath.FromSlash(rel))
	}
	if _, err := os.Stat(fullPath); err != nil {
		return inspection{}, &CLIError{Code: "LOCAL_FILE_MISSING", Message: "font file does not exist", Details: map[string]any{"path": path}}
	}
	format, err := protocol.FormatFromPath(rel)
	if err != nil {
		return inspection{}, protocolErrorToCLI(err)
	}
	return inspectFontFile(fullPath, rel, format)
}

func inspectFontFile(fullPath, relPath, format string) (inspection, error) {
	style, weight, name := inferFromFilename(relPath)
	info := inspection{
		Path:         relPath,
		Format:       format,
		Style:        style,
		Weight:       weight,
		Name:         name,
		styleSource:  "filename_heuristic",
		weightSource: "filename_heuristic",
		nameSource:   "filename_heuristic",
	}

	body, err := os.ReadFile(fullPath)
	if err != nil {
		return inspection{}, &CLIError{Code: "INTERNAL_ERROR", Message: "could not read font file", Details: map[string]any{"path": relPath, "reason": err.Error()}}
	}
	if meta, ok := parseEmbeddedFontMetadata(relPath, body); ok {
		if meta.Style != "" {
			info.Style = meta.Style
			info.styleSource = "embedded_metadata"
		}
		if meta.Weight > 0 {
			info.Weight = meta.Weight
			info.weightSource = "embedded_metadata"
		}
		if meta.Family != "" {
			info.Name = meta.Family
			info.nameSource = "embedded_metadata"
		}
	}
	return info, nil
}

func applyStemGrouping(assets []inspection) []inspection {
	grouped := make(map[string][]int)
	for i, asset := range assets {
		grouped[stemGroupKey(asset.Path)] = append(grouped[stemGroupKey(asset.Path)], i)
	}
	for _, indexes := range grouped {
		if len(indexes) < 2 {
			continue
		}
		bestNameValue, bestNameSource, hasName := bestGroupedStringField(assets, indexes, func(asset inspection) (string, string) {
			return asset.Name, asset.nameSource
		})
		bestStyleValue, bestStyleSource, hasStyle := bestGroupedStringField(assets, indexes, func(asset inspection) (string, string) {
			return asset.Style, asset.styleSource
		})
		bestWeightValue, bestWeightSource, hasWeight := bestGroupedIntField(assets, indexes, func(asset inspection) (int, string) {
			return asset.Weight, asset.weightSource
		})
		for _, idx := range indexes {
			if hasName && groupedSourcePriority(bestNameSource) > groupedSourcePriority(assets[idx].nameSource) {
				assets[idx].Name = bestNameValue
				assets[idx].nameSource = groupedInheritedSource(bestNameSource)
			}
			if hasStyle && groupedSourcePriority(bestStyleSource) > groupedSourcePriority(assets[idx].styleSource) {
				assets[idx].Style = bestStyleValue
				assets[idx].styleSource = groupedInheritedSource(bestStyleSource)
			}
			if hasWeight && groupedSourcePriority(bestWeightSource) > groupedSourcePriority(assets[idx].weightSource) {
				assets[idx].Weight = bestWeightValue
				assets[idx].weightSource = groupedInheritedSource(bestWeightSource)
			}
		}
	}
	return assets
}

func stemGroupKey(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if dir == "." || dir == "" {
		return base
	}
	return dir + "/" + base
}

func bestGroupedStringField(assets []inspection, indexes []int, get func(inspection) (string, string)) (string, string, bool) {
	bestValue := ""
	bestSource := ""
	bestPriority := -1
	bestFormatPriority := -1
	for _, idx := range indexes {
		value, source := get(assets[idx])
		if strings.TrimSpace(value) == "" {
			continue
		}
		priority := groupedSourcePriority(source)
		formatPriority := groupedFormatPriority(assets[idx].Format)
		if priority > bestPriority || (priority == bestPriority && formatPriority > bestFormatPriority) {
			bestValue = value
			bestSource = source
			bestPriority = priority
			bestFormatPriority = formatPriority
		}
	}
	return bestValue, bestSource, bestPriority >= 0
}

func bestGroupedIntField(assets []inspection, indexes []int, get func(inspection) (int, string)) (int, string, bool) {
	bestValue := 0
	bestSource := ""
	bestPriority := -1
	bestFormatPriority := -1
	for _, idx := range indexes {
		value, source := get(assets[idx])
		if value <= 0 {
			continue
		}
		priority := groupedSourcePriority(source)
		formatPriority := groupedFormatPriority(assets[idx].Format)
		if priority > bestPriority || (priority == bestPriority && formatPriority > bestFormatPriority) {
			bestValue = value
			bestSource = source
			bestPriority = priority
			bestFormatPriority = formatPriority
		}
	}
	return bestValue, bestSource, bestPriority >= 0
}

func groupedSourcePriority(source string) int {
	switch source {
	case "embedded_metadata":
		return 30
	case "group_embedded_metadata":
		return 25
	case "filename_heuristic":
		return 10
	default:
		return 0
	}
}

func groupedFormatPriority(format string) int {
	switch format {
	case "otf":
		return 3
	case "ttf":
		return 2
	case "woff2":
		return 1
	default:
		return 0
	}
}

func groupedInheritedSource(source string) string {
	if source == "embedded_metadata" || source == "group_embedded_metadata" {
		return "group_embedded_metadata"
	}
	return source
}

func inferFromFilename(path string) (string, int, string) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	lower := strings.ToLower(base)
	style := "normal"
	weight := 400
	switch {
	case strings.Contains(lower, "italic"):
		style = "italic"
	case strings.Contains(lower, "oblique"):
		style = "oblique"
	}
	switch {
	case strings.Contains(lower, "thin"):
		weight = 100
	case strings.Contains(lower, "extralight") || strings.Contains(lower, "ultralight"):
		weight = 200
	case strings.Contains(lower, "light"):
		weight = 300
	case strings.Contains(lower, "medium"):
		weight = 500
	case strings.Contains(lower, "semibold") || strings.Contains(lower, "demibold"):
		weight = 600
	case strings.Contains(lower, "extrabold") || strings.Contains(lower, "ultrabold"):
		weight = 800
	case strings.Contains(lower, "bold"):
		weight = 700
	case strings.Contains(lower, "black") || strings.Contains(lower, "heavy"):
		weight = 900
	}
	name := base
	for _, token := range []string{"-thin", "-extralight", "-ultralight", "-light", "-regular", "-italic", "-oblique", "-medium", "-semibold", "-demibold", "-bold", "-extrabold", "-ultrabold", "-black", "-heavy"} {
		if idx := strings.Index(strings.ToLower(name), token); idx > 0 {
			name = name[:idx]
			break
		}
	}
	name = stripVariableAxisSuffix(name)
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = splitCamelCaseName(name)
	name = strings.TrimSpace(name)
	return style, weight, name
}

func stripVariableAxisSuffix(name string) string {
	if idx := strings.Index(name, "["); idx > 0 && strings.HasSuffix(name, "]") {
		return name[:idx]
	}
	return name
}

func splitCamelCaseName(name string) string {
	if name == "" {
		return name
	}
	runes := []rune(name)
	out := make([]rune, 0, len(runes)+4)
	for i, r := range runes {
		if i > 0 && shouldInsertNameSpace(runes, i) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	return string(out)
}

func shouldInsertNameSpace(runes []rune, idx int) bool {
	prev := runes[idx-1]
	current := runes[idx]
	if prev == ' ' || current == ' ' {
		return false
	}
	if prev >= '0' && prev <= '9' {
		return false
	}
	if idx > 1 {
		beforePrev := runes[idx-2]
		if beforePrev >= '0' && beforePrev <= '9' {
			return false
		}
	}
	return isLowerASCII(prev) && isUpperASCII(current)
}

func isLowerASCII(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isUpperASCII(r rune) bool {
	return r >= 'A' && r <= 'Z'
}
