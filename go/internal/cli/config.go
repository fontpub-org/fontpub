package cli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fontpub-org/fontpub/go/internal/indexer/githubraw"
)

type Config struct {
	BaseURL              string
	StateDir             string
	DefaultActivationDir string
	HTTPTimeout          time.Duration
	UserAgent            string
	LocalRepoMap         map[string]string
}

func DefaultConfig() Config {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	baseURL := os.Getenv("FONTPUB_BASE_URL")
	if baseURL == "" {
		baseURL = "https://fontpub.org"
	}
	stateDir := os.Getenv("FONTPUB_STATE_DIR")
	if stateDir == "" {
		stateDir = filepath.Join(home, ".fontpub")
	}
	localRepoMap, _ := githubraw.ParseLocalRepoMap(os.Getenv("FONTPUB_DEV_LOCAL_REPO_MAP"))
	return Config{
		BaseURL:              baseURL,
		StateDir:             stateDir,
		DefaultActivationDir: os.Getenv("FONTPUB_ACTIVATION_DIR"),
		HTTPTimeout:          10 * time.Second,
		UserAgent:            "fontpub/dev",
		LocalRepoMap:         localRepoMap,
	}
}

func (c Config) LockfilePath() string {
	return filepath.Join(c.StateDir, "fontpub.lock")
}
