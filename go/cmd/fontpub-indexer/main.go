package main

import (
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
	artifactStore := buildArtifactStoreFromEnv()

	verifier := buildVerifierFromEnv()
	server := updateapi.Server{
		Verifier: verifier,
		Processor: updateapi.PublishingProcessor{
			ValidationProcessor: updateapi.ValidationProcessor{
				State:   state.NewMemoryStore(),
				Fetcher: githubraw.HTTPFetcher{Client: http.DefaultClient},
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

func buildArtifactStoreFromEnv() artifacts.Store {
	if root := os.Getenv("FONTPUB_ARTIFACTS_DIR"); root != "" {
		return artifacts.NewFileStore(root)
	}
	return artifacts.NewMemoryStore()
}
