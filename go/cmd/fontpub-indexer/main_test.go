package main

import (
	"testing"

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
