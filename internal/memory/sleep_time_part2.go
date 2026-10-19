package memory

import (
	"time"
)

const consolidationSystemPrompt = `You are an isolated memory consolidation agent. You have no tools, no network, and must not spawn agents, fetch URLs, or collaborate with other agents.

Analyse the provided memories and produce consolidation operations as JSON.

Return a JSON object with an "operations" array. Each operation has a "type" field:
- "merge": Merge duplicate or highly similar memories. Provide "target_id" (ID of memory to update), "source_ids" (IDs to delete after merge), "merged_content" (new content), and optionally "merged_topics".
- "reflect": Extract a higher-level reflection or insight. Provide "reflection" (text) and "topics" (array of strings).
- "update_core": Update core memory (key facts). Provide "updates" (array of {"key": "...", "value": "..."} objects).

Rules:
- Only emit operations when memories contain durable high-signal facts (preferences, constraints, decisions, corrections). Otherwise return {"operations": []}.
- Never persist secrets, API keys, passwords, tokens, or raw PII. If a memory only contains those, skip it.
- Do not invent facts that are not in the input.

Only produce operations when clearly warranted. If no consolidation is needed, return {"operations": []}.`

// consolidationQueue is a simple in-memory implementation of ConsolidationQueue.
type consolidationQueue struct {
	ch chan ConsolidationJobRequest
}

// NewConsolidationQueue creates a new in-memory consolidation queue with the
// given buffer size. Falls back to a default size when bufferSize <= 0.
func NewConsolidationQueue(bufferSize int) ConsolidationQueue {
	if bufferSize <= 0 {
		bufferSize = consolidationQueueDefaultSize
	}
	return &consolidationQueue{
		ch: make(chan ConsolidationJobRequest, bufferSize),
	}
}

func (q *consolidationQueue) Enqueue(r ConsolidationJobRequest) {
	if q == nil {
		return
	}
	if r.EnqueuedAt.IsZero() {
		r.EnqueuedAt = time.Now()
	}
	select {
	case q.ch <- r:
	default:

	}
}

func (q *consolidationQueue) Chan() <-chan ConsolidationJobRequest {
	if q == nil {
		return nil
	}
	return q.ch
}
