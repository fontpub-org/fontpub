package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
	"github.com/fontpub-org/fontpub/go/internal/indexer/oidc"
	"github.com/fontpub-org/fontpub/go/internal/indexer/state"
	"github.com/fontpub-org/fontpub/go/internal/indexer/updateapi"
)

func TestBuildFetcherFromEnvDefaultsToHTTP(t *testing.T) {
	fetcher := buildFetcherFromEnv(func(string) string { return "" })
	if _, ok := fetcher.(githubraw.HTTPFetcher); !ok {
		t.Fatalf("expected HTTPFetcher, got %T", fetcher)
	}
}

func TestBuildFetcherFromEnvUsesRoutingFetcher(t *testing.T) {
	fetcher := buildFetcherFromEnv(func(key string) string {
		if key == "FONTPUB_DEV_LOCAL_REPO_MAP" {
			return "0xtype/gamut=/tmp/gamut"
		}
		return ""
	})
	routing, ok := fetcher.(githubraw.RoutingFetcher)
	if !ok {
		t.Fatalf("expected RoutingFetcher, got %T", fetcher)
	}
	if got := routing.LocalRepos["0xtype/gamut"]; got != "/tmp/gamut" {
		t.Fatalf("unexpected local repo mapping: %q", got)
	}
}

func TestBuildFetcherFromEnvFallsBackOnInvalidMap(t *testing.T) {
	fetcher := buildFetcherFromEnv(func(key string) string {
		if key == "FONTPUB_DEV_LOCAL_REPO_MAP" {
			return "invalid"
		}
		return ""
	})
	if _, ok := fetcher.(githubraw.HTTPFetcher); !ok {
		t.Fatalf("expected HTTPFetcher on invalid map, got %T", fetcher)
	}
}

func TestLoadConfigBuildsDefaultStores(t *testing.T) {
	cfg, err := LoadConfig(context.Background(), func(string) string { return "" })
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("unexpected addr: %s", cfg.Addr)
	}
	if _, ok := cfg.ArtifactStore.(*artifacts.MemoryStore); !ok {
		t.Fatalf("expected MemoryStore, got %T", cfg.ArtifactStore)
	}
	if _, ok := cfg.StateStore.(*state.MemoryStore); !ok {
		t.Fatalf("expected MemoryStore, got %T", cfg.StateStore)
	}
}

func TestLoadConfigUsesFileStores(t *testing.T) {
	artifactsDir := t.TempDir()
	stateDir := t.TempDir()
	cfg, err := LoadConfig(context.Background(), func(key string) string {
		switch key {
		case "FONTPUB_ARTIFACTS_BACKEND":
			return "file"
		case "FONTPUB_ARTIFACTS_DIR":
			return artifactsDir
		case "FONTPUB_STATE_BACKEND":
			return "file"
		case "FONTPUB_STATE_DIR":
			return stateDir
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := cfg.ArtifactStore.(*artifacts.FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", cfg.ArtifactStore)
	}
	if _, ok := cfg.StateStore.(*state.FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", cfg.StateStore)
	}
}

func TestBuildVerifierFromEnvUsesStaticProvider(t *testing.T) {
	verifier := buildVerifierFromEnv(func(key string) string {
		if key == "FONTPUB_GITHUB_JWKS_JSON" {
			return `{"keys":[]}`
		}
		return ""
	})
	oidcVerifier, ok := verifier.(oidc.Verifier)
	if !ok {
		t.Fatalf("expected oidc.Verifier, got %T", verifier)
	}
	if _, ok := oidcVerifier.Provider.(oidc.StaticProvider); !ok {
		t.Fatalf("expected StaticProvider, got %T", oidcVerifier.Provider)
	}
}

func TestBuildVerifierFromEnvUsesRemoteProviderByDefault(t *testing.T) {
	verifier := buildVerifierFromEnv(func(string) string { return "" })
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
	verifier := buildVerifierFromEnv(func(key string) string {
		if key == "FONTPUB_GITHUB_JWKS_CACHE_TTL" {
			return "nope"
		}
		return ""
	})
	if _, ok := verifier.(updateapi.StaticVerifier); !ok {
		t.Fatalf("expected StaticVerifier error, got %T", verifier)
	}
}

func TestAppListenAndServeUsesConfiguredAddress(t *testing.T) {
	app := App{Config: Config{
		Addr:          "127.0.0.1:18080",
		ArtifactStore: artifacts.NewMemoryStore(),
		StateStore:    state.NewMemoryStore(),
		Verifier:      updateapi.StaticVerifier{},
		Fetcher:       githubraw.HTTPFetcher{Client: http.DefaultClient},
	}}

	var gotAddr string
	err := app.ListenAndServe(func(addr string, handler http.Handler) error {
		gotAddr = addr
		if handler == nil {
			t.Fatalf("handler is nil")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}
	if gotAddr != "127.0.0.1:18080" {
		t.Fatalf("unexpected addr: %s", gotAddr)
	}
}
