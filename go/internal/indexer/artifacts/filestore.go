package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ma/fontpub/go/internal/indexer/derive"
	"github.com/ma/fontpub/go/internal/protocol"
)

type FileStore struct {
	root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{root: root}
}

func (s *FileStore) GetVersionedPackageDetail(ctx context.Context, packageID, versionKey string) (protocol.VersionedPackageDetail, bool, error) {
	return s.readVersionedPackageDetail(ctx, VersionedPackageDetailPath(packageID, versionKey))
}

func (s *FileStore) PutVersionedPackageDetail(_ context.Context, detail protocol.VersionedPackageDetail, body []byte, _ string) error {
	return s.writeDocument(VersionedPackageDetailPath(detail.PackageID, detail.VersionKey), body)
}

func (s *FileStore) ListPackageVersionedPackageDetails(ctx context.Context, packageID string) ([]protocol.VersionedPackageDetail, error) {
	dir := s.absolutePath("/v1/packages/" + packageID + "/versions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]protocol.VersionedPackageDetail, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		artifactPath := "/v1/packages/" + packageID + "/versions/" + entry.Name()
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

func (s *FileStore) ListAllVersionedPackageDetails(ctx context.Context) ([]protocol.VersionedPackageDetail, error) {
	root := s.absolutePath("/v1/packages")
	out := make([]protocol.VersionedPackageDetail, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || filepath.Base(filepath.Dir(path)) != "versions" || !strings.HasSuffix(path, ".json") {
			return nil
		}
		artifactPath := "/" + filepath.ToSlash(strings.TrimPrefix(path, s.root))
		detail, ok, err := s.readVersionedPackageDetail(ctx, artifactPath)
		if err != nil {
			return err
		}
		if ok {
			out = append(out, detail)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PackageID == out[j].PackageID {
			return out[i].VersionKey < out[j].VersionKey
		}
		return out[i].PackageID < out[j].PackageID
	})
	return out, nil
}

func (s *FileStore) PutPackageVersionsIndex(_ context.Context, packageID string, _ protocol.PackageVersionsIndex, body []byte, _ string) error {
	return s.writeDocument(PackageVersionsIndexPath(packageID), body)
}

func (s *FileStore) PutLatestAlias(_ context.Context, packageID string, body []byte, _ string) error {
	return s.writeDocument(LatestAliasPath(packageID), body)
}

func (s *FileStore) PutRootIndex(_ context.Context, _ protocol.RootIndex, body []byte, _ string) error {
	return s.writeDocument(RootIndexPath(), body)
}

func (s *FileStore) GetDocument(_ context.Context, path string) (Document, bool, error) {
	body, err := os.ReadFile(s.absolutePath(path))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Document{}, false, nil
		}
		return Document{}, false, err
	}
	return Document{
		Path: path,
		ETag: derive.ComputeETag(body),
		Body: body,
	}, true, nil
}

func (s *FileStore) readVersionedPackageDetail(_ context.Context, artifactPath string) (protocol.VersionedPackageDetail, bool, error) {
	body, err := os.ReadFile(s.absolutePath(artifactPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return protocol.VersionedPackageDetail{}, false, nil
		}
		return protocol.VersionedPackageDetail{}, false, err
	}
	var detail protocol.VersionedPackageDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return protocol.VersionedPackageDetail{}, false, err
	}
	return detail, true, nil
}

func (s *FileStore) writeDocument(path string, body []byte) error {
	abs := s.absolutePath(path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".fontpub-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (s *FileStore) absolutePath(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	return filepath.Join(s.root, filepath.FromSlash(trimmed))
}
