package main

import (
	"testing"

	"github.com/fontpub-org/fontpub/go/internal/indexer/artifacts"
	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
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
