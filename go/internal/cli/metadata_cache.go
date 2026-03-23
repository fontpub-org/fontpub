package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/fontpub-org/fontpub/go/internal/protocol"
)

type metadataCacheFile struct {
	SchemaVersion string             `json:"schema_version"`
	RootIndex     *cachedJSONPayload `json:"root_index,omitempty"`
}

type cachedJSONPayload struct {
	ETag string          `json:"etag"`
	Body json.RawMessage `json:"body"`
}

type MetadataCacheStore struct {
	Path string
}

func (s MetadataCacheStore) LoadRootIndex() (protocol.RootIndex, string, bool) {
	body, err := os.ReadFile(s.Path)
	if err != nil {
		return protocol.RootIndex{}, "", false
	}
	var cache metadataCacheFile
	if err := json.Unmarshal(body, &cache); err != nil {
		return protocol.RootIndex{}, "", false
	}
	if cache.SchemaVersion != "1" || cache.RootIndex == nil || len(cache.RootIndex.Body) == 0 {
		return protocol.RootIndex{}, "", false
	}
	var root protocol.RootIndex
	if err := json.Unmarshal(cache.RootIndex.Body, &root); err != nil {
		return protocol.RootIndex{}, "", false
	}
	return root, cache.RootIndex.ETag, true
}

func (s MetadataCacheStore) SaveRootIndex(body []byte, etag string) error {
	if s.Path == "" || etag == "" || len(body) == 0 {
		return nil
	}
	cache := metadataCacheFile{
		SchemaVersion: "1",
		RootIndex: &cachedJSONPayload{
			ETag: etag,
			Body: append(json.RawMessage(nil), body...),
		},
	}
	encoded, err := protocol.MarshalCanonical(cache)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".fontpub-cache-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(encoded, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func removeMetadataCache(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
