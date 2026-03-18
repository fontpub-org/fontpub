package state

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrOwnershipMismatch = errors.New("ownership mismatch")
	ErrReplayDetected    = errors.New("replay detected")
)

type Store interface {
	CheckAndReserveJTI(ctx context.Context, jti string) error
	CheckOrBindPackage(ctx context.Context, packageID, repositoryID string) error
}

type MemoryStore struct {
	mu         sync.Mutex
	usedJTI    map[string]struct{}
	packageIDs map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usedJTI:    map[string]struct{}{},
		packageIDs: map[string]string{},
	}
}

func (s *MemoryStore) CheckAndReserveJTI(_ context.Context, jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.usedJTI[jti]; exists {
		return ErrReplayDetected
	}
	s.usedJTI[jti] = struct{}{}
	return nil
}

func (s *MemoryStore) CheckOrBindPackage(_ context.Context, packageID, repositoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.packageIDs[packageID]; exists {
		if existing != repositoryID {
			return ErrOwnershipMismatch
		}
		return nil
	}
	s.packageIDs[packageID] = repositoryID
	return nil
}
