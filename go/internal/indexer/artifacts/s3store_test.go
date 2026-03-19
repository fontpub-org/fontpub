package artifacts

import (
	"bytes"
	"context"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/fontpub-org/fontpub/go/internal/indexer/derive"
	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type fakeS3Client struct {
	objects  map[string][]byte
	pageSize int
}

func (f *fakeS3Client) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[aws.ToString(in.Key)] = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3Client) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, fakeS3APIError{code: "NoSuchKey"}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeS3Client) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	keys := make([]string, 0, len(f.objects))
	prefix := aws.ToString(in.Prefix)
	for key := range f.objects {
		if len(prefix) == 0 || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	start := 0
	if token := aws.ToString(in.ContinuationToken); token != "" {
		for i, key := range keys {
			if key > token {
				start = i
				break
			}
			if i == len(keys)-1 {
				start = len(keys)
			}
		}
	}
	pageSize := f.pageSize
	if pageSize <= 0 {
		pageSize = len(keys)
	}
	end := start + pageSize
	if end > len(keys) {
		end = len(keys)
	}
	contents := make([]types.Object, 0, end-start)
	for _, key := range keys[start:end] {
		keyCopy := key
		contents = append(contents, types.Object{Key: &keyCopy})
	}
	out := &s3.ListObjectsV2Output{Contents: contents}
	if end < len(keys) {
		out.IsTruncated = aws.Bool(true)
		next := keys[end-1]
		out.NextContinuationToken = &next
	}
	return out, nil
}

type fakeS3APIError struct{ code string }

func (e fakeS3APIError) Error() string          { return e.code }
func (e fakeS3APIError) ErrorCode() string      { return e.code }
func (e fakeS3APIError) ErrorMessage() string   { return e.code }
func (e fakeS3APIError) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

func TestS3StoreRoundTrip(t *testing.T) {
	client := &fakeS3Client{objects: map[string][]byte{}}
	store := NewS3Store(client, "fontpub-test", S3StoreOptions{Prefix: "dev"})
	detail := protocol.VersionedPackageDetail{
		SchemaVersion: "1",
		PackageID:     "0xtype/gamut",
		DisplayName:   "Zx Gamut",
		Author:        "0xType",
		License:       "OFL-1.1",
		Version:       "1.002",
		VersionKey:    "1.002",
		PublishedAt:   "2026-03-19T00:00:00Z",
		GitHub:        protocol.GitHubRef{Owner: "0xtype", Repo: "gamut", SHA: "0123456789abcdef0123456789abcdef01234567"},
		ManifestURL:   "https://raw.githubusercontent.com/0xtype/gamut/0123456789abcdef0123456789abcdef01234567/fontpub.json",
		Assets:        []protocol.VersionedAsset{{Path: "fonts/static/ZxGamut-Bold.otf", URL: "https://raw.githubusercontent.com/0xtype/gamut/0123456789abcdef0123456789abcdef01234567/fonts/static/ZxGamut-Bold.otf", SHA256: "abc", Format: "otf", Style: "normal", Weight: 700, SizeBytes: 11}},
	}
	body, err := protocol.MarshalCanonical(detail)
	if err != nil {
		t.Fatalf("MarshalCanonical: %v", err)
	}
	if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
		t.Fatalf("PutVersionedPackageDetail: %v", err)
	}
	got, ok, err := store.GetVersionedPackageDetail(context.Background(), "0xtype/gamut", "1.002")
	if err != nil || !ok {
		t.Fatalf("GetVersionedPackageDetail: ok=%v err=%v", ok, err)
	}
	if got.PackageID != detail.PackageID || got.VersionKey != detail.VersionKey {
		t.Fatalf("unexpected detail: %+v", got)
	}
	doc, ok, err := store.GetDocument(context.Background(), VersionedPackageDetailPath("0xtype/gamut", "1.002"))
	if err != nil || !ok {
		t.Fatalf("GetDocument: ok=%v err=%v", ok, err)
	}
	if doc.ETag != derive.ComputeETag(body) {
		t.Fatalf("unexpected etag: %s", doc.ETag)
	}
}

func TestS3StoreListAllVersionedPackageDetails(t *testing.T) {
	client := &fakeS3Client{objects: map[string][]byte{}, pageSize: 1}
	store := NewS3Store(client, "fontpub-test", S3StoreOptions{Prefix: "dev"})
	for _, detail := range []protocol.VersionedPackageDetail{
		{SchemaVersion: "1", PackageID: "0xtype/gamut", DisplayName: "Zx Gamut", Author: "0xType", License: "OFL-1.1", Version: "1.002", VersionKey: "1.002", PublishedAt: "2026-03-19T00:00:00Z"},
		{SchemaVersion: "1", PackageID: "example/family", DisplayName: "Example Sans", Author: "Example", License: "OFL-1.1", Version: "1.2.3", VersionKey: "1.2.3", PublishedAt: "2026-03-19T00:00:01Z"},
	} {
		body, err := protocol.MarshalCanonical(detail)
		if err != nil {
			t.Fatalf("MarshalCanonical: %v", err)
		}
		if err := store.PutVersionedPackageDetail(context.Background(), detail, body, derive.ComputeETag(body)); err != nil {
			t.Fatalf("PutVersionedPackageDetail: %v", err)
		}
	}
	list, err := store.ListAllVersionedPackageDetails(context.Background())
	if err != nil {
		t.Fatalf("ListAllVersionedPackageDetails: %v", err)
	}
	if len(list) != 2 || list[0].PackageID != "0xtype/gamut" || list[1].PackageID != "example/family" {
		t.Fatalf("unexpected list: %+v", list)
	}
}
