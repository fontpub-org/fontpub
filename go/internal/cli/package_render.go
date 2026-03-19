package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

func printPackageInitSummary(w io.Writer, root string, manifest protocol.Manifest, assets []inspection, inferences []inferenceRecord, conflicts []conflictRecord, unresolved []string) {
	fmt.Fprintf(w, "Repository: %s\n", root)
	fmt.Fprintln(w, "Discovered assets:")
	for _, asset := range assets {
		fmt.Fprintf(
			w,
			"  - %s [%s] style=%s (%s) weight=%d (%s)\n",
			asset.Path,
			asset.Format,
			asset.Style,
			humanizeInferenceSource(asset.styleSource),
			asset.Weight,
			humanizeInferenceSource(asset.weightSource),
		)
		if asset.Name != "" {
			fmt.Fprintf(w, "    family=%s (%s)\n", asset.Name, humanizeInferenceSource(asset.nameSource))
		}
	}

	inferenceByField := map[string]inferenceRecord{}
	for _, inference := range inferences {
		inferenceByField[inference.Field] = inference
	}

	fmt.Fprintln(w, "Manifest fields:")
	printManifestFieldSummary(w, "name", manifest.Name, inferenceByField["name"])
	printManifestFieldSummary(w, "author", manifest.Author, inferenceByField["author"])
	printManifestFieldSummary(w, "version", manifest.Version, inferenceByField["version"])
	printManifestFieldSummary(w, "license", manifest.License, inferenceByField["license"])

	if len(conflicts) > 0 {
		fmt.Fprintln(w, "Conflicts:")
		for _, conflict := range conflicts {
			status := "unresolved"
			if conflict.Resolved {
				status = "resolved"
			}
			fmt.Fprintf(w, "  %s (%s)\n", conflict.Field, status)
			if conflict.Resolved && conflict.ChosenValue != "" {
				fmt.Fprintf(w, "    chosen: %s\n", conflict.ChosenValue)
			}
			for _, candidate := range conflict.Candidates {
				fmt.Fprintf(w, "    - %s (%s)\n", candidate.Value, humanizeInferenceSource(candidate.Source))
			}
		}
	} else {
		fmt.Fprintln(w, "Conflicts: none")
	}
	if len(unresolved) > 0 {
		fmt.Fprintf(w, "Unresolved fields: %s\n", strings.Join(unresolved, ", "))
	} else {
		fmt.Fprintln(w, "Unresolved fields: none")
	}
	fmt.Fprintln(w)
}

func printManifestFieldSummary(w io.Writer, field, value string, inference inferenceRecord) {
	if value == "" {
		fmt.Fprintf(w, "  %s: unresolved\n", field)
		return
	}
	source := inference.Source
	if source == "" {
		source = "user_input"
	}
	fmt.Fprintf(w, "  %s: %s (%s)\n", field, value, humanizeInferenceSource(source))
}

func printInspectionSummary(w io.Writer, info inspection) {
	fmt.Fprintf(w, "Path: %s\n", info.Path)
	fmt.Fprintf(w, "Format: %s\n", info.Format)
	if info.Name != "" {
		fmt.Fprintf(w, "Family: %s (%s)\n", info.Name, humanizeInferenceSource(info.nameSource))
	}
	fmt.Fprintf(w, "Style: %s (%s)\n", info.Style, humanizeInferenceSource(info.styleSource))
	fmt.Fprintf(w, "Weight: %d (%s)\n", info.Weight, humanizeInferenceSource(info.weightSource))
}

func humanizeInferenceSource(source string) string {
	switch source {
	case "embedded_metadata":
		return "embedded metadata"
	case "group_embedded_metadata":
		return "grouped embedded metadata"
	case "repository_readme":
		return "repository README"
	case "repository_owner":
		return "repository owner"
	case "repository_changelog":
		return "repository changelog"
	case "repository_tag":
		return "repository tag"
	case "filename_heuristic":
		return "filename heuristic"
	case "user_input":
		return "user input"
	default:
		return source
	}
}
