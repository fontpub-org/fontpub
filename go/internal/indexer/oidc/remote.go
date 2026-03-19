package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type RemoteJWKSProvider struct {
	URL    string
	Client HTTPDoer
	TTL    time.Duration
	Now    func() time.Time

	mu       sync.Mutex
	cached   JWKS
	cachedAt time.Time
	hasCache bool
}

func (p *RemoteJWKSProvider) KeySet(ctx context.Context) (JWKS, error) {
	return p.keySet(ctx, false)
}

func (p *RemoteJWKSProvider) RefreshKeySet(ctx context.Context) (JWKS, error) {
	return p.keySet(ctx, true)
}

func (p *RemoteJWKSProvider) keySet(ctx context.Context, force bool) (JWKS, error) {
	p.mu.Lock()
	client := p.httpClient()
	now := p.now()
	url := p.URL
	if url == "" {
		p.mu.Unlock()
		return JWKS{}, fmt.Errorf("jwks url is not configured")
	}
	if !force && p.hasCache && p.cacheValid(now) {
		cached := p.cached
		p.mu.Unlock()
		return cached, nil
	}
	cached := p.cached
	hasCache := p.hasCache
	p.mu.Unlock()

	set, err := p.fetch(ctx, client, url)
	if err != nil {
		if hasCache {
			return cached, nil
		}
		return JWKS{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.cached = set
	p.cachedAt = now
	p.hasCache = true
	return set, nil
}

func (p *RemoteJWKSProvider) fetch(ctx context.Context, client HTTPDoer, url string) (JWKS, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return JWKS{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return JWKS{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return JWKS{}, fmt.Errorf("jwks fetch failed: status %d", resp.StatusCode)
	}
	var set JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return JWKS{}, err
	}
	return set, nil
}

func (p *RemoteJWKSProvider) cacheValid(now time.Time) bool {
	ttl := p.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return now.Sub(p.cachedAt) < ttl
}

func (p *RemoteJWKSProvider) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *RemoteJWKSProvider) httpClient() HTTPDoer {
	if p.Client != nil {
		return p.Client
	}
	return http.DefaultClient
}
