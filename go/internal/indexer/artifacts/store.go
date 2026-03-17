package artifacts

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/ma/fontpub/go/internal/protocol"
)

var ErrWriteConflict = errors.New("artifact write conflict")

type Document struct {
	Path string
	ETag string
	Body []byte
}

type Store interface {
	GetVersionedPackageDetail(ctx context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error)
	PutVersionedPackageDetail(ctx context.Context, detail protocol.VersionedPackageDetail, body []byte, etag string) error
	ListPackageVersionedPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, error)
	ListAllVersionedPackageDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error)
	PutPackageVersionsIndex(ctx context.Context, packageID string, index protocol.PackageVersionsIndex, body []byte, etag string) error
	PutLatestAlias(ctx context.Context, packageID string, body []byte, etag string) error
	PutRootIndex(ctx context.Context, index protocol.RootIndex, body []byte, etag string) error
	GetDocument(ctx context.Context, path string) (Document, bool, error)
}

type MemoryStore struct {
	mu           sync.Mutex
	versioned    map[string]protocol.VersionedPackageDetail
	documents    map[string]Document
	failNextPath map[string]int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		versioned:    map[string]protocol.VersionedPackageDetail{},
		documents:    map[string]Document{},
		failNextPath: map[string]int{},
	}
}

func (s *MemoryStore) FailNextWrite(path string, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNextPath[path] = count
}

func (s *MemoryStore) GetVersionedPackageDetail(_ context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	detail, ok := s.versioned[versionedKey(packageID, versionKey)]
	return detail, ok, nil
}

func (s *MemoryStore) PutVersionedPackageDetail(_ context.Context, detail protocol.VersionedPackageDetail, body []byte, etag string) error {
	path := VersionedPackageDetailPath(detail.PackageID, detail.VersionKey)
	if err := s.maybeFail(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.versioned[versionedKey(detail.PackageID, detail.VersionKey)] = detail
	s.documents[path] = Document{Path: path, ETag: etag, Body: append([]byte(nil), body...)}
	return nil
}

func (s *MemoryStore) ListPackageVersionedPackageDetails(_ context.Context, packageID string) ([]protocol.VersionedPackageDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]protocol.VersionedPackageDetail, 0)
	prefix := packageID + "@"
	for key, detail := range s.versioned {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			out = append(out, detail)
		}
	}
	return out, nil
}

func (s *MemoryStore) ListAllVersionedPackageDetails(_ context.Context) ([]protocol.VersionedPackageDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]protocol.VersionedPackageDetail, 0, len(s.versioned))
	for _, detail := range s.versioned {
		out = append(out, detail)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PackageID == out[j].PackageID {
			return out[i].VersionKey < out[j].VersionKey
		}
		return out[i].PackageID < out[j].PackageID
	})
	return out, nil
}

func (s *MemoryStore) PutPackageVersionsIndex(_ context.Context, packageID string, _ protocol.PackageVersionsIndex, body []byte, etag string) error {
	path := PackageVersionsIndexPath(packageID)
	if err := s.maybeFail(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[path] = Document{Path: path, ETag: etag, Body: append([]byte(nil), body...)}
	return nil
}

func (s *MemoryStore) PutLatestAlias(_ context.Context, packageID string, body []byte, etag string) error {
	path := LatestAliasPath(packageID)
	if err := s.maybeFail(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[path] = Document{Path: path, ETag: etag, Body: append([]byte(nil), body...)}
	return nil
}

func (s *MemoryStore) PutRootIndex(_ context.Context, _ protocol.RootIndex, body []byte, etag string) error {
	path := RootIndexPath()
	if err := s.maybeFail(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.documents[path] = Document{Path: path, ETag: etag, Body: append([]byte(nil), body...)}
	return nil
}

func (s *MemoryStore) GetDocument(_ context.Context, path string) (Document, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, ok := s.documents[path]
	return doc, ok, nil
}

func (s *MemoryStore) maybeFail(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if remaining := s.failNextPath[path]; remaining > 0 {
		s.failNextPath[path] = remaining - 1
		return fmt.Errorf("%w: %s", ErrWriteConflict, path)
	}
	return nil
}

func versionedKey(packageID, versionKey string) string {
	return packageID + "@" + versionKey
}

func RootIndexPath() string {
	return "/v1/index.json"
}

func PackageVersionsIndexPath(packageID string) string {
	return "/v1/packages/" + packageID + "/index.json"
}

func LatestAliasPath(packageID string) string {
	return "/v1/packages/" + packageID + ".json"
}

func VersionedPackageDetailPath(packageID, versionKey string) string {
	return "/v1/packages/" + packageID + "/versions/" + versionKey + ".json"
}
