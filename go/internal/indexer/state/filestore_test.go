package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestFileStoreAcquireLockRespectsContextCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(store.lockPath, []byte("held"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := store.acquireLock(ctx); err == nil {
		t.Fatalf("expected context cancellation")
	}
}

func TestFileStoreLoadInitializesMissingCollectionsAndRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileStore(path)
	current, err := store.load()
	if err != nil {
		t.Fatalf("load missing file: %v", err)
	}
	if current.SchemaVersion != "1" || current.PackageBindings == nil || current.UsedJTI == nil {
		t.Fatalf("unexpected current state: %+v", current)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := store.load(); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}
