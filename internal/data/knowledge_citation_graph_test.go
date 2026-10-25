package data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
)

func TestGraphCitationSnapshot_IdentityAndVersionValidation(t *testing.T) {
	for _, bad := range []string{"raw-chunk", "g33e/0/d", "g33r/no/d", "g33e/1/"} {
		if _, _, _, ok := parseGraphCitationID(bad); ok {
			t.Fatalf("invalid card reference accepted: %q", bad)
		}
	}
	kind, ref, doc, ok := parseGraphCitationID("g33r/11/host")
	if !ok || kind != "edge" || ref != 11 || doc != "host" {
		t.Fatalf("valid reference rejected: %s %d %s %v", kind, ref, doc, ok)
	}
	snapshot := graphCitationSnapshot{collectionID: "c1", content: "historical card"}
	if snapshot.valid() {
		t.Fatal("unversioned card must not be accepted")
	}
	snapshot.hash = bizknowledge.GraphCardSourceHash(snapshot.content)
	if !snapshot.valid() {
		t.Fatal("valid historical card snapshot rejected")
	}
	snapshot.content = "current replacement"
	if snapshot.valid() {
		t.Fatal("current card must not silently replace a historical snapshot")
	}
}

func TestGraphCitationTrace_UsesSnapshotAndOriginalViewerACL(t *testing.T) {
	repo := setupGraphCardPG(t)
	d := repo.data
	for _, sql := range []string{
		`ALTER TABLE knowledge_collections ADD COLUMN workspace TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE knowledge_chunks (id TEXT PRIMARY KEY, content TEXT NOT NULL, cited_count INT NOT NULL DEFAULT 0)`,
		`CREATE TABLE knowledge_chunk_citations (chunk_id TEXT, turn_id TEXT, created_at TIMESTAMPTZ, PRIMARY KEY(chunk_id, turn_id))`,
		`CREATE TABLE steps_v2 (id TEXT PRIMARY KEY, turn_id TEXT, kind TEXT, notice_type TEXT, content TEXT, is_final BOOL, seq BIGINT, started_at TIMESTAMPTZ)`,
		`INSERT INTO knowledge_graph_embeddings(kind, ref_id, collection_id, text) VALUES ('entity',2,'c1','current card version')`,
	} {
		if _, err := d.Postgres().ExecContext(context.Background(), sql); err != nil {
			t.Fatal(err)
		}
	}
	const snapshot = "private entity card at recall time"
	for _, entry := range []struct {
		turn string
		uid  int64
	}{
		{"owner", 42}, {"outsider", 7},
	} {
		payload, err := json.Marshal(map[string]any{
			"workspace_id": "ws-1", "viewer_user_id": entry.uid,
			"chunks": []map[string]any{{"chunk_id": "g33e/2/mine", "collection_id": "c1", "n": 1,
				"evidence_kind": "entity", "evidence_hash": bizknowledge.GraphCardSourceHash(snapshot), "evidence_content": snapshot}},
		})
		if err != nil {
			t.Fatal(err)
		}
		for seq, part := range []struct{ kind, content, notice string }{
			{"notice", string(payload), bizknowledge.KnowledgeRecalledNoticeType}, {"reply", "answer [1]", ""},
		} {
			if _, err := d.Postgres().ExecContext(context.Background(),
				`INSERT INTO steps_v2 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())`,
				entry.turn+part.kind, entry.turn, part.kind, part.notice, part.content, part.kind == "reply", seq); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Worker 的 system 身份不能替代原调用者的当前 private/shared 授权。
	reader := NewKnowledgeCitationTraceReader(d)
	got, err := reader.ListKnowledgeCitationCandidates(workspace.WithSystemWorkspace(context.Background()), time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TurnID != "owner" || got[0].Chunks[0].Content != snapshot {
		t.Fatalf("snapshot/version/viewer isolation failed: %+v", got)
	}
	recorder := NewKnowledgeChunkCitationRecorder(d)
	for i := 0; i < 2; i++ {
		if err := recorder.RecordChunkCitations(context.Background(), []bizknowledge.ChunkCitation{{ChunkID: "g33e/2/mine", TurnID: "owner"}}); err != nil {
			t.Fatal(err)
		}
	}
	var ledger, fakeChunks int
	if err := d.Postgres().QueryRow(`SELECT COUNT(*) FROM knowledge_chunk_citations`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err := d.Postgres().QueryRow(`SELECT COUNT(*) FROM knowledge_chunks`).Scan(&fakeChunks); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 || fakeChunks != 0 {
		t.Fatalf("derived citation must be idempotent without fabricating chunks: ledger=%d fake=%d", ledger, fakeChunks)
	}
}
