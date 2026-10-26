// applier_doc_test.go —— W3 文档 Applier 钉阵（biz Usecase 管道 × 内存仓：
// 建/更/adopt/幂等短路/悬挂重绑/二进制降级/删除管辖权）+ Walker↔Runner↔Applier
// e2e 三轮钉（出席→更新→级联删除全链）。
package sourcesync

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ──────────────────────────────── 文档仓内存替身 ────────────────────────────────
// 嵌 bizknowledge.Repo 接口：applier 路径未触达的方法调用即 panic（触线=钉缺口显形）。

type docMemRepo struct {
	bizknowledge.Repo
	mu           sync.Mutex
	docs         map[string]bizknowledge.Document
	chunks       map[string][]bizknowledge.Chunk
	contentCalls int
	commitCalls  int
	createErr    error
	commitErr    error
	// 评审修轮焊钉载体（wf_6f89c52c-5be）：status 写调用流水（#D2 prevChunkCount
	// 焊钉断言源）+ 提交 docDelta 流水（#D1 计数信号焊钉断言源）+ 删除计数
	//（#W3-F1 孤儿补偿钉断言源）。
	statusCalls  []statusCall
	commitDeltas []int
	deleteCalls  int
}

// statusCall UpdateDocumentStatus 调用流水行。
type statusCall struct {
	id, status, errMsg string
	chunkCount         int
}

func newDocMemRepo() *docMemRepo {
	return &docMemRepo{docs: map[string]bizknowledge.Document{}, chunks: map[string][]bizknowledge.Chunk{}}
}

func (m *docMemRepo) CreateDocument(_ context.Context, d bizknowledge.Document) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return bizknowledge.Document{}, m.createErr
	}
	m.docs[d.ID] = d
	return d, nil
}

func (m *docMemRepo) GetDocument(_ context.Context, id string) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return bizknowledge.Document{}, apierror.NotFound(apierror.DomainKnowledge, "doc %s", id)
	}
	return d, nil
}

func (m *docMemRepo) GetDocumentByContentHash(_ context.Context, collectionID, contentHash string) (bizknowledge.Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.docs {
		if d.CollectionID == collectionID && d.ContentHash == contentHash && contentHash != "" {
			return d, nil
		}
	}
	return bizknowledge.Document{}, apierror.NotFound(apierror.DomainKnowledge, "hash %s", contentHash)
}

func (m *docMemRepo) UpdateDocumentContent(_ context.Context, id, contentText string, organized bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return apierror.NotFound(apierror.DomainKnowledge, "doc %s", id)
	}
	d.ContentText = contentText
	d.Organized = organized
	m.docs[id] = d
	m.contentCalls++
	return nil
}

func (m *docMemRepo) UpdateDocumentStatus(_ context.Context, id, status, errMsg string, chunkCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.docs[id]
	if !ok {
		return apierror.NotFound(apierror.DomainKnowledge, "doc %s", id)
	}
	d.Status = status
	d.ErrorMessage = errMsg
	if chunkCount > 0 || status == "indexed" {
		d.ChunkCount = chunkCount
	}
	m.docs[id] = d
	m.statusCalls = append(m.statusCalls, statusCall{id: id, status: status, errMsg: errMsg, chunkCount: chunkCount})
	return nil
}

func (m *docMemRepo) DeleteDocument(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.docs[id]; !ok {
		return apierror.NotFound(apierror.DomainKnowledge, "doc %s", id)
	}
	delete(m.docs, id)
	delete(m.chunks, id)
	m.deleteCalls++
	return nil
}

// CommitIndexedDocument IngestCommitter 权威通道：替换 chunks + meta + 计数同框。
func (m *docMemRepo) CommitIndexedDocument(_ context.Context, collectionID, docID string, chunks []bizknowledge.Chunk, docDelta int, syncMeta *bizknowledge.DocumentSyncMeta) (bizknowledge.ChunkDiffStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.commitErr != nil {
		return bizknowledge.ChunkDiffStats{}, m.commitErr
	}
	m.commitDeltas = append(m.commitDeltas, docDelta)
	d, ok := m.docs[docID]
	if !ok {
		return bizknowledge.ChunkDiffStats{}, apierror.NotFound(apierror.DomainKnowledge, "doc %s", docID)
	}
	child := 0
	for _, ch := range chunks {
		if !ch.IsParent {
			child++
		}
	}
	m.chunks[docID] = append([]bizknowledge.Chunk(nil), chunks...)
	d.Status = "indexed"
	d.ChunkCount = child
	if syncMeta != nil {
		d.ContentHash = syncMeta.ContentHash
		d.Summary = syncMeta.Summary
		d.SummaryHash = syncMeta.SummaryHash
		d.Tags = syncMeta.Tags
		d.DocType = syncMeta.DocType
	}
	m.docs[docID] = d
	m.commitCalls++
	return bizknowledge.ChunkDiffStats{Added: child, ChildAdded: child}, nil
}

func (m *docMemRepo) UpdateCollectionCounts(context.Context, string, int, int) error { return nil }

// ──────────────────────────────── 场景装配 ────────────────────────────────

type docApplierFixture struct {
	docs  *docMemRepo
	items *memItemRepo
	appl  *DocApplier
	conn  bizsync.Connector
	hooks map[string]int
	uc    *bizknowledge.Usecase
}

func newDocApplierFixture(t *testing.T) *docApplierFixture {
	t.Helper()
	repo := newDocMemRepo()
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	f := &docApplierFixture{docs: repo, items: newMemItemRepo(), uc: uc, hooks: map[string]int{}}
	appl, err := NewDocApplier(DocApplierDeps{UC: uc, Items: f.items, Log: loggateway.NewNoop()})
	if err != nil {
		t.Fatalf("NewDocApplier err = %v", err)
	}
	appl.SetEntityHook(func(string, string) { f.hooks["entity"]++ })
	appl.SetRelationHook(func(string, string) { f.hooks["relation"]++ })
	appl.SetBlockHook(func(context.Context, string, string, string) error { f.hooks["block"]++; return nil })
	appl.SetDiffHook(func(string, string, bizknowledge.ChunkDiffStats) { f.hooks["diff"]++ })
	f.appl = appl
	f.conn = bizsync.Connector{ID: "src:conn:1", Kind: bizsync.KindSharedDir, CollectionID: "col1"}
	return f
}

func textPayload(s string) *Payload {
	return &Payload{Body: io.NopCloser(strings.NewReader(s)), MIME: "text/markdown", SizeBytes: int64(len(s))}
}

// TestDocApplier_CreatePath 新建序列：pending→indexing→indexed、rel_path 落空、
// Source 剥扩展名、hash 落库、chunks 非空、图谱四 hook 各一次、own=true。
func TestDocApplier_CreatePath(t *testing.T) {
	f := newDocApplierFixture(t)
	body := "# Hello\nworld body\n"
	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "docs/hi.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "docs/hi.md"},
		textPayload(body))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if !out.OwnLifecycle || out.DocumentID == "" || out.ContentHash != bizknowledge.HashContent(body) {
		t.Fatalf("outcome = %+v", out)
	}
	doc, _ := f.docs.GetDocument(context.Background(), out.DocumentID)
	if doc.Status != "indexed" || doc.RelPath != "" || doc.Source != "docs/hi" || doc.ContentText != body {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.ContentHash != out.ContentHash || doc.ChunkCount == 0 {
		t.Fatalf("commit meta = %+v", doc)
	}
	for _, k := range []string{"entity", "relation", "block", "diff"} {
		if f.hooks[k] != 1 {
			t.Fatalf("hook %s = %d", k, f.hooks[k])
		}
	}
}

// TestDocApplier_IdempotentShortCircuit hash 一致且 indexed → 零写（content/chunks 不重编排）。
func TestDocApplier_IdempotentShortCircuit(t *testing.T) {
	f := newDocApplierFixture(t)
	body := "# Same\n"
	h := bizknowledge.HashContent(body)
	f.docs.docs["doc:1"] = bizknowledge.Document{ID: "doc:1", CollectionID: "col1", Source: "a", Status: "indexed", ContentHash: h, ContentText: body, ChunkCount: 2}
	f.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "a.md", DocumentID: "doc:1", OwnLifecycle: true, Status: bizsync.ItemStatusActive})

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "a.md", Fingerprint: h, DisplayPath: "a.md"}, textPayload(body))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if out.DocumentID != "doc:1" || !out.OwnLifecycle {
		t.Fatalf("outcome = %+v", out)
	}
	if f.docs.contentCalls != 0 || f.docs.commitCalls != 0 {
		t.Fatalf("幂等短路必零写: contentCalls=%d commitCalls=%d", f.docs.contentCalls, f.docs.commitCalls)
	}
	if f.hooks["diff"] != 0 {
		t.Fatalf("短路不发布帧")
	}
}

// TestDocApplier_UpdatePath 指纹漂移：同 docID 原位更新，own 保留，hash 推进。
func TestDocApplier_UpdatePath(t *testing.T) {
	f := newDocApplierFixture(t)
	oldBody := "# Old\n"
	f.docs.docs["doc:1"] = bizknowledge.Document{ID: "doc:1", CollectionID: "col1", Source: "a", Status: "indexed",
		ContentHash: bizknowledge.HashContent(oldBody), ContentText: oldBody, ChunkCount: 1}
	f.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "a.md", DocumentID: "doc:1", OwnLifecycle: true, Status: bizsync.ItemStatusActive})
	newBody := "# Old\nupdated line\n"

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "a.md", Fingerprint: bizknowledge.HashContent(newBody), DisplayPath: "a.md"}, textPayload(newBody))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if out.DocumentID != "doc:1" || !out.OwnLifecycle || out.ContentHash != bizknowledge.HashContent(newBody) {
		t.Fatalf("outcome = %+v", out)
	}
	doc, _ := f.docs.GetDocument(context.Background(), "doc:1")
	if doc.ContentText != newBody || doc.Status != "indexed" {
		t.Fatalf("更新落库 = %+v", doc)
	}
}

// TestDocApplier_AdoptBoundOnly adopt：同库同 hash 命中→登记不改写、own=false、
// 不建新档、不发帧（红线 3 与管辖权互斥双履约）。
func TestDocApplier_AdoptBoundOnly(t *testing.T) {
	f := newDocApplierFixture(t)
	body := "# Adopt me\n"
	f.docs.docs["doc:ui"] = bizknowledge.Document{ID: "doc:ui", CollectionID: "col1", Source: "upload",
		Status: "indexed", ContentHash: bizknowledge.HashContent(body), ContentText: body, OwnerUserID: "u1"}

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "a.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "a.md"}, textPayload(body))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if out.DocumentID != "doc:ui" || out.OwnLifecycle {
		t.Fatalf("adopt outcome = %+v", out)
	}
	if len(f.docs.docs) != 1 || f.docs.contentCalls != 0 || f.docs.commitCalls != 0 {
		t.Fatalf("adopt 必零写: %+v / %d / %d", f.docs.docs, f.docs.contentCalls, f.docs.commitCalls)
	}
	doc, _ := f.docs.GetDocument(context.Background(), "doc:ui")
	if doc.OwnerUserID != "u1" {
		t.Fatalf("adopt 不得改写归属: %+v", doc)
	}
}

// TestDocApplier_BinaryExtractorGate 需抽取 MIME：未配置 Extractor 硬错（零副作用）；
// 配置后走归一化管道且 docMime 保留原件类型。
func TestDocApplier_BinaryExtractorGate(t *testing.T) {
	f := newDocApplierFixture(t)
	raw := "%PDF-1.7 fake"
	snap := Snapshot{ExternalID: "deck.pdf", Fingerprint: bizknowledge.HashContent(raw), DisplayPath: "deck.pdf"}
	if _, err := f.appl.Upsert(context.Background(), f.conn, snap,
		&Payload{Body: io.NopCloser(strings.NewReader(raw)), MIME: "application/pdf", SizeBytes: int64(len(raw))}); err == nil {
		t.Fatalf("无 Extractor 必硬错进 fetch_failed 桶")
	}
	if len(f.docs.docs) != 0 {
		t.Fatalf("失败零副作用: %+v", f.docs.docs)
	}

	f.appl.d.Extractor = extractorFunc(func(_ context.Context, _ []byte, source, mime string) (string, error) {
		if source != "deck.pdf" || mime != "application/pdf" {
			t.Fatalf("extractor 入参: %q %q", source, mime)
		}
		return "# Deck\ncontent\n", nil
	})
	out, err := f.appl.Upsert(context.Background(), f.conn, snap,
		&Payload{Body: io.NopCloser(strings.NewReader(raw)), MIME: "application/pdf", SizeBytes: int64(len(raw))})
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	doc, _ := f.docs.GetDocument(context.Background(), out.DocumentID)
	if doc.MimeType != "application/pdf" || doc.ContentText != "# Deck\ncontent\n" {
		t.Fatalf("抽取落库 = %+v", doc)
	}
}

type extractorFunc func(ctx context.Context, raw []byte, source, mimeType string) (string, error)

func (fn extractorFunc) Extract(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	return fn(ctx, raw, source, mimeType)
}

// TestDocApplier_DanglingBindingRebinds 悬挂绑定（带外删档）→ adopt/create 重新建档
// （W2 评审修 #5 派生条款——不整轮 abort）。
func TestDocApplier_DanglingBindingRebinds(t *testing.T) {
	f := newDocApplierFixture(t)
	f.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "a.md", DocumentID: "doc:gone", OwnLifecycle: true, Status: bizsync.ItemStatusActive})
	body := "# Rebind\n"

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "a.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "a.md"}, textPayload(body))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if out.DocumentID == "doc:gone" || !out.OwnLifecycle {
		t.Fatalf("重绑 outcome = %+v", out)
	}
	if _, err := f.docs.GetDocument(context.Background(), out.DocumentID); err != nil {
		t.Fatalf("新档已建: %v", err)
	}
}

// TestDocApplier_DeleteJurisdiction 删除管辖权：own=true 才动文档级删；
// adopt 仅登记不接管删除；not-found 幂等吞。
func TestDocApplier_DeleteJurisdiction(t *testing.T) {
	f := newDocApplierFixture(t)
	f.docs.docs["doc:own"] = bizknowledge.Document{ID: "doc:own", CollectionID: "col1", Status: "indexed", ContentText: "x"}
	f.docs.docs["doc:foreign"] = bizknowledge.Document{ID: "doc:foreign", CollectionID: "col1", Status: "indexed", ContentText: "y"}

	if err := f.appl.Delete(context.Background(), f.conn,
		bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "a", DocumentID: "doc:own", OwnLifecycle: true}); err != nil {
		t.Fatalf("Delete own err = %v", err)
	}
	if _, err := f.docs.GetDocument(context.Background(), "doc:own"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("own 档已级联删除")
	}
	if err := f.appl.Delete(context.Background(), f.conn,
		bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "b", DocumentID: "doc:foreign", OwnLifecycle: false}); err != nil {
		t.Fatalf("adopt 不动文档: %v", err)
	}
	if _, err := f.docs.GetDocument(context.Background(), "doc:foreign"); err != nil {
		t.Fatalf("adopt 档必须存活: %v", err)
	}
	if err := f.appl.Delete(context.Background(), f.conn,
		bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "c", DocumentID: "doc:gone", OwnLifecycle: true}); err != nil {
		t.Fatalf("not-found 幂等: %v", err)
	}
}

// ──────────────────────────── 评审修轮焊钉（wf_6f89c52c-5be 确认项） ────────────────────────────

// TestDocApplier_AdoptDriftCreatesOwn 评审修 #W3-PIN-1：adopt 登记档（own=false）
// 源漂移→新 hash 未命中 adopt→转 own 自建（同库不同 hash 不撞红线 3）；原 adopt 档零触。
func TestDocApplier_AdoptDriftCreatesOwn(t *testing.T) {
	f := newDocApplierFixture(t)
	uiBody := "# ui original\n"
	f.docs.docs["doc:ui"] = bizknowledge.Document{ID: "doc:ui", CollectionID: "col1", Source: "upload",
		Status: "indexed", ContentHash: bizknowledge.HashContent(uiBody), ContentText: uiBody, OwnerUserID: "u1"}
	f.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "a.md", DocumentID: "doc:ui", OwnLifecycle: false, Status: bizsync.ItemStatusActive})
	drift := "# ui original\nsource drifted\n"

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "a.md", Fingerprint: bizknowledge.HashContent(drift), DisplayPath: "a.md"}, textPayload(drift))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if !out.OwnLifecycle || out.DocumentID == "doc:ui" {
		t.Fatalf("漂移转 own: %+v", out)
	}
	ui, _ := f.docs.GetDocument(context.Background(), "doc:ui")
	if ui.ContentText != uiBody || ui.OwnerUserID != "u1" {
		t.Fatalf("adopt 档零触: %+v", ui)
	}
	if len(f.docs.docs) != 2 {
		t.Fatalf("own 新档已建: %+v", f.docs.docs)
	}
}

// TestDocApplier_CountingSignalAndPrevChunkWeld 评审修 #D1/#D2 双焊钉：
// docDelta 唯看 content_hash 空白度（非空——含 error 残态重试——已计补 0，
// 空=首交计 1）；indexing CAS 传 prevChunkCount 保旧账（PG 无条件写 chunk_count 列）。
func TestDocApplier_CountingSignalAndPrevChunkWeld(t *testing.T) {
	// error 残态重试档：曾提交成功过（hash 非空）→内容漂移重启摄取
	f := newDocApplierFixture(t)
	old := "# v1\n"
	f.docs.docs["doc:ret"] = bizknowledge.Document{ID: "doc:ret", CollectionID: "col1", Source: "a", Status: "error",
		ContentHash: bizknowledge.HashContent(old), ContentText: old, ChunkCount: 7, ErrorMessage: "boom"}
	f.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "ret.md", DocumentID: "doc:ret", OwnLifecycle: true, Status: bizsync.ItemStatusActive})
	body := "# v1\nretry line\n"
	if _, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "ret.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "ret.md"}, textPayload(body)); err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	// D1：hash 非空⇒已计数⇒docDelta=0（vault status 口径在此序列双倍计数，挂账 W9）
	if len(f.docs.commitDeltas) != 1 || f.docs.commitDeltas[0] != 0 {
		t.Fatalf("D1 焊钉: commitDeltas = %v, want [0]", f.docs.commitDeltas)
	}
	// D2：indexing CAS 保旧账 chunk=7
	cas := 0
	for _, c := range f.docs.statusCalls {
		if c.id == "doc:ret" && c.status == "indexing" {
			cas++
			if c.chunkCount != 7 {
				t.Fatalf("D2 焊钉: indexing CAS chunkCount = %d, want 7", c.chunkCount)
			}
		}
	}
	if cas != 1 {
		t.Fatalf("indexing CAS 恰好一次: %+v", f.docs.statusCalls)
	}

	// 空白 hash（带外建档从未提交成功）⇒首次提交 docDelta=1
	f2 := newDocApplierFixture(t)
	f2.docs.docs["doc:new"] = bizknowledge.Document{ID: "doc:new", CollectionID: "col1", Source: "a", Status: "pending", ContentText: old}
	f2.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "new.md", DocumentID: "doc:new", OwnLifecycle: true, Status: bizsync.ItemStatusActive})
	if _, err := f2.appl.Upsert(context.Background(), f2.conn,
		Snapshot{ExternalID: "new.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "new.md"}, textPayload(body)); err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if len(f2.docs.commitDeltas) != 1 || f2.docs.commitDeltas[0] != 1 {
		t.Fatalf("首交计数: commitDeltas = %v, want [1]", f2.docs.commitDeltas)
	}
}

// TestDocApplier_CommitFailureMarkedErrorAndNoOrphan 评审修 #W3-F1/D4 孤儿补偿
// + markError 保账焊钉：新建路径提交失败→best-effort 回滚（无孤儿残档、hook
// 零发布）；更新路径提交失败→文档存活、error 态带 prevChunkCount（旧账不清零）。
func TestDocApplier_CommitFailureMarkedErrorAndNoOrphan(t *testing.T) {
	// 新建路径：孤儿补偿
	f := newDocApplierFixture(t)
	f.docs.commitErr = errors.New("pg down")
	body := "# orphan\n"
	if _, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "o.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "o.md"}, textPayload(body)); err == nil {
		t.Fatal("提交失败必须返回 error")
	}
	if len(f.docs.docs) != 0 || f.docs.deleteCalls != 1 {
		t.Fatalf("孤儿补偿钉: docs=%+v deleteCalls=%d", f.docs.docs, f.docs.deleteCalls)
	}
	for _, k := range []string{"entity", "relation", "block", "diff"} {
		if f.hooks[k] != 0 {
			t.Fatalf("失败轮 hook 零发布: %s=%d", k, f.hooks[k])
		}
	}

	// 更新路径：markError 保旧账（chunkCount=7 不清零）+ 文档存活不动删除
	f2 := newDocApplierFixture(t)
	old := "# old\n"
	f2.docs.docs["doc:u"] = bizknowledge.Document{ID: "doc:u", CollectionID: "col1", Source: "a", Status: "indexed",
		ContentHash: bizknowledge.HashContent(old), ContentText: old, ChunkCount: 7}
	f2.items.seed(bizsync.Item{ConnectorID: "src:conn:1", ExternalID: "u.md", DocumentID: "doc:u", OwnLifecycle: true, Status: bizsync.ItemStatusActive})
	f2.docs.commitErr = errors.New("pg down")
	nb := "# old\ndrift\n"
	if _, err := f2.appl.Upsert(context.Background(), f2.conn,
		Snapshot{ExternalID: "u.md", Fingerprint: bizknowledge.HashContent(nb), DisplayPath: "u.md"}, textPayload(nb)); err == nil {
		t.Fatal("更新提交失败必须返回 error")
	}
	if f2.docs.deleteCalls != 0 {
		t.Fatalf("更新路径不动删除: deleteCalls=%d", f2.docs.deleteCalls)
	}
	doc, gerr := f2.docs.GetDocument(context.Background(), "doc:u")
	if gerr != nil || doc.Status != "error" || doc.ChunkCount != 7 {
		t.Fatalf("error 态保账回写: %+v %v", doc, gerr)
	}
	errCalls := 0
	for _, c := range f2.docs.statusCalls {
		if c.id == "doc:u" && c.status == "error" {
			errCalls++
			if c.chunkCount != 7 {
				t.Fatalf("markError 保账钉: chunkCount = %d, want 7", c.chunkCount)
			}
		}
	}
	if errCalls != 1 {
		t.Fatalf("markError 恰好一次: %+v", f2.docs.statusCalls)
	}
}

// TestDocApplier_SelfProofWrongFingerprint 评审修 #W3-PIN-4 字节自证钉：Applier
// 不信 walker 指纹传输值——docHash 由 payload 字节重算；指纹被污染（指向他档
// hash）不得误 adopt、不得落错 hash。
func TestDocApplier_SelfProofWrongFingerprint(t *testing.T) {
	f := newDocApplierFixture(t)
	uiBody := "# ui doc\n"
	wrong := bizknowledge.HashContent(uiBody)
	f.docs.docs["doc:ui"] = bizknowledge.Document{ID: "doc:ui", CollectionID: "col1", Source: "upload",
		Status: "indexed", ContentHash: wrong, ContentText: uiBody, OwnerUserID: "u1"}
	body := "# watched\n"

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "w.md", Fingerprint: wrong /* 被污染的传输指纹 */, DisplayPath: "w.md"}, textPayload(body))
	if err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	if !out.OwnLifecycle || out.DocumentID == "doc:ui" {
		t.Fatalf("指纹不信：必须自建 own 档不误 adopt: %+v", out)
	}
	doc, _ := f.docs.GetDocument(context.Background(), out.DocumentID)
	if doc.ContentHash != bizknowledge.HashContent(body) {
		t.Fatalf("docHash 必须字节重算: %q want %q", doc.ContentHash, bizknowledge.HashContent(body))
	}
}

// TestDocApplier_HookPanicContained 评审修 #D3：图谱 hook panic 由 safeInvoke
// 拦截——主提交不回滚、后续 hook 照跑、Upsert 成功返回。
func TestDocApplier_HookPanicContained(t *testing.T) {
	f := newDocApplierFixture(t)
	f.appl.SetEntityHook(func(string, string) { panic("graph boom") })
	body := "# calm\n"

	out, err := f.appl.Upsert(context.Background(), f.conn,
		Snapshot{ExternalID: "h.md", Fingerprint: bizknowledge.HashContent(body), DisplayPath: "h.md"}, textPayload(body))
	if err != nil {
		t.Fatalf("hook panic 不得外逸: %v", err)
	}
	if _, gerr := f.docs.GetDocument(context.Background(), out.DocumentID); gerr != nil {
		t.Fatalf("主提交不回滚: %v", gerr)
	}
	if f.hooks["relation"] != 1 || f.hooks["block"] != 1 || f.hooks["diff"] != 1 {
		t.Fatalf("后续 hook 照跑: %+v", f.hooks)
	}
}

// ──────────────────────────────── e2e：walker↔runner↔applier 真链 ────────────────────────────────

// TestE2E_SharedDirThreeRounds 四轮全链钉：R1 建 2 → R2 全不变 → R3 原位更新 →
// R4 外部删除级联 tombstone（own=true 动文档、item 落 deleted、run 审计齐）。
func TestE2E_SharedDirThreeRounds(t *testing.T) {
	root := t.TempDir()
	writeSourceFile(t, root, "keep.md", "# keep\n")
	writeSourceFile(t, root, "gone.md", "# gone\n")

	repo := newDocMemRepo()
	uc := bizknowledge.NewUsecaseFromRepo(repo)
	conns := newMemConnectorRepo()
	items := newMemItemRepo()
	runs := &memRunRepo{}
	appl, err := NewDocApplier(DocApplierDeps{UC: uc, Items: items, Log: loggateway.NewNoop()})
	if err != nil {
		t.Fatal(err)
	}
	walker := NewSharedDirWalker(nil, nil)
	conn := bizsync.Connector{
		ID: "src:conn:e2e", Kind: bizsync.KindSharedDir, Name: "e2e", CollectionID: "col1",
		ConfigJSON:      `{"root": ` + jsonQuote(root) + `}`,
		IntervalSeconds: 900, Enabled: true, SyncState: bizsync.SyncStateIdle,
		MaxItemsPerRun: 500, MaxDeletesPerRun: 200, MaxFetchBytesPerRun: 512 << 20,
		CreatedAt: fixedClock, UpdatedAt: fixedClock,
	}
	conns.seed(conn)
	r := NewRunner(Deps{
		Connectors: conns, Items: items, Runs: runs, Locker: &fakeLocker{ok: true},
		Walkers:  map[bizsync.Kind]Walker{bizsync.KindSharedDir: walker},
		Appliers: map[bizsync.Kind]Applier{bizsync.KindSharedDir: appl},
		Now:      func() time.Time { return fixedClock },
	})
	ctx := context.Background()

	// R1：出席建 2
	out, err := r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess {
		t.Fatalf("R1: %v %s", err, out.Status)
	}
	if out.Stats.Created != 2 {
		t.Fatalf("R1 created = %+v", out.Stats)
	}
	if len(repo.docs) != 2 {
		t.Fatalf("R1 docs = %+v", repo.docs)
	}
	// R2：全不变
	out, err = r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess || out.Stats.Unchanged != 2 {
		t.Fatalf("R2: %v %s %+v", err, out.Status, out.Stats)
	}
	// R3：内容改 → 原位更新
	writeSourceFile(t, root, "keep.md", "# keep\nv2 line\n")
	out, err = r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess || out.Stats.Updated != 1 {
		t.Fatalf("R3: %v %s %+v", err, out.Status, out.Stats)
	}
	// R4：外部删除 → 确认级联（walk-absent 即真相）
	if err := os.Remove(filepath.Join(root, "gone.md")); err != nil {
		t.Fatal(err)
	}
	out, err = r.SyncOnce(ctx, conn.ID)
	if err != nil || out.Status != bizsync.RunStatusSuccess || out.Stats.Deleted != 1 {
		t.Fatalf("R4: %v %s %+v", err, out.Status, out.Stats)
	}
	goneItem, _ := items.get(conn.ID, "gone.md")
	if goneItem.Status != bizsync.ItemStatusDeleted {
		t.Fatalf("gone item = %+v", goneItem)
	}
	if len(repo.docs) != 1 {
		t.Fatalf("级联后仅剩 keep.md: %+v", repo.docs)
	}
	if runs.count() != 4 {
		t.Fatalf("审计四轮全落: %d", runs.count())
	}
}
