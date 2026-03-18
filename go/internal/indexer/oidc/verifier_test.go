package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"
)

func TestVerifierValidToken(t *testing.T) {
	privateKey, jwks := generateKeySet(t, "kid-1")
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	token := signToken(t, privateKey, "kid-1", map[string]any{
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
	verifier := Verifier{
		Provider: StaticProvider{Set: jwks},
		Issuer:   "https://token.actions.githubusercontent.com",
		Audience: "https://fontpub.org",
		Now:      func() time.Time { return now },
	}
	claims, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Repository != "example/family" || claims.RepositoryID != "123456789" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestVerifierRejectsInvalidToken(t *testing.T) {
	privateKey, jwks := generateKeySet(t, "kid-1")
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	baseClaims := map[string]any{
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
	}
	tests := []struct {
		name   string
		claims map[string]any
		mutate func(string) string
	}{
		{name: "bad audience", claims: mergeClaims(baseClaims, map[string]any{"aud": []string{"other"}})},
		{name: "expired", claims: mergeClaims(baseClaims, map[string]any{"exp": now.Add(-1 * time.Minute).Unix()})},
		{name: "bad signature", claims: baseClaims, mutate: func(token string) string { return token + "x" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := signToken(t, privateKey, "kid-1", tc.claims)
			if tc.mutate != nil {
				token = tc.mutate(token)
			}
			verifier := Verifier{
				Provider: StaticProvider{Set: jwks},
				Issuer:   "https://token.actions.githubusercontent.com",
				Audience: "https://fontpub.org",
				Now:      func() time.Time { return now },
			}
			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatalf("expected verification failure")
			}
		})
	}
}

func generateKeySet(t *testing.T, kid string) (*rsa.PrivateKey, JWKS) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	jwks := JWKS{
		Keys: []JWK{
			{
				Kid: kid,
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				E:   encodeExponent(key.PublicKey.E),
			},
		},
	}
	return key, jwks
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	headerBytes, _ := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	})
	payloadBytes, _ := json.Marshal(claims)
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func encodeExponent(exp int) string {
	return base64.RawURLEncoding.EncodeToString(new(big.Int).SetInt64(int64(exp)).Bytes())
}

func mergeClaims(base map[string]any, override map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func TestFindRSAKeyMissingKid(t *testing.T) {
	_, jwks := generateKeySet(t, "kid-1")
	if _, err := findRSAKey(jwks, "missing"); err == nil || !strings.Contains(err.Error(), "kid not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
