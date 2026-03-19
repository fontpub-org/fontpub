package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/oidc"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/indexer/updateapi"
)

func main() {
	addr := os.Getenv("FONTPUB_INDEXER_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	artifactStore, err := buildArtifactStoreFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	verifier := buildVerifierFromEnv()
	server := updateapi.Server{
		Verifier: verifier,
		Processor: updateapi.PublishingProcessor{
			ValidationProcessor: updateapi.ValidationProcessor{
				State:   state.NewMemoryStore(),
				Fetcher: buildFetcherFromEnv(),
			},
			ArtifactStore: artifactStore,
			Clock:         updateapi.RealClock{},
		},
	}

	log.Fatal(http.ListenAndServe(addr, server.Handler()))
}

func buildVerifierFromEnv() updateapi.Verifier {
	jwksJSON := os.Getenv("FONTPUB_GITHUB_JWKS_JSON")
	if jwksJSON == "" {
		return updateapi.StaticVerifier{Err: os.ErrInvalid}
	}
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

func buildArtifactStoreFromEnv() (artifacts.Store, error) {
	return artifacts.NewStoreFromEnv(context.Background(), artifacts.EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv:         os.Getenv,
	})
}

func buildFetcherFromEnv() githubraw.Fetcher {
	remote := githubraw.HTTPFetcher{Client: http.DefaultClient}
	localRepos, err := githubraw.ParseLocalRepoMap(os.Getenv("FONTPUB_DEV_LOCAL_REPO_MAP"))
	if err != nil || len(localRepos) == 0 {
		return remote
	}
	return githubraw.RoutingFetcher{
		LocalRepos: localRepos,
		Remote:     remote,
	}
}
