package state

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const lockRetryInterval = 10 * time.Millisecond

type fileState struct {
	SchemaVersion   string            `json:"schema_version"`
	PackageBindings map[string]string `json:"package_bindings"`
	UsedJTI         []string          `json:"used_jti"`
}

type FileStore struct {
	mu       sync.Mutex
	path     string
	lockPath string
}

func NewFileStore(path string) *FileStore {
	return &FileStore{
		path:     path,
		lockPath: path + ".lock",
	}
}

func (s *FileStore) CheckAndReserveJTI(ctx context.Context, jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := s.acquireLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	current, err := s.load()
	if err != nil {
		return err
	}
	if current.PackageBindings == nil {
		current.PackageBindings = map[string]string{}
	}
	used := make(map[string]struct{}, len(current.UsedJTI))
	for _, existing := range current.UsedJTI {
		used[existing] = struct{}{}
	}
	if _, exists := used[jti]; exists {
		return ErrReplayDetected
	}
	used[jti] = struct{}{}
	current.UsedJTI = sortedKeys(used)
	return s.save(current)
}

func (s *FileStore) CheckOrBindPackage(ctx context.Context, packageID, repositoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	unlock, err := s.acquireLock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	current, err := s.load()
	if err != nil {
		return err
	}
	if current.PackageBindings == nil {
		current.PackageBindings = map[string]string{}
	}
	if existing, exists := current.PackageBindings[packageID]; exists {
		if existing != repositoryID {
			return ErrOwnershipMismatch
		}
		return nil
	}
	current.PackageBindings[packageID] = repositoryID
	return s.save(current)
}

func (s *FileStore) acquireLock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, err
	}
	for {
		lock, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = lock.Close()
			return func() { _ = os.Remove(s.lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(lockRetryInterval):
			}
		} else {
			time.Sleep(lockRetryInterval)
		}
	}
}

func (s *FileStore) load() (fileState, error) {
	body, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileState{
				SchemaVersion:   "1",
				PackageBindings: map[string]string{},
				UsedJTI:         []string{},
			}, nil
		}
		return fileState{}, err
	}
	var current fileState
	if err := json.Unmarshal(body, &current); err != nil {
		return fileState{}, err
	}
	if current.SchemaVersion == "" {
		current.SchemaVersion = "1"
	}
	if current.PackageBindings == nil {
		current.PackageBindings = map[string]string{}
	}
	if current.UsedJTI == nil {
		current.UsedJTI = []string{}
	}
	return current, nil
}

func (s *FileStore) save(current fileState) error {
	current.SchemaVersion = "1"
	if current.PackageBindings == nil {
		current.PackageBindings = map[string]string{}
	}
	sort.Strings(current.UsedJTI)

	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".fontpub-state-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}
