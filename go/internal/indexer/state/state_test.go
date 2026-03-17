package state

import (
	"context"
	"testing"
)

func TestMemoryStoreReplay(t *testing.T) {
	store := NewMemoryStore()
	if err := store.CheckAndReserveJTI(context.Background(), "jti-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.CheckAndReserveJTI(context.Background(), "jti-1"); err != ErrReplayDetected {
		t.Fatalf("got %v want ErrReplayDetected", err)
	}
}

func TestMemoryStoreOwnership(t *testing.T) {
	store := NewMemoryStore()
	if err := store.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("unexpected rebind error: %v", err)
	}
	if err := store.CheckOrBindPackage(context.Background(), "example/family", "456"); err != ErrOwnershipMismatch {
		t.Fatalf("got %v want ErrOwnershipMismatch", err)
	}
}
