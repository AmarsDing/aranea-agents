package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
)

// TestBackfillVectorEmbeddingsFromBlob is a MANUAL, one-off repair job for the
// 2026-08-26 domain-B regression root cause: fact embeddings were written only
// to the per-agent agent_memory_<dim> store and memory_facts.embedding_blob,
// never to the shared vector_embeddings read index that L2/L3 recall searches.
//
// It copies existing embedding_blob values (no re-embedding) into
// vector_embeddings via upsert, so it is idempotent and safe to re-run.
//
// Usage:
//
//	ARANEA_VECTOR_BACKFILL=1 \
//	ARANEA_VECTOR_BACKFILL_DSN="postgres://postgres:123456@127.0.0.1:5432/aranea?sslmode=disable" \
//	go test ./internal/data/ -run TestBackfillVectorEmbeddingsFromBlob -v -count=1
//
// The DSN env var is mandatory and has no default so the job can never
// accidentally run against the wrong database.
func TestBackfillVectorEmbeddingsFromBlob(t *testing.T) {
	if os.Getenv("ARANEA_VECTOR_BACKFILL") != "1" {
		t.Skip("set ARANEA_VECTOR_BACKFILL=1 to run the vector_embeddings backfill")
	}
	dsn := strings.TrimSpace(os.Getenv("ARANEA_VECTOR_BACKFILL_DSN"))
	if dsn == "" {
		t.Fatal("ARANEA_VECTOR_BACKFILL_DSN is required (explicitly point at the target DB)")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open target DB: %v", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping target DB: %v", err)
	}

	// Detect the declared dimension of vector_embeddings.embedding, e.g.
	// "vector(1024)". Facts whose embedding length disagrees are skipped, not
	// forced in (a dim mismatch would fail at the PG type layer anyway).
	var colType string
	err = db.QueryRowContext(ctx, `SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		WHERE a.attrelid = 'vector_embeddings'::regclass AND a.attname = 'embedding'`).Scan(&colType)
	if err != nil {
		t.Fatalf("detect vector_embeddings column type: %v", err)
	}
	colDim := parseVectorDim(t, colType)
	t.Logf("vector_embeddings.embedding type = %s (dim=%d)", colType, colDim)

	rows, err := db.QueryContext(ctx, `SELECT id, COALESCE(agent_id, ''), COALESCE(user_id, ''), statement, embedding_blob, embedding_dim
		FROM memory_facts
		WHERE embedding_status = 'fresh' AND embedding_blob IS NOT NULL AND deleted_at = ''
		ORDER BY id`)
	if err != nil {
		t.Fatalf("query memory_facts: %v", err)
	}
	defer rows.Close()

	var scanned, upserted, dimSkipped, corruptSkipped int
	var firstErr error
	for rows.Next() {
		var id, agentID, userID, statement string
		var blob []byte
		var embDim int
		if err := rows.Scan(&id, &agentID, &userID, &statement, &blob, &embDim); err != nil {
			t.Fatalf("scan fact row: %v", err)
		}
		scanned++
		if len(blob) == 0 || len(blob)%4 != 0 {
			corruptSkipped++
			t.Logf("skip %s: corrupt embedding_blob (%d bytes)", id, len(blob))
			continue
		}
		emb := decodeFloat32Blob(blob)
		if colDim > 0 && len(emb) != colDim {
			dimSkipped++
			t.Logf("skip %s: embedding dim %d != column dim %d (declared %d)", id, len(emb), colDim, embDim)
			continue
		}
		meta, _ := json.Marshal(map[string]string{
			"agent_id": agentID,
			"user_id":  userID,
			"content":  statement,
		})
		_, uerr := db.ExecContext(ctx, `INSERT INTO vector_embeddings (id, embedding, meta) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET embedding = $2, meta = $3`,
			id, pgvector.NewVector(emb), string(meta))
		if uerr != nil {
			t.Logf("upsert %s failed: %v", id, uerr)
			if firstErr == nil {
				firstErr = uerr
			}
			continue
		}
		upserted++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fact rows: %v", err)
	}
	t.Logf("backfill done: scanned=%d upserted=%d dimSkipped=%d corruptSkipped=%d", scanned, upserted, dimSkipped, corruptSkipped)
	if firstErr != nil {
		t.Fatalf("at least one upsert failed, first error: %v", firstErr)
	}
	if upserted == 0 && scanned > 0 {
		t.Fatalf("no rows upserted out of %d scanned (dim mismatch? column type %s)", scanned, colType)
	}

	// Sanity: the read index must now cover every fresh fact embedding.
	var uncovered int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_facts m
		LEFT JOIN vector_embeddings v ON v.id = m.id
		WHERE m.embedding_status = 'fresh' AND m.embedding_blob IS NOT NULL AND m.deleted_at = '' AND v.id IS NULL`).Scan(&uncovered); err != nil {
		t.Fatalf("verify coverage: %v", err)
	}
	t.Logf("coverage check: %d fresh facts still missing from vector_embeddings", uncovered)
	if dimSkipped == 0 && corruptSkipped == 0 && uncovered != 0 {
		t.Fatalf("coverage check failed: %d fresh facts missing from vector_embeddings", uncovered)
	}
}

func parseVectorDim(t *testing.T, typ string) int {
	t.Helper()
	typ = strings.TrimSpace(typ)
	if !strings.HasPrefix(typ, "vector(") || !strings.HasSuffix(typ, ")") {
		t.Fatalf("unexpected vector_embeddings.embedding type %q, want vector(N)", typ)
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(typ, "vector("), ")"))
	if err != nil || n <= 0 {
		t.Fatalf("parse dim from %q: %v", typ, err)
	}
	return n
}
