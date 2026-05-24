package sessionmemory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
)

// ConsolidateWriteResult holds JSON rows written atomically by UpsertFactsAndEpisodeBatch.
type ConsolidateWriteResult struct {
	FactRows     [][]byte
	EpisodeRow   []byte
	FactsWritten int
}

// UpsertFactsAndEpisodeBatch upserts facts and optionally one episode in a single SQLite transaction.
// Vector / pgvector index sync remains the caller's responsibility after commit.
func (st *Store) UpsertFactsAndEpisodeBatch(ctx context.Context, facts []MemoryFactUpsert, ep *EpisodeInsert) (*ConsolidateWriteResult, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	if len(facts) == 0 && ep == nil {
		return &ConsolidateWriteResult{}, nil
	}
	if len(facts) == 0 && ep != nil {
		return nil, errors.New("episode requires at least one fact")
	}

	tx, err := st.client.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	db := tx.Client()

	epID := ""
	if ep != nil {
		epID = strings.TrimSpace(ep.ID)
		if epID == "" {
			epID = uuid.NewString()
			epCopy := *ep
			epCopy.ID = epID
			ep = &epCopy
		}
	}

	out := &ConsolidateWriteResult{}
	for i := range facts {
		in := facts[i]
		if epID != "" && strings.TrimSpace(in.SourceEpisodeID) == "" {
			in.SourceEpisodeID = epID
		}
		raw, err := st.upsertFactRowOn(ctx, db, in)
		if err != nil {
			return nil, err
		}
		if err := st.recordPolicyOnTx(ctx, db, MemoryActionLogInsert{
			Action:        "UPSERT",
			TargetKind:    "fact",
			TargetID:      factIDFromRow(raw, in.ID),
			Reason:        strings.TrimSpace(in.SourceKind),
			PolicyVersion: biz.PolicyVersionConsolidateV1,
			SourceEventIDs: []string{
				strings.TrimSpace(in.SourceSessionID),
				strings.TrimSpace(in.SourceMessageID),
			},
			TurnID:       composeTurnRef(in.SourceSessionID, in.SourceMessageID),
			MetadataJSON: strings.TrimSpace(in.MetadataJSON),
		}); err != nil {
			return nil, err
		}
		out.FactRows = append(out.FactRows, raw)
	}
	out.FactsWritten = len(out.FactRows)

	if ep != nil {
		epRaw, err := st.insertEpisodeRowOn(ctx, db, *ep)
		if err != nil {
			return nil, err
		}
		out.EpisodeRow = epRaw
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func factIDFromRow(raw []byte, fallback string) string {
	if id := strings.TrimSpace(fallback); id != "" {
		return id
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err == nil {
		if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
			return id
		}
	}
	return fallback
}
