package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupEnvelopeTestRepo boots the production memory-chain schema plus the
// 20261325 content_enc column (same ALTER as the migration file), and wires a
// fixed-test-key fact cipher into Data.
func setupEnvelopeTestRepo(t *testing.T, keySeed byte) (*Data, *l3FactRepo, *sql.DB) {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if err := EnsureSessionMemorySchema(ctx, client, DialectPostgres, loggateway.NewNoop()); err != nil {
		t.Fatalf("ensure session memory schema: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE memory_facts ADD COLUMN IF NOT EXISTS content_enc TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatalf("add content_enc column: %v", err)
	}
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	k := make([]byte, 32)
	for i := range k {
		k[i] = keySeed
	}
	crypto := biz.NewCredentialCrypto(func(context.Context) ([]byte, error) { return k, nil }, loggateway.NewNoop())
	d.SetFactContentCipher(biz.NewCredentialFactCipher(crypto))
	return d, newL3FactRepo(d, nil), db
}

func envelopeFactID(t *testing.T, raw []byte) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatal("fact id missing from upsert result")
	}
	return id
}

// TestFactEnvelope_WriteEncrypts_ReadDecrypts 钉死 D3 N-2 拍板 B 的核心链路：
// UpsertFactRow 落库时 content_enc 为密文（无明文泄漏），经 scanFactRowJSON
// 透明解密后读出原文。
func TestFactEnvelope_WriteEncrypts_ReadDecrypts(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()

	const stmt = "信封测试：张三的项目代号是 Azure"
	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:       "agent",
		ScopeID:         "envelope-agent",
		AgentID:         "envelope-agent",
		Statement:       stmt,
		DetailsMarkdown: "细节：跨部门协作备忘",
		FactKind:        "note",
		Importance:      0.7,
		Confidence:      1.0,
		SourceKind:      "manual",
	})
	if err != nil {
		t.Fatalf("UpsertFactRow: %v", err)
	}
	id := envelopeFactID(t, raw)

	var contentEnc string
	if err := db.QueryRowContext(ctx,
		`SELECT content_enc FROM memory_facts WHERE id = $1`, id).Scan(&contentEnc); err != nil {
		t.Fatalf("read content_enc: %v", err)
	}
	if contentEnc == "" {
		t.Fatal("content_enc must be written (envelope on write)")
	}
	if strings.Contains(contentEnc, "Azure") || strings.Contains(contentEnc, "张三") {
		t.Fatal("content_enc leaks plaintext")
	}

	got, err := r.GetFactRowsByIDs(ctx, []string{id})
	if err != nil {
		t.Fatalf("GetFactRowsByIDs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 fact, got %d", len(got))
	}
	var m map[string]any
	if err := json.Unmarshal(got[0], &m); err != nil {
		t.Fatal(err)
	}
	if s, _ := m["statement"].(string); s != stmt {
		t.Fatalf("decrypted statement mismatch: got %q want %q", s, stmt)
	}
	if det, _ := m["details_markdown"].(string); det != "细节：跨部门协作备忘" {
		t.Fatalf("decrypted details mismatch: got %q", det)
	}
}

// TestFactEnvelope_AuthoritativeOverCarrier：信封解密结果必须覆写（可能被
// 后续裁剪的）明文载体列——content_enc 才是权威全文。
func TestFactEnvelope_AuthoritativeOverCarrier(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:  "agent",
		ScopeID:    "carrier-agent",
		AgentID:    "carrier-agent",
		Statement:  "原始权威全文",
		FactKind:   "note",
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("UpsertFactRow: %v", err)
	}
	id := envelopeFactID(t, raw)

	// 模拟「载体列被裁剪/污染」：信封仍在，载体已坏。
	if _, err := db.ExecContext(ctx,
		`UPDATE memory_facts SET statement = 'CARRIER-CORRUPTED' WHERE id = $1`, id); err != nil {
		t.Fatalf("corrupt carrier: %v", err)
	}

	got, err := r.GetFactRowsByIDs(ctx, []string{id})
	if err != nil || len(got) != 1 {
		t.Fatalf("GetFactRowsByIDs: %v n=%d", err, len(got))
	}
	var m map[string]any
	if err := json.Unmarshal(got[0], &m); err != nil {
		t.Fatal(err)
	}
	if s, _ := m["statement"].(string); s != "原始权威全文" {
		t.Fatalf("envelope must override corrupted carrier, got %q", s)
	}
}

// TestFactEnvelope_DecryptFailureFallsBackToCarrier：密文损坏时读路径降级
// 为明文载体（不 crash、不把密文透出）。
func TestFactEnvelope_DecryptFailureFallsBackToCarrier(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:  "agent",
		ScopeID:    "fallback-agent",
		AgentID:    "fallback-agent",
		Statement:  "载体可用时的回退文本",
		FactKind:   "note",
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("UpsertFactRow: %v", err)
	}
	id := envelopeFactID(t, raw)

	if _, err := db.ExecContext(ctx,
		`UPDATE memory_facts SET content_enc = 'AAAA-corrupted' WHERE id = $1`, id); err != nil {
		t.Fatalf("corrupt envelope: %v", err)
	}

	got, err := r.GetFactRowsByIDs(ctx, []string{id})
	if err != nil || len(got) != 1 {
		t.Fatalf("GetFactRowsByIDs must not fail on corrupt envelope: %v n=%d", err, len(got))
	}
	var m map[string]any
	if err := json.Unmarshal(got[0], &m); err != nil {
		t.Fatal(err)
	}
	if s, _ := m["statement"].(string); s != "载体可用时的回退文本" {
		t.Fatalf("fallback to carrier statement expected, got %q", s)
	}
	if _, leaked := m["content_enc"]; leaked {
		t.Fatal("ciphertext must never enter the scanned JSON map")
	}
}

// TestFactEnvelope_NilCipherWritesPlainCarrier：无密钥环境整体降级——写不
// 产生信封、读走载体，全链路不 panic。
func TestFactEnvelope_NilCipherWritesPlainCarrier(t *testing.T) {
	d, r, db := setupEnvelopeTestRepo(t, 0x2a)
	d.SetFactContentCipher(nil)
	ctx := context.Background()

	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:  "agent",
		ScopeID:    "nocipher-agent",
		AgentID:    "nocipher-agent",
		Statement:  "无密钥环境的明文载体",
		FactKind:   "note",
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("UpsertFactRow: %v", err)
	}
	id := envelopeFactID(t, raw)

	var contentEnc string
	if err := db.QueryRowContext(ctx,
		`SELECT content_enc FROM memory_facts WHERE id = $1`, id).Scan(&contentEnc); err != nil {
		t.Fatalf("read content_enc: %v", err)
	}
	if contentEnc != "" {
		t.Fatalf("nil cipher must not write envelope, got %q", contentEnc)
	}

	got, err := r.GetFactRowsByIDs(ctx, []string{id})
	if err != nil || len(got) != 1 {
		t.Fatalf("read without cipher: %v n=%d", err, len(got))
	}
	var m map[string]any
	if err := json.Unmarshal(got[0], &m); err != nil {
		t.Fatal(err)
	}
	if s, _ := m["statement"].(string); s != "无密钥环境的明文载体" {
		t.Fatalf("carrier statement expected, got %q", s)
	}
}

// TestFactRecall_PIIGateExcludesUserScopePIIFacts 钉死 D3 user 域 PII 门：
// user scope 且 pii_flag=1 的事实不可召回，其余行不受影响。
func TestFactRecall_PIIGateExcludesUserScopePIIFacts(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()

	piiRaw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:  "user",
		ScopeID:    "user-1",
		UserID:     "user-1",
		Statement:  "PII gated secret 4862",
		FactKind:   "note",
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("upsert pii fact: %v", err)
	}
	cleanRaw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:  "user",
		ScopeID:    "user-1",
		UserID:     "user-1",
		Statement:  "普通可见笔记：季度目标是稳定性",
		FactKind:   "note",
		SourceKind: "manual",
	})
	if err != nil {
		t.Fatalf("upsert clean fact: %v", err)
	}
	piiID := envelopeFactID(t, piiRaw)
	if _, err := db.ExecContext(ctx,
		`UPDATE memory_facts SET pii_flag = 1 WHERE id = $1`, piiID); err != nil {
		t.Fatalf("flag pii: %v", err)
	}
	_ = cleanRaw

	rows, err := r.RecallL3Facts(ctx, "user", "user-1", "user-1", "目标", nil, 10, 0)
	if err != nil {
		t.Fatalf("RecallL3Facts: %v", err)
	}
	for _, b := range rows {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		id, _ := m["id"].(string)
		if id == piiID {
			t.Fatal("PII-flagged user-scope fact must not be recallable")
		}
	}
}
