package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/oidc"
	"github.com/fontpub-org/fontpub/go/internal/indexer/updateapi"
)

func TestBuildFetcherFromEnvDefaultsToHTTP(t *testing.T) {
	t.Setenv("FONTPUB_DEV_LOCAL_REPO_MAP", "")
	fetcher := buildFetcherFromEnv()
	if _, ok := fetcher.(githubraw.HTTPFetcher); !ok {
		t.Fatalf("expected HTTPFetcher, got %T", fetcher)
	}
}

func TestBuildFetcherFromEnvUsesRoutingFetcher(t *testing.T) {
	t.Setenv("FONTPUB_DEV_LOCAL_REPO_MAP", "0xtype/gamut=/tmp/gamut")
	fetcher := buildFetcherFromEnv()
	routing, ok := fetcher.(githubraw.RoutingFetcher)
	if !ok {
		t.Fatalf("expected RoutingFetcher, got %T", fetcher)
	}
	if got := routing.LocalRepos["0xtype/gamut"]; got != "/tmp/gamut" {
		t.Fatalf("unexpected local repo mapping: %q", got)
	}
}

func TestBuildFetcherFromEnvFallsBackOnInvalidMap(t *testing.T) {
	t.Setenv("FONTPUB_DEV_LOCAL_REPO_MAP", "invalid")
	fetcher := buildFetcherFromEnv()
	if _, ok := fetcher.(githubraw.HTTPFetcher); !ok {
		t.Fatalf("expected HTTPFetcher on invalid map, got %T", fetcher)
	}
}

func TestBuildArtifactStoreFromEnvDefaultsToMemory(t *testing.T) {
	t.Setenv("FONTPUB_ARTIFACTS_BACKEND", "")
	t.Setenv("FONTPUB_ARTIFACTS_DIR", "")
	store, err := buildArtifactStoreFromEnv()
	if err != nil {
		t.Fatalf("buildArtifactStoreFromEnv: %v", err)
	}
	if _, ok := store.(*artifacts.MemoryStore); !ok {
		t.Fatalf("expected MemoryStore, got %T", store)
	}
}

func TestBuildArtifactStoreFromEnvUsesFileStore(t *testing.T) {
	t.Setenv("FONTPUB_ARTIFACTS_BACKEND", "file")
	t.Setenv("FONTPUB_ARTIFACTS_DIR", t.TempDir())
	store, err := buildArtifactStoreFromEnv()
	if err != nil {
		t.Fatalf("buildArtifactStoreFromEnv: %v", err)
	}
	if _, ok := store.(*artifacts.FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", store)
	}
}

func TestBuildVerifierFromEnvUsesStaticProvider(t *testing.T) {
	t.Setenv("FONTPUB_GITHUB_JWKS_JSON", `{"keys":[]}`)
	verifier := buildVerifierFromEnv()
	oidcVerifier, ok := verifier.(oidc.Verifier)
	if !ok {
		t.Fatalf("expected oidc.Verifier, got %T", verifier)
	}
	if _, ok := oidcVerifier.Provider.(oidc.StaticProvider); !ok {
		t.Fatalf("expected StaticProvider, got %T", oidcVerifier.Provider)
	}
}

func TestBuildVerifierFromEnvUsesRemoteProviderByDefault(t *testing.T) {
	t.Setenv("FONTPUB_GITHUB_JWKS_JSON", "")
	t.Setenv("FONTPUB_GITHUB_JWKS_URL", "")
	verifier := buildVerifierFromEnv()
	oidcVerifier, ok := verifier.(oidc.Verifier)
	if !ok {
		t.Fatalf("expected oidc.Verifier, got %T", verifier)
	}
	provider, ok := oidcVerifier.Provider.(*oidc.RemoteJWKSProvider)
	if !ok {
		t.Fatalf("expected RemoteJWKSProvider, got %T", oidcVerifier.Provider)
	}
	if provider.URL != "https://token.actions.githubusercontent.com/.well-known/jwks" {
		t.Fatalf("unexpected URL: %s", provider.URL)
	}
	if provider.TTL != 10*time.Minute {
		t.Fatalf("unexpected TTL: %s", provider.TTL)
	}
	client, ok := provider.Client.(*http.Client)
	if !ok {
		t.Fatalf("unexpected client: %T", provider.Client)
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("unexpected timeout: %s", client.Timeout)
	}
}

func TestBuildVerifierFromEnvRejectsBadDurations(t *testing.T) {
	t.Setenv("FONTPUB_GITHUB_JWKS_JSON", "")
	t.Setenv("FONTPUB_GITHUB_JWKS_CACHE_TTL", "nope")
	verifier := buildVerifierFromEnv()
	if _, ok := verifier.(updateapi.StaticVerifier); !ok {
		t.Fatalf("expected StaticVerifier error, got %T", verifier)
	}
}
