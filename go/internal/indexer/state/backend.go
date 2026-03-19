package state

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type BackendConfig struct {
	Backend  string
	StateDir string
}

type EnvStoreOptions struct {
	DefaultBackend string
	Getenv         func(string) string
}

func LoadBackendConfig(getenv func(string) string) BackendConfig {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	return BackendConfig{
		Backend:  strings.ToLower(strings.TrimSpace(getenv("FONTPUB_STATE_BACKEND"))),
		StateDir: strings.TrimSpace(getenv("FONTPUB_STATE_DIR")),
	}
}

func NewStoreFromEnv(_ context.Context, opts EnvStoreOptions) (Store, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := LoadBackendConfig(getenv)
	backend := cfg.Backend
	if backend == "" {
		backend = opts.DefaultBackend
	}
	switch backend {
	case "memory":
		return NewMemoryStore(), nil
	case "file":
		if cfg.StateDir == "" {
			return nil, fmt.Errorf("FONTPUB_STATE_DIR is required when FONTPUB_STATE_BACKEND=file")
		}
		return NewFileStore(filepath.Join(cfg.StateDir, "state.json")), nil
	default:
		return nil, fmt.Errorf("unsupported state backend: %s", backend)
	}
}
