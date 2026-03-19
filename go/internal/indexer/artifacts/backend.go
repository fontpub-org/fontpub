package artifacts

import (
	"context"
	"fmt"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type BackendConfig struct {
	Backend          string
	ArtifactsDir     string
	S3Bucket         string
	S3Region         string
	S3Endpoint       string
	S3Prefix         string
	S3ForcePathStyle bool
}

type EnvStoreOptions struct {
	DefaultBackend string
	Getenv         func(string) string
	NewS3Client    func(context.Context, BackendConfig) (S3Client, error)
}

func LoadBackendConfig(getenv func(string) string) BackendConfig {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	return BackendConfig{
		Backend:          strings.ToLower(strings.TrimSpace(getenv("FONTPUB_ARTIFACTS_BACKEND"))),
		ArtifactsDir:     strings.TrimSpace(getenv("FONTPUB_ARTIFACTS_DIR")),
		S3Bucket:         strings.TrimSpace(getenv("FONTPUB_S3_BUCKET")),
		S3Region:         strings.TrimSpace(getenv("FONTPUB_S3_REGION")),
		S3Endpoint:       strings.TrimSpace(getenv("FONTPUB_S3_ENDPOINT")),
		S3Prefix:         strings.TrimSpace(getenv("FONTPUB_S3_PREFIX")),
		S3ForcePathStyle: parseBool(getenv("FONTPUB_S3_FORCE_PATH_STYLE")),
	}
}

func NewStoreFromEnv(ctx context.Context, opts EnvStoreOptions) (Store, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := LoadBackendConfig(getenv)
	backend := cfg.Backend
	if backend == "" {
		if cfg.ArtifactsDir != "" {
			backend = "file"
		} else {
			backend = opts.DefaultBackend
		}
	}
	switch backend {
	case "memory":
		return NewMemoryStore(), nil
	case "file":
		if cfg.ArtifactsDir == "" {
			return nil, fmt.Errorf("FONTPUB_ARTIFACTS_DIR is required when FONTPUB_ARTIFACTS_BACKEND=file")
		}
		return NewFileStore(cfg.ArtifactsDir), nil
	case "s3":
		if cfg.S3Bucket == "" {
			return nil, fmt.Errorf("FONTPUB_S3_BUCKET is required when FONTPUB_ARTIFACTS_BACKEND=s3")
		}
		if cfg.S3Region == "" {
			return nil, fmt.Errorf("FONTPUB_S3_REGION is required when FONTPUB_ARTIFACTS_BACKEND=s3")
		}
		newS3Client := opts.NewS3Client
		if newS3Client == nil {
			newS3Client = newAWSClientFromConfig
		}
		client, err := newS3Client(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return NewS3Store(client, cfg.S3Bucket, S3StoreOptions{
			Prefix:         cfg.S3Prefix,
			ForcePathStyle: cfg.S3ForcePathStyle,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported artifact backend: %s", backend)
	}
}

func newAWSClientFromConfig(ctx context.Context, cfg BackendConfig) (S3Client, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3Region),
	}
	if cfg.S3Endpoint != "" {
		loadOptions = append(loadOptions, awsconfig.WithBaseEndpoint(cfg.S3Endpoint))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.S3ForcePathStyle
	}), nil
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
