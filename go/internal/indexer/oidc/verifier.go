package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type Provider interface {
	KeySet(ctx context.Context) (JWKS, error)
}

type RefreshingProvider interface {
	Provider
	RefreshKeySet(ctx context.Context) (JWKS, error)
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

type JWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type StaticProvider struct {
	Set JWKS
	Err error
}

func (p StaticProvider) KeySet(context.Context) (JWKS, error) {
	if p.Err != nil {
		return JWKS{}, p.Err
	}
	return p.Set, nil
}

type Verifier struct {
	Provider Provider
	Issuer   string
	Audience string
	Now      func() time.Time
}

func (v Verifier) Verify(ctx context.Context, rawToken string) (protocol.OIDCClaims, error) {
	if v.Provider == nil {
		return protocol.OIDCClaims{}, fmt.Errorf("jwks provider is not configured")
	}
	now := v.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return protocol.OIDCClaims{}, fmt.Errorf("token must have 3 parts")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid header encoding: %w", err)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid payload encoding: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid signature encoding: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid header json: %w", err)
	}
	if header.Alg != "RS256" {
		return protocol.OIDCClaims{}, fmt.Errorf("unsupported alg: %s", header.Alg)
	}
	if header.Kid == "" {
		return protocol.OIDCClaims{}, fmt.Errorf("missing kid")
	}

	var rawClaims map[string]any
	if err := json.Unmarshal(payloadBytes, &rawClaims); err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid payload json: %w", err)
	}
	if iss, ok := rawClaims["iss"].(string); !ok || iss != v.Issuer {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid issuer")
	}
	if !audContains(rawClaims["aud"], v.Audience) {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid audience")
	}
	if !validateTimeClaims(rawClaims, now()) {
		return protocol.OIDCClaims{}, fmt.Errorf("invalid token time")
	}

	set, err := v.Provider.KeySet(ctx)
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	key, err := findRSAKey(set, header.Kid)
	if err != nil {
		var missingKidErr *MissingKIDError
		if errors.As(err, &missingKidErr) {
			if refresher, ok := v.Provider.(RefreshingProvider); ok {
				refreshed, refreshErr := refresher.RefreshKeySet(ctx)
				if refreshErr == nil {
					set = refreshed
					key, err = findRSAKey(set, header.Kid)
				}
			}
		}
	}
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature); err != nil {
		return protocol.OIDCClaims{}, fmt.Errorf("signature verification failed: %w", err)
	}

	claims, err := decodeProtocolClaims(rawClaims)
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	return claims, nil
}

func audContains(raw any, audience string) bool {
	switch v := raw.(type) {
	case string:
		return v == audience
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == audience {
				return true
			}
		}
	}
	return false
}

func validateTimeClaims(raw map[string]any, now time.Time) bool {
	exp, ok := numericClaim(raw["exp"])
	if !ok {
		return false
	}
	iat, ok := numericClaim(raw["iat"])
	if !ok {
		return false
	}
	nowUnix := now.Unix()
	return exp > nowUnix && iat >= nowUnix-600 && iat <= nowUnix+600
}

func numericClaim(raw any) (int64, bool) {
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

type MissingKIDError struct {
	KID string
}

func (e *MissingKIDError) Error() string {
	return fmt.Sprintf("kid not found: %s", e.KID)
}

func findRSAKey(set JWKS, kid string) (*rsa.PublicKey, error) {
	for _, key := range set.Keys {
		if key.Kid != kid {
			continue
		}
		if key.Kty != "RSA" {
			return nil, fmt.Errorf("unsupported key type: %s", key.Kty)
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			return nil, fmt.Errorf("invalid jwk modulus: %w", err)
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			return nil, fmt.Errorf("invalid jwk exponent: %w", err)
		}
		if len(eBytes) > 8 {
			return nil, errors.New("invalid jwk exponent length")
		}
		var exponent uint64
		padded := append(make([]byte, 8-len(eBytes)), eBytes...)
		exponent = binary.BigEndian.Uint64(padded)
		return &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(exponent),
		}, nil
	}
	return nil, &MissingKIDError{KID: kid}
}

func decodeProtocolClaims(raw map[string]any) (protocol.OIDCClaims, error) {
	getString := func(name string) (string, error) {
		value, ok := raw[name].(string)
		if !ok || value == "" {
			return "", fmt.Errorf("missing string claim %s", name)
		}
		return value, nil
	}
	sub, err := getString("sub")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	repository, err := getString("repository")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	repositoryID, err := getString("repository_id")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	repositoryOwner, err := getString("repository_owner")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	sha, err := getString("sha")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	ref, err := getString("ref")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	workflowRef, err := getString("workflow_ref")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	workflowSHA, err := getString("workflow_sha")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	jti, err := getString("jti")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	eventName, err := getString("event_name")
	if err != nil {
		return protocol.OIDCClaims{}, err
	}
	return protocol.OIDCClaims{
		Sub:             sub,
		Repository:      repository,
		RepositoryID:    repositoryID,
		RepositoryOwner: repositoryOwner,
		SHA:             sha,
		Ref:             ref,
		WorkflowRef:     workflowRef,
		WorkflowSHA:     workflowSHA,
		JTI:             jti,
		EventName:       eventName,
	}, nil
}
