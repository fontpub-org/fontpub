package githubraw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ma/fontpub/go/internal/protocol"
)

var (
	ErrNotFound    = errors.New("upstream not found")
	ErrFetchFailed = errors.New("upstream fetch failed")
	ErrTooLarge    = errors.New("upstream payload too large")
)

type Result struct {
	Body []byte
	Size int64
}

type Fetcher interface {
	Fetch(ctx context.Context, url string, maxBytes int64) (Result, error)
}

type HTTPFetcher struct {
	Client *http.Client
}

func BuildManifestURL(repository, sha string) (string, error) {
	owner, repo, err := splitRepository(repository)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/fontpub.json", owner, repo, sha), nil
}

func BuildAssetURL(repository, sha, path string) (string, error) {
	if err := protocol.ValidateAssetPath(path); err != nil {
		return "", err
	}
	owner, repo, err := splitRepository(repository)
	if err != nil {
		return "", err
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", owner, repo, sha, strings.Join(segments, "/")), nil
}

func (f HTTPFetcher) Fetch(ctx context.Context, target string, maxBytes int64) (Result, error) {
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, ErrFetchFailed
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Result{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, ErrFetchFailed
	}

	limited := io.LimitReader(resp.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Result{}, ErrFetchFailed
	}
	if int64(len(body)) > maxBytes {
		return Result{}, ErrTooLarge
	}
	return Result{Body: body, Size: int64(len(body))}, nil
}

func splitRepository(repository string) (string, string, error) {
	repository = strings.ToLower(repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository")
	}
	return parts[0], parts[1], nil
}
