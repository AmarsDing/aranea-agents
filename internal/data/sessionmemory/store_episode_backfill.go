package sessionmemory

import (
	"context"
	"strings"
)

// EpisodeEmbedCandidate is one episode row pending vector embedding.
type EpisodeEmbedCandidate struct {
	ID      string
	AgentID string
	Title   string
	Summary string
}

// ListEpisodesPendingEmbedding returns episodes without ready embedding blobs.
func (st *Store) ListEpisodesPendingEmbedding(ctx context.Context, limit int) ([]EpisodeEmbedCandidate, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 32
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := st.client.QueryContext(ctx, `
SELECT id, agent_id, title, outcome_summary FROM memory_episodes
WHERE deleted_at = '' AND consolidation_status = 'consolidated'
 AND (embedding_blob IS NULL OR embedding_status = '' OR embedding_status = 'pending')
ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EpisodeEmbedCandidate
	for rows.Next() {
		var c EpisodeEmbedCandidate
		if err := rows.Scan(&c.ID, &c.AgentID, &c.Title, &c.Summary); err != nil {
			return nil, err
		}
		c.ID = strings.TrimSpace(c.ID)
		if c.ID != "" {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}
