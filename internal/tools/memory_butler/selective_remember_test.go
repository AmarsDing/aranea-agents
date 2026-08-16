package memory_butler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ── P2-3 测试夹具：真实 MemoryAdminUsecase + nil 嵌入 stub（同
// service/memory_list_facts_test.go 模式），仅覆盖被调用的方法。 ─────────────

type stubSelectiveAdminDeps struct {
	biz.MemoryAdminDeps // nil 嵌入；仅 ListFactRows 可被调用
	rows                [][]byte
}

func (s *stubSelectiveAdminDeps) ListFactRows(_ context.Context, scopeType, scopeID, _, _, _, _ string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	return s.rows, int32(len(s.rows)), 0, 0, nil
}

type stubSelectiveFactWriter struct {
	biz.L3FactWriter // nil 嵌入；仅 UpsertFactRow 可被调用
	last             biz.FactUpsert
	calls            int
}

func (s *stubSelectiveFactWriter) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	s.calls++
	s.last = in
	return []byte(`{"id":"f-new","agent_id":"a1","statement":"x"}`), nil
}

// stubEmbedder 输出可控向量：vecs 按调用序依次返回 contentVec / 每条 statement
// 同一个 existingVec，从而精确控制余弦相似度。
type stubEmbedder struct {
	contentVec  []float32
	existingVec []float32
	err         error
	calls       int
}

func (s *stubEmbedder) EmbedSingle(context.Context, string) ([]float32, error) { return nil, errors.New("not used") }

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i := range out {
		if i == 0 {
			out[i] = s.contentVec
		} else {
			out[i] = s.existingVec
		}
	}
	return out, nil
}

func newSelectiveTestDeps(rows [][]byte, embedder *stubEmbedder) (Deps, *stubSelectiveFactWriter) {
	adminDeps := &stubSelectiveAdminDeps{rows: rows}
	writer := &stubSelectiveFactWriter{}
	admin := biz.NewMemoryAdminUsecase(adminDeps, nil, nil, writer, loggateway.NewNoop())
	d := Deps{MemoryAdmin: admin, LG: loggateway.NewNoop()}
	if embedder != nil {
		d.Embedder = embedder
	}
	return d, writer
}

func callSelectiveRemember(t *testing.T, d Deps, content string) selectiveRememberOutput {
	t.Helper()
	tl, ok := newSelectiveRememberTool(d).(trpctool.CallableTool)
	if !ok {
		t.Fatal("selective_remember must be callable")
	}
	args, _ := json.Marshal(map[string]string{"agent_id": "a1", "content": content})
	out, err := tl.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res, ok := out.(selectiveRememberOutput)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	return res
}

// 精确重复：字符串判重拦截，不落库、不调 Embedder。
func TestSelectiveRemember_ExactDupSkipped(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"f1","statement":"部署必须走灰度"}`)}
	embed := &stubEmbedder{contentVec: []float32{1, 0}, existingVec: []float32{1, 0}}
	deps, writer := newSelectiveTestDeps(rows, embed)
	out := callSelectiveRemember(t, deps, "部署必须走灰度")
	if out.Remembered {
		t.Fatal("exact duplicate must not be remembered")
	}
	if writer.calls != 0 {
		t.Fatalf("upsert must not run on dup, got %d calls", writer.calls)
	}
	if embed.calls != 0 {
		t.Fatalf("embed must not run when string dedup hits, got %d calls", embed.calls)
	}
}

// 语义重复：字符串不命中但余弦 1.0 ≥ 0.92 → 拦截，不落库。
func TestSelectiveRemember_SemanticDupSkipped(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"f1","statement":"部署必须走灰度发布"}`)}
	embed := &stubEmbedder{contentVec: []float32{0.5, 0.5}, existingVec: []float32{0.5, 0.5}}
	deps, writer := newSelectiveTestDeps(rows, embed)
	out := callSelectiveRemember(t, deps, "上线务必经过灰度通道")
	if out.Remembered {
		t.Fatal("semantic duplicate must not be remembered")
	}
	if out.Reason != "semantically redundant with existing memory" {
		t.Fatalf("reason = %q", out.Reason)
	}
	if writer.calls != 0 {
		t.Fatalf("upsert must not run on semantic dup, got %d calls", writer.calls)
	}
	if embed.calls != 1 {
		t.Fatalf("embed must run once for semantic pass, got %d calls", embed.calls)
	}
}

// 语义不重复（正交向量）→ 落库，且置信度/重要度为 butler 默认值。
func TestSelectiveRemember_NovelWritesWithDefaults(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"f1","statement":"部署必须走灰度发布"}`)}
	embed := &stubEmbedder{contentVec: []float32{1, 0}, existingVec: []float32{0, 1}}
	deps, writer := newSelectiveTestDeps(rows, embed)
	out := callSelectiveRemember(t, deps, "用户偏好简洁直接的回复风格")
	if !out.Remembered {
		t.Fatal("novel content must be remembered")
	}
	if writer.calls != 1 {
		t.Fatalf("upsert calls = %d, want 1", writer.calls)
	}
	if writer.last.Confidence != selectiveRememberConfidence {
		t.Fatalf("confidence = %v, want %v（零值会被召回阈值永久过滤）", writer.last.Confidence, selectiveRememberConfidence)
	}
	if writer.last.Importance != selectiveRememberImportance {
		t.Fatalf("importance = %v, want %v", writer.last.Importance, selectiveRememberImportance)
	}
}

// Embedder 未接线：降级纯字符串判重，新颖内容照常落库。
func TestSelectiveRemember_NilEmbedderDegrades(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"f1","statement":"部署必须走灰度发布"}`)}
	deps, writer := newSelectiveTestDeps(rows, nil)
	out := callSelectiveRemember(t, deps, "用户偏好简洁直接的回复风格")
	if !out.Remembered {
		t.Fatal("nil embedder must degrade to string-only dedup, novel content remembered")
	}
	if writer.calls != 1 {
		t.Fatalf("upsert calls = %d, want 1", writer.calls)
	}
}

// Embedder 调用失败：降级不阻断写入。
func TestSelectiveRemember_EmbedFailureDegrades(t *testing.T) {
	rows := [][]byte{[]byte(`{"id":"f1","statement":"部署必须走灰度发布"}`)}
	embed := &stubEmbedder{err: errors.New("embedding svc down")}
	deps, writer := newSelectiveTestDeps(rows, embed)
	out := callSelectiveRemember(t, deps, "用户偏好简洁直接的回复风格")
	if !out.Remembered {
		t.Fatal("embed failure must not block novel write")
	}
	if writer.calls != 1 {
		t.Fatalf("upsert calls = %d, want 1", writer.calls)
	}
}

func TestCosineSimilarity32(t *testing.T) {
	if sim := cosineSimilarity32([]float32{1, 2, 3}, []float32{1, 2, 3}); sim < 0.9999 {
		t.Fatalf("identical vectors cosine should be ~1, got %v", sim)
	}
	if sim := cosineSimilarity32([]float32{1, 0}, []float32{0, 1}); sim != 0 {
		t.Fatalf("orthogonal vectors cosine should be 0, got %v", sim)
	}
	if sim := cosineSimilarity32([]float32{1, 2}, []float32{1, 2, 3}); sim != 0 {
		t.Fatalf("length mismatch should return 0, got %v", sim)
	}
	if sim := cosineSimilarity32([]float32{0, 0}, []float32{1, 1}); sim != 0 {
		t.Fatalf("zero vector should return 0, got %v", sim)
	}
}
