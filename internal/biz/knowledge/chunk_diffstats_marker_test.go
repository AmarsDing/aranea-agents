package knowledge

import (
	"context"
	"errors"
	"testing"
)

// ── G-32 T5 决策9 哨兵钉：ChunkDiffStats.Exact 的来源纪律 ─────────────────

// 权威通道（IngestCommitter 十三步 diff-commit）返回的统计必须被打上 Exact 标记，
// 且原值透传；T5 WS knowledge_chunks_diff 帧以 Exact 为发布闸。
func TestUsecase_CommitIndexedDocument_IngestCommitter_StatsMarkedExact(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, Status: "indexing"}, nil
	}
	ic := &ingestCommitterMockRepo{
		mockRepo: mr,
		stats:    ChunkDiffStats{Added: 1, Kept: 2, VersionNo: 3, Exact: false},
	}
	u := NewUsecaseFromRepo(ic)
	stats, err := u.CommitIndexedDocument(context.Background(), "col-1", "doc-1",
		[]Chunk{{ID: "c1", CollectionID: "col-1"}}, 1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if ic.calls != 1 {
		t.Fatalf("IngestCommitter calls = %d, want 1", ic.calls)
	}
	if !stats.Exact {
		t.Fatal("IngestCommitter 权威通道统计必须被打上 Exact=true 标记")
	}
	if stats.Added != 1 || stats.Kept != 2 || stats.VersionNo != 3 {
		t.Fatalf("权威统计原值失真（应原样透传仅补标记）: %+v", stats)
	}
}

// 退化顺序写路径（单元 fakes 无 IngestCommitter）返回伪全量汇总，
// Exact 必须为 false——「repo 不支持 DiffStats 时零发布」的哨兵落点。
func TestUsecase_CommitIndexedDocument_FallbackStats_NotExact(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, Status: "indexing"}, nil
	}
	u := NewUsecaseFromRepo(mr)
	stats, err := u.CommitIndexedDocument(context.Background(), "col-1", "doc-1",
		[]Chunk{{ID: "c1", CollectionID: "col-1"}}, 1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if stats.Exact {
		t.Fatal("退化 fakes 路径不得冒充权威 diff（Exact 必须 false）")
	}
	if stats.Added != 1 || stats.Deleted != 0 || stats.Kept != 0 {
		t.Fatalf("退化路径伪全量汇总失真: %+v (want Added=1 全量)", stats)
	}
}

// 权威通道报错时不得打标记也不可得利（stats 为错误侧零值，帧函数不因标记误发）。
func TestUsecase_CommitIndexedDocument_IngestCommitter_ErrorNoExactMarked(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, Status: "indexing"}, nil
	}
	ic := &ingestCommitterMockRepo{
		mockRepo: mr,
		err:      errors.New("ingest commit failed"),
	}
	u := NewUsecaseFromRepo(ic)
	stats, err := u.CommitIndexedDocument(context.Background(), "col-1", "doc-1",
		[]Chunk{{ID: "c1", CollectionID: "col-1"}}, 1)
	if err == nil {
		t.Fatal("底层错误必须透传")
	}
	if stats.Exact {
		t.Fatal("错误路径不得出现 Exact=true（帧函数绝不可借错误统计误发）")
	}
}

// ingestCommitterMockRepo 在 mockRepo 之上实现 IngestCommitter——命中 Usecase
// 的权威 diff-commit 通道分支（T4 接线契约）。
type ingestCommitterMockRepo struct {
	*mockRepo
	stats ChunkDiffStats
	err   error
	calls int
}

func (m *ingestCommitterMockRepo) CommitIndexedDocument(_ context.Context, _, _ string, _ []Chunk, _ int, _ *DocumentSyncMeta) (ChunkDiffStats, error) {
	m.calls++
	return m.stats, m.err
}
