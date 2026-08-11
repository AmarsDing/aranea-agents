package wechatilink

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestStateFileRoundtrip(t *testing.T) {
	WithStateDir(t.TempDir(), func() {
		s := &stateFile{
			GetUpdatesBuf: "buf_v1",
			ContextTokens: map[string]string{"u1": "tk1"},
			LoginStatus:   "active",
		}
		if err := writeState("ch-1", s); err != nil {
			t.Fatal(err)
		}
		loaded, err := readState("ch-1")
		if err != nil {
			t.Fatal(err)
		}
		if loaded.GetUpdatesBuf != "buf_v1" {
			t.Errorf("buf want buf_v1, got %s", loaded.GetUpdatesBuf)
		}
		if loaded.ContextTokens["u1"] != "tk1" {
			t.Error("context token mismatch")
		}
		if loaded.LoginStatus != "active" {
			t.Errorf("status want active, got %s", loaded.LoginStatus)
		}
	})
}

func TestReadStateNotExist(t *testing.T) {
	WithStateDir(t.TempDir(), func() {
		s, err := readState("ch-missing")
		if err != nil {
			t.Fatal(err)
		}
		if s.GetUpdatesBuf != "" {
			t.Error("fresh state should have empty buf")
		}
		if s.ContextTokens == nil {
			t.Error("ContextTokens should be initialized")
		}
	})
}

func TestWriteStateAtomicNoTmpLeft(t *testing.T) {
	dir := t.TempDir()
	WithStateDir(dir, func() {
		if err := writeState("ch-2", &stateFile{GetUpdatesBuf: "x"}); err != nil {
			t.Fatal(err)
		}
	})
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("tmp file should be renamed, found %s", e.Name())
		}
	}
}

func TestStateFileCorrupted(t *testing.T) {
	dir := t.TempDir()
	WithStateDir(dir, func() {
		p := stateFilePath("ch-bad")
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{invalid json"), 0o640); err != nil {
			t.Fatal(err)
		}
		if _, err := readState("ch-bad"); err == nil {
			t.Error("corrupted state file should return error")
		}
	})
}

func TestCachedContextToken(t *testing.T) {
	WithStateDir(t.TempDir(), func() {
		if got := CachedContextToken("ch-none", "u1"); got != "" {
			t.Errorf("missing state file should yield empty token, got %q", got)
		}
		if err := writeState("ch-3", &stateFile{ContextTokens: map[string]string{"u1": "tk-cached"}}); err != nil {
			t.Fatal(err)
		}
		if got := CachedContextToken("ch-3", "u1"); got != "tk-cached" {
			t.Errorf("want tk-cached, got %q", got)
		}
		if got := CachedContextToken("ch-3", "u2"); got != "" {
			t.Errorf("unknown user should yield empty token, got %q", got)
		}
		if got := CachedContextToken("", "u1"); got != "" {
			t.Errorf("empty channel id should yield empty token, got %q", got)
		}
	})
}

var _ = json.Marshal // keep import used if helpers change
var _ = sync.Mutex{}
