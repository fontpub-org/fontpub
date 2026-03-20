package rebuilder

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
)

type Config struct {
	PackageID string
	Store     artifacts.Store
}

type App struct {
	Rebuilder Rebuilder
}

func Main() {
	options, err := ParseOptions(os.Args[1:], os.Getenv, flag.ExitOnError)
	if err != nil {
		log.Fatal(err)
	}
	store, err := buildStore(context.Background(), options, os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	result, err := App{Rebuilder: Rebuilder{Store: store}}.Run(context.Background(), options.PackageID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("rebuilt packages=%d versions=%d", result.Packages, result.Versions)
}

func LoadConfig(ctx context.Context, args []string, getenv func(string) string) (Config, error) {
	options, err := ParseOptions(args, getenv, flag.ContinueOnError)
	if err != nil {
		return Config{}, err
	}
	store, err := buildStore(ctx, options, getenv)
	if err != nil {
		return Config{}, err
	}
	return Config{
		PackageID: options.PackageID,
		Store:     store,
	}, nil
}

func ParseOptions(args []string, getenv func(string) string, errorHandling flag.ErrorHandling) (Options, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	fs := flag.NewFlagSet("fontpub-rebuilder", errorHandling)
	if errorHandling == flag.ContinueOnError {
		fs.SetOutput(ioDiscard{})
	}

	options := Options{}
	fs.StringVar(&options.PackageID, "package-id", "", "rebuild only a single package")
	fs.StringVar(&options.ArtifactsDir, "artifacts-dir", getenv("FONTPUB_ARTIFACTS_DIR"), "directory containing published Fontpub artifacts")
	fs.StringVar(&options.Backend, "artifacts-backend", getenv("FONTPUB_ARTIFACTS_BACKEND"), "artifact backend: file, memory, or s3")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	return options, nil
}

func (a App) Run(ctx context.Context, packageID string) (Result, error) {
	return a.rebuilder().run(ctx, packageID)
}

func (a App) rebuilder() Rebuilder {
	if a.Rebuilder.Store != nil {
		return a.Rebuilder
	}
	return Rebuilder{}
}

func buildStore(ctx context.Context, options Options, getenv func(string) string) (artifacts.Store, error) {
	return artifacts.NewStoreFromEnv(ctx, artifacts.EnvStoreOptions{
		DefaultBackend: "file",
		Getenv: func(key string) string {
			switch key {
			case "FONTPUB_ARTIFACTS_DIR":
				return options.ArtifactsDir
			case "FONTPUB_ARTIFACTS_BACKEND":
				return options.Backend
			default:
				if getenv == nil {
					return ""
				}
				return getenv(key)
			}
		},
	})
}

type Options struct {
	PackageID    string
	ArtifactsDir string
	Backend      string
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
