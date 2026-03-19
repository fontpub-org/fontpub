package state

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileStorePersistsReplayProtectionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)
	if err := store.CheckAndReserveJTI(context.Background(), "jti-1"); err != nil {
		t.Fatalf("CheckAndReserveJTI: %v", err)
	}

	restarted := NewFileStore(path)
	if err := restarted.CheckAndReserveJTI(context.Background(), "jti-1"); err != ErrReplayDetected {
		t.Fatalf("got %v want ErrReplayDetected", err)
	}
}

func TestFileStorePersistsOwnershipAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)
	if err := store.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("CheckOrBindPackage: %v", err)
	}

	restarted := NewFileStore(path)
	if err := restarted.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("restarted CheckOrBindPackage: %v", err)
	}
	if err := restarted.CheckOrBindPackage(context.Background(), "example/family", "456"); err != ErrOwnershipMismatch {
		t.Fatalf("got %v want ErrOwnershipMismatch", err)
	}
}

func TestFileStoreConcurrentInstancesDoNotCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := NewFileStore(path)
	second := NewFileStore(path)
	if err := first.CheckAndReserveJTI(context.Background(), "jti-1"); err != nil {
		t.Fatalf("first CheckAndReserveJTI: %v", err)
	}
	if err := second.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("second CheckOrBindPackage: %v", err)
	}

	restarted := NewFileStore(path)
	if err := restarted.CheckAndReserveJTI(context.Background(), "jti-1"); err != ErrReplayDetected {
		t.Fatalf("got %v want ErrReplayDetected", err)
	}
	if err := restarted.CheckOrBindPackage(context.Background(), "example/family", "123"); err != nil {
		t.Fatalf("restarted CheckOrBindPackage: %v", err)
	}
}
