package modelregistry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePolicyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	p := DefaultPolicy()
	p.SyncIntervalHours = 12
	if err := st.SavePolicy(p); err != nil {
		t.Fatal(err)
	}
	got, err := st.LoadPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncIntervalHours != 12 {
		t.Fatalf("interval: got %d", got.SyncIntervalHours)
	}
}

func TestStoreDirectoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := NewStore(dir)
	cat := Directory{
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Env:  []string{"ANTHROPIC_API_KEY"},
			Npm:  "@ai-sdk/anthropic",
			Doc:  "https://docs.anthropic.com",
			Models: map[string]Model{
				"claude-sonnet-4-6": {
					ID:          "claude-sonnet-4-6",
					Name:        "Claude Sonnet 4.6",
					Attachment:  true,
					Reasoning:   true,
					ToolCall:    true,
					OpenWeights: false,
					ReleaseDate: "2026-02-17",
					LastUpdated: "2026-03-13",
					Limit:       ModelLimit{Context: 1_000_000, Output: 64_000},
					Modalities:  Modalities{Input: []string{"text"}, Output: []string{"text"}},
					Cost:        &ModelCost{Input: 3, Output: 15},
				},
			},
		},
	}
	meta := Meta{SyncedAt: "2026-05-25T00:00:00Z", SourceURL: "https://models.dev/api.json"}
	if err := st.SaveDirectory(cat, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), currentFile)); err != nil {
		t.Fatal(err)
	}
	loaded, gotMeta, err := st.LoadDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || gotMeta.SourceURL != meta.SourceURL {
		t.Fatalf("reload mismatch")
	}
}

func TestMigrateProviderCode(t *testing.T) {
	if got := MigrateProviderCode("aliyun-qwen"); got != "alibaba-cn" {
		t.Fatalf("got %q", got)
	}
}
