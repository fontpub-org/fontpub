package state

import (
	"context"
	"testing"
)

func TestNewStoreFromEnvDefaultsToMemory(t *testing.T) {
	store, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
	})
	if err != nil {
		t.Fatalf("NewStoreFromEnv: %v", err)
	}
	if _, ok := store.(*MemoryStore); !ok {
		t.Fatalf("expected MemoryStore, got %T", store)
	}
}

func TestNewStoreFromEnvUsesFileStore(t *testing.T) {
	stateDir := t.TempDir()
	store, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			switch key {
			case "FONTPUB_STATE_BACKEND":
				return "file"
			case "FONTPUB_STATE_DIR":
				return stateDir
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatalf("NewStoreFromEnv: %v", err)
	}
	fileStore, ok := store.(*FileStore)
	if !ok {
		t.Fatalf("expected FileStore, got %T", store)
	}
	if fileStore.path != stateDir+"/state.json" {
		t.Fatalf("unexpected state path: %s", fileStore.path)
	}
}

func TestNewStoreFromEnvRejectsMissingFileDir(t *testing.T) {
	_, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			if key == "FONTPUB_STATE_BACKEND" {
				return "file"
			}
			return ""
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadBackendConfigDefaultsAndRejectsUnsupportedBackend(t *testing.T) {
	cfg := LoadBackendConfig(nil)
	if cfg.Backend != "" || cfg.StateDir != "" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	_, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			if key == "FONTPUB_STATE_BACKEND" {
				return "bogus"
			}
			return ""
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
