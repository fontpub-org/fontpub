package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/oidc"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/indexer/updateapi"
)

type Config struct {
	Addr          string
	ArtifactStore artifacts.Store
	StateStore    state.Store
	Verifier      updateapi.Verifier
	Fetcher       githubraw.Fetcher
	Clock         updateapi.Clock
}

type App struct {
	Config Config
}

func Main() {
	if err := Run(context.Background(), os.Getenv, http.ListenAndServe); err != nil {
		log.Fatal(err)
	}
}

func Run(ctx context.Context, getenv func(string) string, listenAndServe func(string, http.Handler) error) error {
	cfg, err := LoadConfig(ctx, getenv)
	if err != nil {
		return err
	}
	return App{Config: cfg}.ListenAndServe(listenAndServe)
}

func LoadConfig(ctx context.Context, getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	addr := getenv("FONTPUB_INDEXER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	artifactStore, err := buildArtifactStoreFromEnv(ctx, getenv)
	if err != nil {
		return Config{}, err
	}
	stateStore, err := buildStateStoreFromEnv(ctx, getenv)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:          addr,
		ArtifactStore: artifactStore,
		StateStore:    stateStore,
		Verifier:      buildVerifierFromEnv(getenv),
		Fetcher:       buildFetcherFromEnv(getenv),
		Clock:         updateapi.RealClock{},
	}, nil
}

func (a App) Handler() http.Handler {
	server := updateapi.Server{
		Verifier: a.Config.Verifier,
		Processor: updateapi.PublishingProcessor{
			ValidationProcessor: updateapi.ValidationProcessor{
				State:   a.Config.StateStore,
				Fetcher: a.Config.Fetcher,
			},
			ArtifactStore: a.Config.ArtifactStore,
			Clock:         a.clock(),
		},
	}
	return server.Handler()
}

func (a App) ListenAndServe(listenAndServe func(string, http.Handler) error) error {
	if listenAndServe == nil {
		listenAndServe = http.ListenAndServe
	}
	return listenAndServe(a.Config.Addr, a.Handler())
}

func (a App) clock() updateapi.Clock {
	if a.Config.Clock != nil {
		return a.Config.Clock
	}
	return updateapi.RealClock{}
}

func buildVerifierFromEnv(getenv func(string) string) updateapi.Verifier {
	jwksJSON := getenv("FONTPUB_GITHUB_JWKS_JSON")
	if jwksJSON != "" {
		var set oidc.JWKS
		if err := json.Unmarshal([]byte(jwksJSON), &set); err != nil {
			return updateapi.StaticVerifier{Err: err}
		}
		return oidc.Verifier{
			Provider: oidc.StaticProvider{Set: set},
			Issuer:   "https://token.actions.githubusercontent.com",
			Audience: "https://fontpub.org",
		}
	}

	timeout, err := parseDurationEnv(getenv, "FONTPUB_GITHUB_JWKS_TIMEOUT", 5*time.Second)
	if err != nil {
		return updateapi.StaticVerifier{Err: err}
	}
	ttl, err := parseDurationEnv(getenv, "FONTPUB_GITHUB_JWKS_CACHE_TTL", 10*time.Minute)
	if err != nil {
		return updateapi.StaticVerifier{Err: err}
	}

	jwksURL := getenv("FONTPUB_GITHUB_JWKS_URL")
	if jwksURL == "" {
		jwksURL = "https://token.actions.githubusercontent.com/.well-known/jwks"
	}
	return oidc.Verifier{
		Provider: &oidc.RemoteJWKSProvider{
			URL:    jwksURL,
			Client: &http.Client{Timeout: timeout},
			TTL:    ttl,
		},
		Issuer:   "https://token.actions.githubusercontent.com",
		Audience: "https://fontpub.org",
	}
}

func parseDurationEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	value := getenv(key)
	if value == "" {
		return fallback, nil
	}
	return time.ParseDuration(value)
}

func buildArtifactStoreFromEnv(ctx context.Context, getenv func(string) string) (artifacts.Store, error) {
	return artifacts.NewStoreFromEnv(ctx, artifacts.EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv:         getenv,
	})
}

func buildStateStoreFromEnv(ctx context.Context, getenv func(string) string) (state.Store, error) {
	return state.NewStoreFromEnv(ctx, state.EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv:         getenv,
	})
}

func buildFetcherFromEnv(getenv func(string) string) githubraw.Fetcher {
	remote := githubraw.HTTPFetcher{Client: http.DefaultClient}
	localRepos, err := githubraw.ParseLocalRepoMap(getenv("FONTPUB_DEV_LOCAL_REPO_MAP"))
	if err != nil || len(localRepos) == 0 {
		return remote
	}
	return githubraw.RoutingFetcher{
		LocalRepos: localRepos,
		Remote:     remote,
	}
}
