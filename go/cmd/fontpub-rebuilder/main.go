package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/ma/fontpub/go/internal/indexer/artifacts"
	"github.com/ma/fontpub/go/internal/indexer/rebuilder"
)

func main() {
	var packageID string
	var artifactsDir string
	flag.StringVar(&packageID, "package-id", "", "rebuild only a single package")
	flag.StringVar(&artifactsDir, "artifacts-dir", os.Getenv("FONTPUB_ARTIFACTS_DIR"), "directory containing published Fontpub artifacts")
	flag.Parse()

	if artifactsDir == "" {
		log.Fatal("artifacts directory is required via -artifacts-dir or FONTPUB_ARTIFACTS_DIR")
	}

	store := artifacts.NewFileStore(artifactsDir)
	r := rebuilder.Rebuilder{Store: store}

	var (
		result rebuilder.Result
		err    error
	)
	if packageID == "" {
		result, err = r.RebuildAll(context.Background())
	} else {
		result, err = r.RebuildPackage(context.Background(), packageID)
	}
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("rebuilt packages=%d versions=%d", result.Packages, result.Versions)
}
