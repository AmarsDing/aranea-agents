package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/outboundwebhook"
)

// R3 3.3 审批中心推送器（C7 桥出站侧）测试。

// 未配置 TWIN_WEBHOOK_SECRET → 推送器禁用（nil），pending 落库不受影响。
func TestTwinMemoryFactPendingNotifierDisabledWithoutSecret(t *testing.T) {
	t.Setenv(twinWebhookSecretEnv, "")
	if n := NewTwinMemoryFactPendingNotifier(nil); n != nil {
		t.Fatalf("expect nil notifier without secret, got %T", n)
	}
}

// 配置齐全 → POST 到 webhook 地址，事件类型/载荷正确，v1 签名可验。
func TestTwinMemoryFactPendingNotifierDelivers(t *testing.T) {
	var gotBody []byte
	var gotSig, gotTs string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(outboundwebhook.HeaderSignature)
		gotTs = r.Header.Get(outboundwebhook.HeaderTimestamp)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv(twinWebhookSecretEnv, "bridge-secret")
	t.Setenv(twinWebhookURLEnv, srv.URL)
	n := NewTwinMemoryFactPendingNotifier(nil)
	if n == nil {
		t.Fatal("expect notifier wired")
	}
	rec := biz.MemoryFactPendingRecord{
		ID: "mfp-uuid-1", AgentID: "agent-1", FactKey: "fact-1",
		Verdict: biz.MemoryFactPendingVerdictUpdate, ProposedBody: "新表述", PriorBody: "旧表述",
	}
	n.NotifyFactWritePending(context.Background(), rec)

	if gotSig == "" || gotTs == "" {
		t.Fatal("missing signature headers")
	}
	ts, err := strconv.ParseInt(gotTs, 10, 64)
	if err != nil {
		t.Fatalf("bad timestamp: %v", err)
	}
	if err := outboundwebhook.Verify("bridge-secret", ts, gotBody, gotSig, 10*time.Minute); err != nil {
		t.Fatalf("signature verify failed: %v", err)
	}
	var env twinEventEnvelope
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("bad envelope: %v", err)
	}
	if env.EventType != twinEventFactWritePending {
		t.Fatalf("event_type = %q, want %q", env.EventType, twinEventFactWritePending)
	}
	if env.RunID != "mfp-uuid-1" {
		t.Fatalf("run_id（pending id 复用位）= %q, want mfp-uuid-1", env.RunID)
	}
	if env.Payload["pending_id"] != "mfp-uuid-1" || env.Payload["verdict"] != "UPDATE" {
		t.Fatalf("unexpected payload: %+v", env.Payload)
	}
	if env.Payload["title"] == "" {
		t.Fatal("expect title prefilled")
	}
}

// 投递失败（对端 5xx / 不可达）不 panic、不阻塞——best-effort 语义。
func TestTwinMemoryFactPendingNotifierBestEffort(t *testing.T) {
	t.Setenv(twinWebhookSecretEnv, "s")
	t.Setenv(twinWebhookURLEnv, "http://127.0.0.1:1") // 不可达
	n := NewTwinMemoryFactPendingNotifier(nil)
	// 不 panic 即通过（投递失败仅日志告警）。
	n.NotifyFactWritePending(context.Background(), biz.MemoryFactPendingRecord{ID: "mfp-x"})
}
