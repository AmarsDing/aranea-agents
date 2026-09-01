package biz

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// stubDeliverableSaver 记录 Save 调用并返回固定 artifact 元数据。
type stubDeliverableSaver struct {
	calls int
	last  struct{ sessionID, name, mime string }
	ret   Artifact
	err   error
}

func (s *stubDeliverableSaver) Save(_ context.Context, sessionID, name, mimeType string, _ []byte) (Artifact, error) {
	s.calls++
	s.last.sessionID, s.last.name, s.last.mime = sessionID, name, mimeType
	return s.ret, s.err
}

func newBridgeDelivery(saver deliverableArtifactSaver, publicBase string) *SpiritDelivery {
	return &SpiritDelivery{lg: loggateway.NewNoop(), artifactSaver: saver, artifactPublicBase: publicBase}
}

func TestBridgeDeliverableDocs_BackfillsArtifactRef(t *testing.T) {
	saver := &stubDeliverableSaver{ret: Artifact{ID: "art-1", StorageURI: "sess-1/云计算十年-v0.md"}}
	d := newBridgeDelivery(saver, "")
	team := Team{ID: "t1", SpiritSessionID: "sess-1"}
	state := map[string]any{
		"article": map[string]any{"title": "云计算十年", "format": "markdown", "content": "# 正文"},
	}
	d.bridgeDeliverableDocs(context.Background(), team, state)

	if saver.calls != 1 {
		t.Fatalf("expected 1 save, got %d", saver.calls)
	}
	if saver.last.sessionID != "sess-1" || saver.last.name != "云计算十年.md" || saver.last.mime != "text/markdown" {
		t.Fatalf("unexpected save args: %+v", saver.last)
	}
	v := state["article"].(map[string]any)
	if v["artifact_id"] != "art-1" || v["artifact_rel"] != "sess-1/云计算十年-v0.md" {
		t.Fatalf("expected backfilled artifact_id/artifact_rel, got %+v", v)
	}
	// 幂等：已带 artifact_id 的 payload 不再桥接
	d.bridgeDeliverableDocs(context.Background(), team, state)
	if saver.calls != 1 {
		t.Fatalf("expected idempotent skip, got %d saves", saver.calls)
	}
}

func TestBridgeDeliverableDocs_SkipsNonDocAndNilSaver(t *testing.T) {
	saver := &stubDeliverableSaver{ret: Artifact{ID: "art-x"}}
	d := newBridgeDelivery(saver, "")
	team := Team{ID: "t1", SpiritSessionID: "sess-1"}
	state := map[string]any{
		"empty":   map[string]any{"title": "空", "content": "  "},
		"plain":   "纯字符串 payload",
		"pointer": map[string]any{"artifact_id": "已有", "sha256": "x"},
	}
	d.bridgeDeliverableDocs(context.Background(), team, state)
	if saver.calls != 0 {
		t.Fatalf("expected no saves, got %d", saver.calls)
	}
	// saver=nil 时桥接关闭且不 panic
	newBridgeDelivery(nil, "").bridgeDeliverableDocs(context.Background(), team,
		map[string]any{"doc": map[string]any{"content": "x"}})
}

func TestDeliverableDocMimeExt(t *testing.T) {
	cases := map[string]struct{ mime, ext string }{
		"markdown": {"text/markdown", ".md"},
		"html":     {"text/html", ".html"},
		"text":     {"text/plain", ".txt"},
		"":         {"text/markdown", ".md"},
		"PDF":      {"text/markdown", ".md"}, // 未识别兜底 markdown
	}
	for format, want := range cases {
		mime, ext := deliverableDocMimeExt(format)
		if mime != want.mime || ext != want.ext {
			t.Fatalf("format %q: want %s/%s, got %s/%s", format, want.mime, want.ext, mime, ext)
		}
	}
}

func TestArtifactDigestLine_RendersPublicPath(t *testing.T) {
	art := DeliverableArtifact{
		Key: "article", Title: "云计算十年", Format: "markdown", SizeChars: 8234,
		ArtifactID: "art-1", StorageURI: "sess-1/云计算十年-v0.md",
	}
	d := newBridgeDelivery(nil, `\\192.168.0.108\deploy102\aranea\docker\volumes\data\artifacts`)
	line := d.artifactDigestLine(art)
	wantTail := `路径：\\192.168.0.108\deploy102\aranea\docker\volumes\data\artifacts\sess-1\云计算十年-v0.md`
	if !strings.Contains(line, "article《云计算十年》（markdown，8234字）") || !strings.Contains(line, wantTail) {
		t.Fatalf("unexpected digest line: %s", line)
	}
	// 未配置 publicBase 时省略路径（本地部署不泄露服务端相对路径之外的信息）
	d2 := newBridgeDelivery(nil, "")
	if line2 := d2.artifactDigestLine(art); strings.Contains(line2, "路径：") {
		t.Fatalf("expected no path without publicBase, got: %s", line2)
	}
}

func TestFillArtifactFromPayload_BridgedKeepsStateKeyKind(t *testing.T) {
	v := map[string]any{
		"title": "报告", "format": "markdown", "content": "正文",
		"artifact_id": "art-1", "artifact_rel": "sess-1/报告-v0.md",
	}
	var art DeliverableArtifact
	fillArtifactFromPayload(&art, v)
	if art.ArtifactID != "art-1" || art.StorageURI != "sess-1/报告-v0.md" {
		t.Fatalf("expected artifact refs filled, got %+v", art)
	}
	// 桥接文档自带正文：Kind 必须保持 state_key（空），避免下游注入行为变化
	if art.ResolvedKind() != DeliverableArtifactKindStateKey {
		t.Fatalf("bridged doc must stay state_key, got %q", art.Kind)
	}
	// 无正文 + 有 artifact_id（生产者自声明指针）仍判 artifact
	var ptr DeliverableArtifact
	fillArtifactFromPayload(&ptr, map[string]any{"artifact_id": "art-9", "sha256": "x"})
	if ptr.ResolvedKind() != DeliverableArtifactKindArtifact {
		t.Fatalf("pointer payload must be artifact kind, got %q", ptr.Kind)
	}
}
