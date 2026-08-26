package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// twinNoopLogger 测试用 noop 日志。
type twinNoopLogger struct{}

func (twinNoopLogger) Debug(string, ...loggateway.Field)            {}
func (twinNoopLogger) Info(string, ...loggateway.Field)             {}
func (twinNoopLogger) Warn(string, ...loggateway.Field)             {}
func (twinNoopLogger) Error(string, ...loggateway.Field)            {}
func (l twinNoopLogger) With(...loggateway.Field) loggateway.Logger { return l }

// newTwinFacadeForTest 构造仅含 token/订阅表的门面（不触达 Usecase 依赖）。
func newTwinFacadeForTest(token string) *TwinOpenAPICompatService {
	return &TwinOpenAPICompatService{
		token:   token,
		lg:      twinNoopLogger{},
		hc:      &http.Client{Timeout: 5 * time.Second},
		subs:    make(map[string]*twinRunSub),
		idempot: make(map[string]string),
	}
}

// TestTwinGuardAuth 机器 token 鉴权：缺失/错误/合法三种形态（常量时间比较）。
func TestTwinGuardAuth(t *testing.T) {
	s := newTwinFacadeForTest("test-machine-token")
	h := s.guard(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"缺失 Authorization", "", http.StatusUnauthorized},
		{"错误 token", "Bearer wrong", http.StatusUnauthorized},
		{"非 Bearer 方案", "Basic test-machine-token", http.StatusUnauthorized},
		{"合法 token", "Bearer test-machine-token", http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("want status %d, got %d", tc.want, rec.Code)
			}
		})
	}
}

// twinCapturedEvent 测试接收端捕获的事件帧。
type twinCapturedEvent struct {
	Signature string
	Timestamp string
	Body      []byte
}

// TestTwinPostEventSignature 事件投递：X-Webhook-Signature v1=<hex> +
// X-Webhook-Timestamp 头与 twinmonitor verifyV1 验签口径一致
// （HMAC-SHA256(secret, "<ts>\n<body>")）。
func TestTwinPostEventSignature(t *testing.T) {
	const secret = "twin-webhook-secret"

	var mu sync.Mutex
	var captured []twinCapturedEvent
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = append(captured, twinCapturedEvent{
			Signature: r.Header.Get("X-Webhook-Signature"),
			Timestamp: r.Header.Get("X-Webhook-Timestamp"),
			Body:      body,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer recv.Close()

	s := newTwinFacadeForTest("tok")
	def := &biz.GraphDefinition{
		ID:   "g-1",
		Name: "demo",
		Nodes: []biz.NodeDef{
			{ID: "n1", Type: "agent", Description: "节点一"},
		},
	}
	s.registerSub("exec-1", def, recv.URL, secret)

	s.postEvent("exec-1", "run.started", map[string]any{"total_nodes": 1})

	mu.Lock()
	defer mu.Unlock()
	if len(captured) != 1 {
		t.Fatalf("want 1 event delivered, got %d", len(captured))
	}
	evt := captured[0]

	// 1. 头存在性与 v1 前缀
	hexSig, found := strings.CutPrefix(evt.Signature, "v1=")
	if !found {
		t.Fatalf("signature header missing v1= prefix: %q", evt.Signature)
	}
	if _, err := strconv.ParseInt(evt.Timestamp, 10, 64); err != nil {
		t.Fatalf("timestamp header not unix seconds: %q", evt.Timestamp)
	}

	// 2. 按 twinmonitor verifyV1 口径重算签名比对
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(evt.Timestamp))
	mac.Write([]byte("\n"))
	mac.Write(evt.Body)
	expect := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expect), []byte(hexSig)) {
		t.Fatalf("signature mismatch: expect %s, got %s", expect, hexSig)
	}

	// 3. 信封字段（对齐 twinmonitor AraneaWebhookEvent）
	var env twinEventEnvelope
	if err := json.Unmarshal(evt.Body, &env); err != nil {
		t.Fatalf("envelope unmarshal: %v", err)
	}
	if env.EventID == "" || env.EventType != "run.started" || env.RunID != "exec-1" || env.GraphID != "g-1" {
		t.Fatalf("envelope fields wrong: %+v", env)
	}
	if env.Payload["total_nodes"] != float64(1) {
		t.Fatalf("payload total_nodes wrong: %v", env.Payload)
	}
}

// TestTwinUnregisterSubStopsDelivery 订阅注销后事件不再投递。
func TestTwinUnregisterSubStopsDelivery(t *testing.T) {
	var calls int
	recv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer recv.Close()

	s := newTwinFacadeForTest("tok")
	s.registerSub("exec-2", &biz.GraphDefinition{ID: "g-1"}, recv.URL, "s")
	s.unregisterSub("exec-2")
	s.postEvent("exec-2", "run.completed", map[string]any{})
	if calls != 0 {
		t.Fatalf("event delivered after unregister: calls=%d", calls)
	}
}

// TestTwinRunStatusMapping aranea 执行状态 → twinmonitor 任务状态映射。
func TestTwinRunStatusMapping(t *testing.T) {
	if got := twinRunStatus(string(biz.GraphExecWaitingHuman)); got != "waiting_approval" {
		t.Fatalf("waiting_human should map to waiting_approval, got %q", got)
	}
	if got := twinRunStatus("running"); got != "running" {
		t.Fatalf("running should pass through, got %q", got)
	}
}

// TestTwinSubGC 过期订阅被惰性清理（防内存泄漏）。
func TestTwinSubGC(t *testing.T) {
	s := newTwinFacadeForTest("tok")
	stale := &twinRunSub{CreatedAt: time.Now().Add(-2 * twinSubTTL)}
	s.subs["stale"] = stale
	s.subs["fresh"] = &twinRunSub{CreatedAt: time.Now()}

	s.mu.Lock()
	s.gcSubsLocked()
	s.mu.Unlock()

	if s.getSub("stale") != nil {
		t.Fatal("stale sub should be gc'd")
	}
	if s.getSub("fresh") == nil {
		t.Fatal("fresh sub should survive gc")
	}
}

// TestTwinGraphInNamespace P1 图命名空间隔离：source 标签命中、twin_seed 存量兼容、
// 无标图/它源图/空 metadata 的排除口径。
func TestTwinGraphInNamespace(t *testing.T) {
	cases := []struct {
		name   string
		meta   map[string]any
		source string
		want   bool
	}{
		{"source 标命中", map[string]any{"source": "twinmonitor"}, "twinmonitor", true},
		{"存量种子戳兼容", map[string]any{"twin_seed": map[string]any{"key": "rca-team", "sha256": "x"}}, "twinmonitor", true},
		{"source 标+twin_seed 双命中", map[string]any{"source": "twinmonitor", "twin_seed": map[string]any{}}, "twinmonitor", true},
		{"无标 aranea 原生图排除", map[string]any{"_version": 1, "team_owned": true}, "twinmonitor", false},
		{"它源标签排除", map[string]any{"source": "other"}, "twinmonitor", false},
		{"空 metadata 排除", nil, "twinmonitor", false},
		{"twin_seed 不服务它源查询", map[string]any{"twin_seed": map[string]any{}}, "other", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := twinGraphInNamespace(tt.meta, tt.source); got != tt.want {
				t.Fatalf("twinGraphInNamespace(%v, %q) = %v, want %v", tt.meta, tt.source, got, tt.want)
			}
		})
	}
}

// TestEnsureTwinGraphSource 打标为合并语义：nil 初始化、保留 twin_seed 等既有键、覆写 source。
func TestEnsureTwinGraphSource(t *testing.T) {
	def := &biz.GraphDefinition{}
	ensureTwinGraphSource(def)
	if def.Metadata["source"] != twinSourceTag {
		t.Fatalf("nil metadata should be initialized and tagged, got %v", def.Metadata)
	}

	def = &biz.GraphDefinition{Metadata: map[string]any{
		"source":    "stale",
		"twin_seed": map[string]any{"key": "alarm-diagnosis"},
	}}
	ensureTwinGraphSource(def)
	if def.Metadata["source"] != twinSourceTag {
		t.Fatalf("source should be overwritten, got %v", def.Metadata["source"])
	}
	if _, ok := def.Metadata["twin_seed"].(map[string]any); !ok {
		t.Fatalf("twin_seed should be preserved, got %v", def.Metadata)
	}
}
