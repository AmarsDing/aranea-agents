package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// chunk_temporal_usecase_test.go G-32 T4 item 4 Usecase 接线钉阵：
// 空腿拒绝（ErrUnavailable，不蜕化 current 快报）、时针装配（nil→time.Now、
// 冻结时钟注入）、span 透传（RecordedAt 保持原样移交 reader 守卫——本层
// 不预拒绝，单一守卫点位在 reader 侧）。

type fakeTemporalReader struct {
	calls   int
	gotSpan TemporalSpan
	retErr  error
}

func (f *fakeTemporalReader) ListChunksAsOf(_ context.Context, _ string, span TemporalSpan, _ string) ([]KnowledgeChunkIncarnation, error) {
	f.calls++
	f.gotSpan = span
	return nil, f.retErr
}

func (f *fakeTemporalReader) RecallChunksAsOf(_ context.Context, _ string, _ []float32, span TemporalSpan, _ int, _ string) ([]KnowledgeChunkIncarnation, error) {
	f.calls++
	f.gotSpan = span
	return nil, f.retErr
}

func (f *fakeTemporalReader) SparseRecallChunksAsOf(_ context.Context, _, _ string, span TemporalSpan, _ int, _ string) ([]KnowledgeChunkIncarnation, error) {
	f.calls++
	f.gotSpan = span
	return nil, f.retErr
}

var _ ChunkTemporalReader = (*fakeTemporalReader)(nil)

// 未接线 ChunkTemporalReader：三腿全部结构性拒绝，reader 零调用。
func TestChunkTemporalUsecaseNilReaderRefuses(t *testing.T) {
	u := &Usecase{}
	span := TemporalSpan{ValidAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if _, err := u.ListChunksAsOf(context.Background(), "c1", span, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListChunksAsOf = %v, want ErrUnavailable", err)
	}
	if _, err := u.RecallChunksAsOf(context.Background(), "c1", []float32{0.1}, span, 5, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RecallChunksAsOf = %v, want ErrUnavailable", err)
	}
	if _, err := u.SparseRecallChunksAsOf(context.Background(), "c1", "q", span, 5, ""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("SparseRecallChunksAsOf = %v, want ErrUnavailable", err)
	}
}

// 冻结时钟注入：零 ValidAt 由装配时钟解析，reader 收到解析后 span。
func TestChunkTemporalUsecaseFrozenClockResolvesZeroValidAt(t *testing.T) {
	f := &fakeTemporalReader{}
	u := &Usecase{}
	frozen := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	u.SetChunkTemporalReader(f, func() time.Time { return frozen })
	if _, err := u.ListChunksAsOf(context.Background(), "c1", TemporalSpan{}, ""); err != nil {
		t.Fatalf("ListChunksAsOf: %v", err)
	}
	if f.calls != 1 {
		t.Fatalf("reader calls=%d want 1", f.calls)
	}
	if !f.gotSpan.ValidAt.Equal(frozen) {
		t.Fatalf("reader got ValidAt=%v want frozen %v", f.gotSpan.ValidAt, frozen)
	}
}

// nil clock → time.Now 生产形态：零 ValidAt 解析为现值附近。
func TestChunkTemporalUsecaseNilClockDefaultsToTimeNow(t *testing.T) {
	f := &fakeTemporalReader{}
	u := &Usecase{}
	u.SetChunkTemporalReader(f, nil)
	before := time.Now().Add(-time.Second)
	if _, err := u.ListChunksAsOf(context.Background(), "c1", TemporalSpan{}, ""); err != nil {
		t.Fatalf("ListChunksAsOf: %v", err)
	}
	if f.gotSpan.ValidAt.Before(before) || f.gotSpan.ValidAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("nil clock did not default to time.Now: %v", f.gotSpan.ValidAt)
	}
}

// RecordedAt 透传：本层不预拒绝（守卫单一点位在 reader 侧），span 原样移交、
// reader 拒绝错误原样上抛不吞错。
func TestChunkTemporalUsecaseRecordedAtPassthrough(t *testing.T) {
	f := &fakeTemporalReader{retErr: ErrUnsupportedOperation}
	u := &Usecase{}
	frozen := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	u.SetChunkTemporalReader(f, func() time.Time { return frozen })
	recorded := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	_, err := u.RecallChunksAsOf(context.Background(), "c1", []float32{0.1},
		TemporalSpan{RecordedAt: recorded}, 5, "")
	if !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("reader rejection must propagate: %v", err)
	}
	if !f.gotSpan.RecordedAt.Equal(recorded) {
		t.Fatalf("RecordedAt did not pass through: %v", f.gotSpan.RecordedAt)
	}
	if !f.gotSpan.ValidAt.Equal(frozen) { // ValidAt 零值仍由时针解析后同传。
		t.Fatalf("ValidAt resolution lost on passthrough: %v", f.gotSpan.ValidAt)
	}
}
