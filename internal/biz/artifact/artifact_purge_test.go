package artifact

// artifact_purge_test.go — T-3 附件生命周期（2026-10-26 评审二轮）：
// Usecase.DeleteBySession 的窄接口断言与 no-op 语义钉板。

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// purgeStubRepo 实现 Repo + SessionPurger 的测试桩。
type purgeStubRepo struct {
	purged []string
	err    error
}

func (s *purgeStubRepo) Load(context.Context, string, int) (Artifact, []byte, error) {
	return Artifact{}, nil, fmt.Errorf("not found")
}
func (s *purgeStubRepo) LoadMeta(context.Context, string, int) (Artifact, error) {
	return Artifact{}, fmt.Errorf("not found")
}
func (s *purgeStubRepo) LoadMetas(context.Context, []string, int) ([]Artifact, error) {
	return nil, nil
}
func (s *purgeStubRepo) List(context.Context, string, int, int) ([]Artifact, int, error) {
	return nil, 0, nil
}
func (s *purgeStubRepo) ListBySessionAndName(context.Context, string, string) ([]Artifact, error) {
	return nil, nil
}
func (s *purgeStubRepo) Save(_ context.Context, sessionID, name, _ string, data []byte) (Artifact, error) {
	return Artifact{ID: NewArtifactID(), SessionID: sessionID, Name: name, Size: int64(len(data))}, nil
}
func (s *purgeStubRepo) Delete(context.Context, string) error { return nil }
func (s *purgeStubRepo) DeleteVersion(context.Context, string, string, int) error {
	return nil
}
func (s *purgeStubRepo) PurgeBySession(_ context.Context, sessionID string) error {
	s.purged = append(s.purged, sessionID)
	return s.err
}

// noPurgeRepo 不实现 SessionPurger 的测试桩（断言 no-op 语义）。显式逐方法
// 转发而非内嵌——内嵌会把 PurgeBySession 一并提升，反而实现了窄接口。
type noPurgeRepo struct{ inner purgeStubRepo }

func (s *noPurgeRepo) Load(ctx context.Context, id string, v int) (Artifact, []byte, error) {
	return s.inner.Load(ctx, id, v)
}
func (s *noPurgeRepo) LoadMeta(ctx context.Context, id string, v int) (Artifact, error) {
	return s.inner.LoadMeta(ctx, id, v)
}
func (s *noPurgeRepo) LoadMetas(ctx context.Context, ids []string, v int) ([]Artifact, error) {
	return s.inner.LoadMetas(ctx, ids, v)
}
func (s *noPurgeRepo) List(ctx context.Context, sid string, l, o int) ([]Artifact, int, error) {
	return s.inner.List(ctx, sid, l, o)
}
func (s *noPurgeRepo) ListBySessionAndName(ctx context.Context, sid, n string) ([]Artifact, error) {
	return s.inner.ListBySessionAndName(ctx, sid, n)
}
func (s *noPurgeRepo) Save(ctx context.Context, sid, n, m string, d []byte) (Artifact, error) {
	return s.inner.Save(ctx, sid, n, m, d)
}
func (s *noPurgeRepo) Delete(ctx context.Context, id string) error { return s.inner.Delete(ctx, id) }
func (s *noPurgeRepo) DeleteVersion(ctx context.Context, sid, n string, v int) error {
	return s.inner.DeleteVersion(ctx, sid, n, v)
}

// ① 支持窄接口的 repo：透传 purge。
func TestDeleteBySession_DelegatesToPurger(t *testing.T) {
	repo := &purgeStubRepo{}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if err := uc.DeleteBySession(context.Background(), "sess-1"); err != nil {
		t.Fatal(err)
	}
	if len(repo.purged) != 1 || repo.purged[0] != "sess-1" {
		t.Fatalf("purged = %v, want [sess-1]", repo.purged)
	}
}

// ② 空 sessionID 拒绝。
func TestDeleteBySession_EmptyIDRejected(t *testing.T) {
	repo := &purgeStubRepo{}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if err := uc.DeleteBySession(context.Background(), "  "); err == nil {
		t.Fatal("empty session_id must be rejected")
	}
	if len(repo.purged) != 0 {
		t.Fatal("repo must not be called for empty session_id")
	}
}

// ③ repo 不支持窄接口时 no-op（不报错、不 panic）——测试桩/未来其他
// 存储后端不被迫实现会话级清理。
func TestDeleteBySession_UnsupportedRepoNoop(t *testing.T) {
	uc := NewUsecase(&noPurgeRepo{}, loggateway.NewNoop())
	if err := uc.DeleteBySession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("unsupported repo must be a no-op, got %v", err)
	}
}

// ④ purge 错误透传给调用方（session usecase 侧告警不回滚）。
func TestDeleteBySession_PropagatesPurgeError(t *testing.T) {
	repo := &purgeStubRepo{err: fmt.Errorf("disk gone")}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if err := uc.DeleteBySession(context.Background(), "sess-1"); err == nil {
		t.Fatal("purge error must propagate")
	}
}
