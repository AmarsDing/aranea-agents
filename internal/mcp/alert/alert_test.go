package alert

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/mcp/metadata"
)

func TestSustainedErrorAfter_Default(t *testing.T) {
	d := SustainedErrorAfter()
	if d != DefaultSustainedErrorAfter {
		t.Errorf("expected %v, got %v", DefaultSustainedErrorAfter, d)
	}
}

func TestMaybeEmitAfterHealth_NilPublisher(t *testing.T) {
	var p *Publisher
	p.MaybeEmitAfterHealth(nil, biz.MCPServer{}, biz.MCPTestResult{OK: false, Status: "error"})
}

func TestMaybeEmitAfterHealth_OKResult(t *testing.T) {
	p := &Publisher{}
	p.MaybeEmitAfterHealth(nil, biz.MCPServer{}, biz.MCPTestResult{OK: true, Status: "ok"})
}

func TestMaybeEmitAfterHealth_TooEarly(t *testing.T) {
	p := &Publisher{}
	srv := biz.MCPServer{ID: "s1", Key: "test", MetadataJSON: "{}"}
	result := biz.MCPTestResult{OK: false, Status: "error"}
	p.MaybeEmitAfterHealth(nil, srv, result)
}

func TestMaybeEmitAfterHealth_SustainedError(t *testing.T) {
	p := &Publisher{}
	now := time.Now().UTC()
	meta := metadata.Parse("{}")
	updatedMeta, _ := metadata.ApplyHealth(meta, "error", false, "connection refused", now.Add(-10*time.Minute))
	raw, err := metadata.Marshal(updatedMeta)
	if err != nil {
		t.Fatal(err)
	}

	srv := biz.MCPServer{ID: "s1", Key: "test", MetadataJSON: raw}
	result := biz.MCPTestResult{OK: false, Status: "error", Message: "connection refused"}
	p.MaybeEmitAfterHealth(nil, srv, result)
}
