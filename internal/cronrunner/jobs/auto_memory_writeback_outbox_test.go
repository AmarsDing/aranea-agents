package jobs

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// G-10：写回失败批次必须落持久化重试队列（AS-EVT-01 Important 分级），
// 不再是 Warn-only 永久丢失。
func TestAutoMemoryWorker_WriteBackFailureEnqueuesOutbox(t *testing.T) {
	w := &AutoMemoryWorker{
		writeBack: &capturingWriteBack{err: errors.New("kb down")},
		lg:        loggateway.NewNoop(),
	}
	outbox := &fakeWriteBackOutbox{}
	w.writeBackOutbox = outbox

	sess := biz.Session{ID: "sess-g10", WorkspaceID: "default", AgentID: "a-1", UserID: "u-1"}
	candidates := []biz.FactWriteCandidate{{
		Statement: "用户偏好深色模式界面",
		FactKind:  "preference",
	}}
	writeRes := biz.FactWriteBatchResult{Added: 1}

	w.maybeWriteBack(context.Background(), sess, candidates, writeRes)
	if len(outbox.enqueued) != 1 {
		t.Fatalf("enqueued = %v, want 1 entry", outbox.enqueued)
	}
	wantKey := bizknowledge.WriteBackBatchKey(sess.ID, writeBackFactsFromPipeline(candidates, writeRes.FactRows))
	if outbox.enqueued[0] != wantKey {
		t.Fatalf("batch key = %q, want %q", outbox.enqueued[0], wantKey)
	}
}

// 写回成功不入队；入队自身失败只记日志（dead letter），不 panic、不影响主流程。
func TestAutoMemoryWorker_WriteBackSuccessSkipsOutbox(t *testing.T) {
	outbox := &fakeWriteBackOutbox{}
	w := &AutoMemoryWorker{
		writeBack:       &capturingWriteBack{},
		writeBackOutbox: outbox,
		lg:              loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-g10-ok", WorkspaceID: "default", AgentID: "a-1", UserID: "u-1"}
	candidates := []biz.FactWriteCandidate{{Statement: "用户偏好深色模式界面", FactKind: "preference"}}
	w.maybeWriteBack(context.Background(), sess, candidates, biz.FactWriteBatchResult{Added: 1})
	if len(outbox.enqueued) != 0 {
		t.Fatalf("success path enqueued = %v", outbox.enqueued)
	}
}

func TestAutoMemoryWorker_WriteBackOutboxEnqueueFailureIsDeadLetter(t *testing.T) {
	outbox := &fakeWriteBackOutbox{enqueueErr: errors.New("db down")}
	w := &AutoMemoryWorker{
		writeBack:       &capturingWriteBack{err: errors.New("kb down")},
		writeBackOutbox: outbox,
		lg:              loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-g10-dead", WorkspaceID: "default", AgentID: "a-1", UserID: "u-1"}
	candidates := []biz.FactWriteCandidate{{Statement: "用户偏好深色模式界面", FactKind: "preference"}}
	// 不 panic、不返回 error——Warn-only dead letter。
	w.maybeWriteBack(context.Background(), sess, candidates, biz.FactWriteBatchResult{Added: 1})
}

// G-10 编解码契约：payload 忠实还原（EntryTitle 执行期字段不入队）。
func TestWriteBackOutboxPayloadCodec(t *testing.T) {
	in := biz.KnowledgeWriteBackInput{
		Workspace: "default", SessionID: "sess-codec", AgentID: "a-1", UserID: "u-1",
		Facts: []biz.KnowledgeWriteBackFact{{
			FactID: "f-1", Statement: "用户偏好深色模式界面", FactKind: "preference",
			Confidence: 0.93, SourceKind: "dialogue", Tags: []string{"偏好"},
			EntryTitle: "执行期回填应被丢弃",
		}},
	}
	b, err := bizknowledge.EncodeWriteBackOutboxPayload(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := bizknowledge.DecodeWriteBackOutboxPayload(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.SessionID != in.SessionID || got.AgentID != in.AgentID || len(got.Facts) != 1 {
		t.Fatalf("decoded = %+v", got)
	}
	f := got.Facts[0]
	if f.Statement != in.Facts[0].Statement || f.Confidence != in.Facts[0].Confidence || len(f.Tags) != 1 {
		t.Fatalf("decoded fact = %+v", f)
	}
	if f.EntryTitle != "" {
		t.Fatalf("EntryTitle leaked into payload: %q", f.EntryTitle)
	}
	// Poison 输入：缺 session / 空 facts / 坏 JSON 一律 error（worker 翻 dead）。
	for _, bad := range [][]byte{[]byte(`{}`), []byte(`{"session_id":"s","facts":[]}`), []byte(`{bad`)} {
		if _, err := bizknowledge.DecodeWriteBackOutboxPayload(bad); err == nil {
			t.Fatalf("poison %q decoded without error", bad)
		}
	}
}
