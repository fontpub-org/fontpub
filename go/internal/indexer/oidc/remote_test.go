package oidc

import (
	"context"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRemoteJWKSProviderCachesWithinTTL(t *testing.T) {
	_, set := generateKeySet(t, "kid-1")
	var calls atomic.Int32
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, err := json.Marshal(set)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return jsonResponse(http.StatusOK, body), nil
	})

	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	provider := &RemoteJWKSProvider{
		URL:    "https://example.test/jwks",
		Client: client,
		TTL: 5 * time.Minute,
		Now: func() time.Time { return now },
	}
	if _, err := provider.KeySet(context.Background()); err != nil {
		t.Fatalf("KeySet: %v", err)
	}
	if _, err := provider.KeySet(context.Background()); err != nil {
		t.Fatalf("KeySet cached: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 fetch, got %d", got)
	}
}

func TestRemoteJWKSProviderRefreshesAfterTTL(t *testing.T) {
	_, set := generateKeySet(t, "kid-1")
	var calls atomic.Int32
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, err := json.Marshal(set)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return jsonResponse(http.StatusOK, body), nil
	})

	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	provider := &RemoteJWKSProvider{
		URL:    "https://example.test/jwks",
		Client: client,
		TTL: time.Minute,
		Now: func() time.Time { return now },
	}
	if _, err := provider.KeySet(context.Background()); err != nil {
		t.Fatalf("KeySet: %v", err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := provider.KeySet(context.Background()); err != nil {
		t.Fatalf("KeySet refresh: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 fetches, got %d", got)
	}
}

func TestRemoteJWKSProviderUsesCachedSetOnFetchFailure(t *testing.T) {
	_, set := generateKeySet(t, "kid-1")
	fail := false
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if fail {
			return jsonResponse(http.StatusBadGateway, []byte(`{"error":"boom"}`)), nil
		}
		body, err := json.Marshal(set)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		return jsonResponse(http.StatusOK, body), nil
	})

	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	provider := &RemoteJWKSProvider{
		URL:    "https://example.test/jwks",
		Client: client,
		TTL: time.Minute,
		Now: func() time.Time { return now },
	}
	first, err := provider.KeySet(context.Background())
	if err != nil {
		t.Fatalf("KeySet: %v", err)
	}
	fail = true
	now = now.Add(2 * time.Minute)
	second, err := provider.KeySet(context.Background())
	if err != nil {
		t.Fatalf("KeySet stale fallback: %v", err)
	}
	if len(first.Keys) != len(second.Keys) || first.Keys[0].Kid != second.Keys[0].Kid {
		t.Fatalf("unexpected fallback keyset: %+v", second)
	}
}

func TestVerifierRefreshesOnMissingKID(t *testing.T) {
	oldKey, oldSet := generateKeySet(t, "kid-old")
	newKey, newSet := generateKeySet(t, "kid-new")
	_ = oldKey
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	token := signToken(t, newKey, "kid-new", map[string]any{
		"iss":              "https://token.actions.githubusercontent.com",
		"aud":              []string{"https://fontpub.org"},
		"exp":              now.Add(5 * time.Minute).Unix(),
		"iat":              now.Unix(),
		"sub":              "repo:example/family:ref:refs/tags/v1.2.3",
		"repository":       "example/family",
		"repository_id":    "123456789",
		"repository_owner": "example",
		"sha":              "0123456789abcdef0123456789abcdef01234567",
		"ref":              "refs/tags/v1.2.3",
		"workflow_ref":     "example/family/.github/workflows/fontpub.yml@refs/heads/main",
		"workflow_sha":     "89abcdef0123456789abcdef0123456789abcdef",
		"jti":              "jwt-id-1",
		"event_name":       "push",
	})

	var calls atomic.Int32
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if calls.Load() == 1 {
			body, err := json.Marshal(oldSet)
			if err != nil {
				t.Fatalf("Marshal old set: %v", err)
			}
			return jsonResponse(http.StatusOK, body), nil
		}
		body, err := json.Marshal(newSet)
		if err != nil {
			t.Fatalf("Marshal new set: %v", err)
		}
		return jsonResponse(http.StatusOK, body), nil
	})

	provider := &RemoteJWKSProvider{
		URL:    "https://example.test/jwks",
		Client: client,
		TTL:    10 * time.Minute,
		Now:    func() time.Time { return now },
	}
	verifier := Verifier{
		Provider: provider,
		Issuer:   "https://token.actions.githubusercontent.com",
		Audience: "https://fontpub.org",
		Now:      func() time.Time { return now },
	}
	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Repository != "example/family" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 fetches, got %d", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
