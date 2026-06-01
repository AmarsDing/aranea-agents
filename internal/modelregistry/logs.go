package modelregistry

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

func AppendSyncLog(store *Store, entry SyncLogEntry) error {
	if store == nil {
		return nil
	}
	if err := store.ensureDir(); err != nil {
		return err
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(store.LogsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func ReadSyncLogs(store *Store, limit int) ([]SyncLogEntry, error) {
	if store == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	f, err := os.Open(store.LogsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	start := 0
	if len(lines) > limit {
		start = len(lines) - limit
	}
	out := make([]SyncLogEntry, 0, len(lines)-start)
	for i := len(lines) - 1; i >= start; i-- {
		var e SyncLogEntry
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func UpdateSyncLogEntry(store *Store, entry SyncLogEntry) error {
	if store == nil || strings.TrimSpace(entry.ID) == "" {
		return nil
	}
	if err := store.ensureDir(); err != nil {
		return err
	}
	f, err := os.Open(store.LogsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return AppendSyncLog(store, entry)
		}
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e SyncLogEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.ID == entry.ID {
			b, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			lines = append(lines, string(b))
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return AppendSyncLog(store, entry)
	}
	body := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(store.LogsPath(), []byte(body), 0o644)
}
