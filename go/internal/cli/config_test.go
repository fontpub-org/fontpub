package cli

import (
	"path/filepath"
	"testing"
)

func TestConfigLockfilePath(t *testing.T) {
	cfg := Config{StateDir: filepath.Join("/tmp", "fontpub-state")}
	if got := cfg.LockfilePath(); got != filepath.Join("/tmp", "fontpub-state", "fontpub.lock") {
		t.Fatalf("LockfilePath() = %s", got)
	}
}
