package githubraw

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type RoutingFetcher struct {
	LocalRepos map[string]string
	Remote     Fetcher
}

func (f RoutingFetcher) Fetch(ctx context.Context, target string, maxBytes int64) (Result, error) {
	repoKey, repoRoot, objectPath, sha, ok := f.resolveLocalTarget(target)
	if ok {
		return fetchLocalGitObject(ctx, repoKey, repoRoot, sha, objectPath, maxBytes)
	}
	if f.Remote == nil {
		return Result{}, ErrFetchFailed
	}
	return f.Remote.Fetch(ctx, target, maxBytes)
}

func (f RoutingFetcher) resolveLocalTarget(target string) (string, string, string, string, bool) {
	if len(f.LocalRepos) == 0 {
		return "", "", "", "", false
	}
	repoKey, sha, objectPath, err := parseRawGitHubTarget(target)
	if err != nil {
		return "", "", "", "", false
	}
	repoRoot, ok := f.LocalRepos[repoKey]
	if !ok {
		return "", "", "", "", false
	}
	return repoKey, repoRoot, objectPath, sha, true
}

func ParseLocalRepoMap(raw string) (map[string]string, error) {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		repo, root, found := strings.Cut(entry, "=")
		if !found {
			return nil, fmt.Errorf("invalid local repo map entry: %s", entry)
		}
		repo = strings.ToLower(strings.TrimSpace(repo))
		root = strings.TrimSpace(root)
		if _, _, err := splitRepository(repo); err != nil {
			return nil, fmt.Errorf("invalid mapped repository: %s", repo)
		}
		if root == "" {
			return nil, fmt.Errorf("missing local repo path for %s", repo)
		}
		out[repo] = root
	}
	return out, nil
}

func parseRawGitHubTarget(target string) (repoKey, sha, objectPath string, err error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", "", "", err
	}
	if parsed.Scheme != "https" || parsed.Host != "raw.githubusercontent.com" {
		return "", "", "", fmt.Errorf("not a GitHub Raw URL")
	}
	segments := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(segments) < 4 {
		return "", "", "", fmt.Errorf("invalid GitHub Raw path")
	}
	owner, err := url.PathUnescape(segments[0])
	if err != nil {
		return "", "", "", err
	}
	repo, err := url.PathUnescape(segments[1])
	if err != nil {
		return "", "", "", err
	}
	repoKey = strings.ToLower(owner + "/" + repo)
	sha, err = url.PathUnescape(segments[2])
	if err != nil {
		return "", "", "", err
	}
	decoded := make([]string, 0, len(segments)-3)
	for _, segment := range segments[3:] {
		part, err := url.PathUnescape(segment)
		if err != nil {
			return "", "", "", err
		}
		decoded = append(decoded, part)
	}
	objectPath = strings.Join(decoded, "/")
	if objectPath == "" {
		return "", "", "", fmt.Errorf("missing object path")
	}
	return repoKey, sha, objectPath, nil
}

func fetchLocalGitObject(ctx context.Context, repoKey, repoRoot, sha, objectPath string, maxBytes int64) (Result, error) {
	if err := verifyLocalCommit(ctx, repoRoot, sha); err != nil {
		return Result{}, ErrNotFound
	}
	spec := fmt.Sprintf("%s:%s", sha, objectPath)
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "show", spec)
	body, err := cmd.Output()
	if err != nil {
		return Result{}, ErrNotFound
	}
	if int64(len(body)) > maxBytes {
		return Result{}, ErrTooLarge
	}
	_ = repoKey
	return Result{Body: body, Size: int64(len(body))}, nil
}

func verifyLocalCommit(ctx context.Context, repoRoot, sha string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "cat-file", "-e", sha+"^{commit}")
	return cmd.Run()
}
