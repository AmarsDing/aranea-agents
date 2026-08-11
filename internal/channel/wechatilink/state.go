package wechatilink

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// stateDir holds per-channel connector state (sync cursor + context tokens).
// Under bin/ per the project root-directory rules (runtime artifacts only).
var stateDir = filepath.Join("bin", "data", "channel-state")

// WithStateDir overrides the state directory (tests). Returns a restore func.
func WithStateDir(dir string, fn func()) {
	prev := stateDir
	stateDir = dir
	defer func() { stateDir = prev }()
	fn()
}

func stateFilePath(channelID string) string {
	return filepath.Join(stateDir, fmt.Sprintf("wechat_ilink-%s.json", channelID))
}

// stateFile is the persisted connector state for one channel.
type stateFile struct {
	GetUpdatesBuf string            `json:"get_updates_buf"`
	ContextTokens map[string]string `json:"context_tokens"`
	LastLoginAt   string            `json:"last_login_at,omitempty"`
	LoginStatus   string            `json:"login_status,omitempty"` // active | expired
}

var stateMu sync.Mutex

// readState loads persisted state; missing file returns a fresh empty state.
func readState(channelID string) (*stateFile, error) {
	b, err := os.ReadFile(stateFilePath(channelID))
	if err != nil {
		if os.IsNotExist(err) {
			return &stateFile{ContextTokens: map[string]string{}}, nil
		}
		return nil, fmt.Errorf("wechat_ilink: read state: %w", err)
	}
	var s stateFile
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("wechat_ilink: parse state: %w", err)
	}
	if s.ContextTokens == nil {
		s.ContextTokens = map[string]string{}
	}
	return &s, nil
}

// writeState persists state atomically (tmp file + rename).
func writeState(channelID string, s *stateFile) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	p := stateFilePath(channelID)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("wechat_ilink: mkdir state dir: %w", err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("wechat_ilink: marshal state: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return fmt.Errorf("wechat_ilink: write state: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("wechat_ilink: rename state: %w", err)
	}
	return nil
}
