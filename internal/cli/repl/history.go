package repl

import (
	"os"
	"path/filepath"

	"github.com/peterh/liner"
)

// historyPath returns the path to the liner history file.
func historyPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "aranea")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "repl_history")
}

// loadHistory reads the liner history file.
func loadHistory(l *liner.State) {
	path := historyPath()
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = l.ReadHistory(f)
}

// saveHistory writes the liner history file.
func saveHistory(l *liner.State) {
	path := historyPath()
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = l.WriteHistory(f)
}
