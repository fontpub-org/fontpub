package artifacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type S3Client interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type S3StoreOptions struct {
	Prefix         string
	ForcePathStyle bool
}

type S3Store struct {
	client S3Client
	bucket string
	prefix string
}

func NewS3Store(client S3Client, bucket string, opts S3StoreOptions) *S3Store {
	return &S3Store{
		client: client,
		bucket: bucket,
		prefix: strings.Trim(strings.TrimSpace(opts.Prefix), "/"),
	}
}

func (s *S3Store) GetVersionedPackageDetail(ctx context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error) {
	return s.readVersionedPackageDetail(ctx, VersionedPackageDetailPath(packageID, versionKey))
}

func (s *S3Store) PutVersionedPackageDetail(ctx context.Context, detail protocol.VersionedPackageDetail, body []byte, _ string) error {
	return s.writeDocument(ctx, VersionedPackageDetailPath(detail.PackageID, detail.VersionKey), body)
}

func (s *S3Store) ListPackageVersionedPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, error) {
	artifactPaths, err := s.listArtifactPaths(ctx, path.Join("v1", "packages", packageID, "versions")+"/")
	if err != nil {
		return nil, err
	}
	out := make([]protocol.VersionedPackageDetail, 0, len(artifactPaths))
	for _, artifactPath := range artifactPaths {
		detail, ok, err := s.readVersionedPackageDetail(ctx, artifactPath)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, detail)
		}
	}
	return out, nil
}

func (s *S3Store) ListAllVersionedPackageDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error) {
	artifactPaths, err := s.listArtifactPaths(ctx, path.Join("v1", "packages")+"/")
	if err != nil {
		return nil, err
	}
	out := make([]protocol.VersionedPackageDetail, 0, len(artifactPaths))
	for _, artifactPath := range artifactPaths {
		if path.Base(path.Dir(artifactPath)) != "versions" || !strings.HasSuffix(artifactPath, ".json") {
			continue
		}
		detail, ok, err := s.readVersionedPackageDetail(ctx, artifactPath)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, detail)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PackageID == out[j].PackageID {
			return out[i].VersionKey < out[j].VersionKey
		}
		return out[i].PackageID < out[j].PackageID
	})
	return out, nil
}

func (s *S3Store) PutPackageVersionsIndex(ctx context.Context, packageID string, _ protocol.PackageVersionsIndex, body []byte, _ string) error {
	return s.writeDocument(ctx, PackageVersionsIndexPath(packageID), body)
}

func (s *S3Store) PutLatestAlias(ctx context.Context, packageID string, body []byte, _ string) error {
	return s.writeDocument(ctx, LatestAliasPath(packageID), body)
}

func (s *S3Store) PutRootIndex(ctx context.Context, _ protocol.RootIndex, body []byte, _ string) error {
	return s.writeDocument(ctx, RootIndexPath(), body)
}

func (s *S3Store) GetDocument(ctx context.Context, artifactPath string) (Document, bool, error) {
	key := s.keyForPath(artifactPath)
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return Document{}, false, nil
		}
		return Document{}, false, err
	}
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		return Document{}, false, err
	}
	return Document{
		Path: artifactPath,
		ETag: derive.ComputeETag(body),
		Body: body,
	}, true, nil
}

func (s *S3Store) readVersionedPackageDetail(ctx context.Context, artifactPath string) (protocol.VersionedPackageDetail, bool, error) {
	doc, ok, err := s.GetDocument(ctx, artifactPath)
	if err != nil || !ok {
		return protocol.VersionedPackageDetail{}, ok, err
	}
	var detail protocol.VersionedPackageDetail
	if err := json.Unmarshal(doc.Body, &detail); err != nil {
		return protocol.VersionedPackageDetail{}, false, err
	}
	return detail, true, nil
}

func (s *S3Store) writeDocument(ctx context.Context, artifactPath string, body []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s.keyForPath(artifactPath)),
		Body:        bytes.NewReader(body),
		ContentType: aws.String("application/json"),
	})
	return err
}

func (s *S3Store) listArtifactPaths(ctx context.Context, logicalPrefix string) ([]string, error) {
	prefix := s.keyForPath("/" + strings.TrimPrefix(logicalPrefix, "/"))
	var (
		out               []string
		continuationToken *string
	)
	for {
		result, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, err
		}
		for _, object := range result.Contents {
			if object.Key == nil {
				continue
			}
			artifactPath, ok := s.pathForKey(*object.Key)
			if ok {
				out = append(out, artifactPath)
			}
		}
		if !aws.ToBool(result.IsTruncated) || result.NextContinuationToken == nil || *result.NextContinuationToken == "" {
			break
		}
		continuationToken = result.NextContinuationToken
	}
	sort.Strings(out)
	return out, nil
}

func (s *S3Store) keyForPath(artifactPath string) string {
	trimmed := strings.TrimPrefix(artifactPath, "/")
	if s.prefix == "" {
		return trimmed
	}
	return s.prefix + "/" + trimmed
}

func (s *S3Store) pathForKey(key string) (string, bool) {
	trimmed := key
	if s.prefix != "" {
		prefix := s.prefix + "/"
		if !strings.HasPrefix(trimmed, prefix) {
			return "", false
		}
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}
	return "/" + strings.TrimPrefix(trimmed, "/"), true
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "NoSuchKey", "NotFound":
		return true
	default:
		return false
	}
}
