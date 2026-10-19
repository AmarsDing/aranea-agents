package runtime

import (
	"aranea-agents/pkg/loggateway"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

func (q *PendingMessageQueue) restore() {
	if q.dir == "" {
		return
	}
	path := filepath.Join(q.dir, pendingSnapshotFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			q.lg.Warn("pending queue snapshot read failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		}
		return
	}
	var snapshot map[string][]PendingMessage
	if err := json.Unmarshal(data, &snapshot); err != nil {
		q.lg.Warn("pending queue snapshot unmarshal failed", loggateway.StepID("runtime.pending_queue.snapshot"), loggateway.Err(err))
		return
	}
	cutoff := time.Now().Add(-2 * time.Hour)
	q.mu.Lock()
	defer q.mu.Unlock()
	for sid, entries := range snapshot {
		var kept []PendingMessage
		for _, e := range entries {
			t, err := time.Parse(time.RFC3339, e.CreatedAt)
			if err == nil && t.After(cutoff) {
				kept = append(kept, e)
			}
		}
		if len(kept) > 0 {
			q.queues[sid] = kept
		}
	}
}
