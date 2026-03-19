package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func main() {
	var (
		outputDir    string
		repository   string
		repositoryID string
		sha          string
		ref          string
		workflowRef  string
		workflowSHA  string
		eventName    string
		audience     string
		issuer       string
		kid          string
	)
	flag.StringVar(&outputDir, "output-dir", "", "directory to write jwks.json and token.txt")
	flag.StringVar(&repository, "repository", "", "repository claim in owner/repo form")
	flag.StringVar(&repositoryID, "repository-id", "", "repository_id claim")
	flag.StringVar(&sha, "sha", "", "git commit sha")
	flag.StringVar(&ref, "ref", "", "git ref")
	flag.StringVar(&workflowRef, "workflow-ref", "", "workflow_ref claim")
	flag.StringVar(&workflowSHA, "workflow-sha", "", "workflow_sha claim")
	flag.StringVar(&eventName, "event-name", "push", "event_name claim")
	flag.StringVar(&audience, "audience", "https://fontpub.org", "OIDC audience")
	flag.StringVar(&issuer, "issuer", "https://token.actions.githubusercontent.com", "OIDC issuer")
	flag.StringVar(&kid, "kid", "fontpub-dev-kid", "JWT kid")
	flag.Parse()

	must(outputDir != "", "missing --output-dir")
	must(repository != "", "missing --repository")
	must(repositoryID != "", "missing --repository-id")
	must(sha != "", "missing --sha")
	must(ref != "", "missing --ref")
	must(workflowRef != "", "missing --workflow-ref")
	must(workflowSHA != "", "missing --workflow-sha")

	owner, _, ok := splitRepository(repository)
	must(ok, "repository must be owner/repo")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	check(err)

	set := jwks{
		Keys: []jwk{{
			Kid: kid,
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
			N:   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(bigEndianInt(key.PublicKey.E)),
		}},
	}
	jwksBody, err := json.Marshal(set)
	check(err)

	now := time.Now().UTC()
	claims := map[string]any{
		"iss":              issuer,
		"aud":              []string{audience},
		"exp":              now.Add(5 * time.Minute).Unix(),
		"iat":              now.Unix(),
		"sub":              fmt.Sprintf("repo:%s:ref:%s", repository, ref),
		"repository":       repository,
		"repository_id":    repositoryID,
		"repository_owner": strings.ToLower(owner),
		"sha":              sha,
		"ref":              ref,
		"workflow_ref":     workflowRef,
		"workflow_sha":     workflowSHA,
		"jti":              fmt.Sprintf("%s-%d", repositoryID, now.UnixNano()),
		"event_name":       eventName,
	}
	token, err := signToken(key, kid, claims)
	check(err)

	check(os.MkdirAll(outputDir, 0o755))
	check(os.WriteFile(filepath.Join(outputDir, "jwks.json"), jwksBody, 0o644))
	check(os.WriteFile(filepath.Join(outputDir, "token.txt"), []byte(token), 0o644))
}

func signToken(key *rsa.PrivateKey, kid string, claims map[string]any) (string, error) {
	headerBytes, err := json.Marshal(map[string]any{
		"alg": "RS256",
		"kid": kid,
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	sum := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func bigEndianInt(v int) []byte {
	if v == 0 {
		return []byte{0}
	}
	buf := make([]byte, 0, 8)
	for v > 0 {
		buf = append([]byte{byte(v & 0xff)}, buf...)
		v >>= 8
	}
	return buf
}

func splitRepository(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '/' {
			if i == 0 || i == len(value)-1 {
				return "", "", false
			}
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}

func must(ok bool, message string) {
	if !ok {
		fmt.Fprintln(os.Stderr, message)
		os.Exit(2)
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
