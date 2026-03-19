package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/rebuilder"
)

func main() {
	var packageID string
	var artifactsDir string
	var backend string
	flag.StringVar(&packageID, "package-id", "", "rebuild only a single package")
	flag.StringVar(&artifactsDir, "artifacts-dir", os.Getenv("FONTPUB_ARTIFACTS_DIR"), "directory containing published Fontpub artifacts")
	flag.StringVar(&backend, "artifacts-backend", os.Getenv("FONTPUB_ARTIFACTS_BACKEND"), "artifact backend: file, memory, or s3")
	flag.Parse()

	store, err := artifacts.NewStoreFromEnv(context.Background(), artifacts.EnvStoreOptions{
		DefaultBackend: "file",
		Getenv: func(key string) string {
			switch key {
			case "FONTPUB_ARTIFACTS_DIR":
				return artifactsDir
			case "FONTPUB_ARTIFACTS_BACKEND":
				return backend
			default:
				return os.Getenv(key)
			}
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	r := rebuilder.Rebuilder{Store: store}

	var (
		result rebuilder.Result
		runErr error
	)
	if packageID == "" {
		result, runErr = r.RebuildAll(context.Background())
	} else {
		result, runErr = r.RebuildPackage(context.Background(), packageID)
	}
	if runErr != nil {
		log.Fatal(runErr)
	}

	log.Printf("rebuilt packages=%d versions=%d", result.Packages, result.Versions)
}
