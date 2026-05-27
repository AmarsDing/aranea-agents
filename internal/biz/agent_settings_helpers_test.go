package biz

import (
	"os"
	"testing"
)

// TestDefaultPromptFilesV1LegacyCount verifies the legacy 9-file set when flag is off.
func TestDefaultPromptFilesV1LegacyCount(t *testing.T) {
	t.Setenv("PGO_DEFAULT_FILES_V2", "0")
	files := defaultPromptFiles()
	if len(files) != 9 {
		t.Errorf("legacy set: want 9 files, got %d", len(files))
	}
	names := fileNames(files)
	for _, must := range []string{"SOUL.md", "HEARTBEAT.md", "USER.md"} {
		if !containsStr(names, must) {
			t.Errorf("legacy set: missing %s", must)
		}
	}
}

// TestDefaultPromptFilesV2Count verifies the V2 5-file set when flag is on.
func TestDefaultPromptFilesV2Count(t *testing.T) {
	t.Setenv("PGO_DEFAULT_FILES_V2", "1")
	files := defaultPromptFiles()
	if len(files) != 5 {
		t.Errorf("V2 set: want 5 files, got %d", len(files))
	}
	names := fileNames(files)
	for _, must := range []string{"AGENTS_CORE.md", "AGENTS_TASK.md", "IDENTITY.md", "CAPABILITIES.md", "RULE.md"} {
		if !containsStr(names, must) {
			t.Errorf("V2 set: missing %s", must)
		}
	}
	for _, banned := range []string{"SOUL.md", "HEARTBEAT.md", "USER.md"} {
		if containsStr(names, banned) {
			t.Errorf("V2 set: should not contain %s", banned)
		}
	}
}

// TestFilesForModeComplete verifies complete mode returns all files.
func TestFilesForModeComplete(t *testing.T) {
	files := sampleFiles()
	out := FilesForMode(files, "complete")
	if len(out) != len(files) {
		t.Errorf("complete: want all %d files, got %d", len(files), len(out))
	}
}

// TestFilesForModeNone verifies none mode returns empty slice.
func TestFilesForModeNone(t *testing.T) {
	out := FilesForMode(sampleFiles(), "none")
	if len(out) != 0 {
		t.Errorf("none: want 0 files, got %d", len(out))
	}
}

// TestFilesForModeMinimized verifies minimized mode returns only core files.
func TestFilesForModeMinimized(t *testing.T) {
	out := FilesForMode(sampleFiles(), "minimized")
	names := fileNames(out)
	for _, must := range []string{"AGENTS_CORE.md", "RULE.md"} {
		if !containsStr(names, must) {
			t.Errorf("minimized: missing %s", must)
		}
	}
	for _, banned := range []string{"AGENTS_TASK.md", "IDENTITY.md", "HEARTBEAT.md"} {
		if containsStr(names, banned) {
			t.Errorf("minimized: should not contain %s", banned)
		}
	}
}

// TestFilesForModeTask verifies task mode returns 5 core files, no HEARTBEAT.
func TestFilesForModeTask(t *testing.T) {
	all := append(sampleFiles(), AgentPromptFile{Name: "HEARTBEAT.md", Body: "hb"})
	out := FilesForMode(all, "task")
	names := fileNames(out)
	for _, must := range []string{"AGENTS_CORE.md", "AGENTS_TASK.md", "IDENTITY.md", "CAPABILITIES.md", "RULE.md"} {
		if !containsStr(names, must) {
			t.Errorf("task: missing %s", must)
		}
	}
	if containsStr(names, "HEARTBEAT.md") {
		t.Error("task: HEARTBEAT.md must be excluded (PGO-1-BIZ-02)")
	}
}

// TestFilesForModeEmptyMeansComplete verifies empty string behaves like complete.
func TestFilesForModeEmptyMeansComplete(t *testing.T) {
	files := sampleFiles()
	out := FilesForMode(files, "")
	if len(out) != len(files) {
		t.Errorf("empty mode: want all %d files, got %d", len(files), len(out))
	}
}

// TestPgoFlagParsing verifies pgoDefaultFilesV2 truth-table.
func TestPgoFlagParsing(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"yes", true},
		{"TRUE", true},
		{"YES", true},
		{"0", false},
		{"false", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run("env="+tc.env, func(t *testing.T) {
			os.Setenv("PGO_DEFAULT_FILES_V2", tc.env)
			defer os.Unsetenv("PGO_DEFAULT_FILES_V2")
			got := pgoDefaultFilesV2()
			if got != tc.want {
				t.Errorf("pgoDefaultFilesV2() with %q: want %v, got %v", tc.env, tc.want, got)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func sampleFiles() []AgentPromptFile {
	return []AgentPromptFile{
		{Name: "AGENTS_CORE.md", Body: "core"},
		{Name: "AGENTS_TASK.md", Body: "task"},
		{Name: "IDENTITY.md", Body: "identity"},
		{Name: "CAPABILITIES.md", Body: "caps"},
		{Name: "RULE.md", Body: "rule"},
	}
}

func fileNames(files []AgentPromptFile) []string {
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	return names
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
