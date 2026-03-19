package artifacts

import (
	"context"
	"testing"
)

func TestLoadBackendConfig(t *testing.T) {
	cfg := LoadBackendConfig(func(key string) string {
		values := map[string]string{
			"FONTPUB_ARTIFACTS_BACKEND":   "s3",
			"FONTPUB_ARTIFACTS_DIR":       "/tmp/artifacts",
			"FONTPUB_S3_BUCKET":           "fontpub-test",
			"FONTPUB_S3_REGION":           "auto",
			"FONTPUB_S3_ENDPOINT":         "https://example.invalid",
			"FONTPUB_S3_PREFIX":           "/prefix/",
			"FONTPUB_S3_FORCE_PATH_STYLE": "true",
		}
		return values[key]
	})
	if cfg.Backend != "s3" || cfg.ArtifactsDir != "/tmp/artifacts" || cfg.S3Bucket != "fontpub-test" || cfg.S3Region != "auto" || cfg.S3Endpoint != "https://example.invalid" || cfg.S3Prefix != "/prefix/" || !cfg.S3ForcePathStyle {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestNewStoreFromEnvDefaultsToFileWhenArtifactsDirPresent(t *testing.T) {
	store, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			if key == "FONTPUB_ARTIFACTS_DIR" {
				return "/tmp/fontpub-artifacts"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("NewStoreFromEnv: %v", err)
	}
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("expected FileStore, got %T", store)
	}
}

func TestNewStoreFromEnvCreatesS3Store(t *testing.T) {
	var gotCfg BackendConfig
	store, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			values := map[string]string{
				"FONTPUB_ARTIFACTS_BACKEND":   "s3",
				"FONTPUB_S3_BUCKET":           "fontpub-test",
				"FONTPUB_S3_REGION":           "auto",
				"FONTPUB_S3_PREFIX":           "dev",
				"FONTPUB_S3_FORCE_PATH_STYLE": "true",
			}
			return values[key]
		},
		NewS3Client: func(_ context.Context, cfg BackendConfig) (S3Client, error) {
			gotCfg = cfg
			return &fakeS3Client{objects: map[string][]byte{}}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewStoreFromEnv: %v", err)
	}
	s3Store, ok := store.(*S3Store)
	if !ok {
		t.Fatalf("expected S3Store, got %T", store)
	}
	if s3Store.bucket != "fontpub-test" || s3Store.prefix != "dev" {
		t.Fatalf("unexpected store config: bucket=%q prefix=%q", s3Store.bucket, s3Store.prefix)
	}
	if gotCfg.S3Region != "auto" || !gotCfg.S3ForcePathStyle {
		t.Fatalf("unexpected s3 config: %+v", gotCfg)
	}
}

func TestNewStoreFromEnvRequiresS3Settings(t *testing.T) {
	_, err := NewStoreFromEnv(context.Background(), EnvStoreOptions{
		DefaultBackend: "memory",
		Getenv: func(key string) string {
			if key == "FONTPUB_ARTIFACTS_BACKEND" {
				return "s3"
			}
			return ""
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
