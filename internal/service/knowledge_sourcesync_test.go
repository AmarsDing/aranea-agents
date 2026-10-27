// knowledge_sourcesync_test.go —— G-38 W5 custom HTTP 面契约钉（⑦ API 契约）。
//
// 钉点域：装配守卫（nil admin → 503，前端可探测未启用）、方法分派（非契约
// 方法 → 405）、PATCH 合并语义（nil 字段继承既有；运行态字段本层根本不接收——
// 请求体无 sync_state/next_run_at 槽位）、config_json 双形态归一、limit 钳制
// 下限、审计条目形状（③ "<verb>.source_connector"，凭据面绝不带 secret——
// secret 只存在于解码后的 sourceCredentialBody，出参 CredentialView 只有
// configured+masked_preview，从结构上没有泄露槽位）。
//
// 纯 handler 面钉：状态码映射权威钉在 knowledgesync admin_test.go（真编排
// + fake repo），本层不重复装配。

package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

// ── 503 装配守卫（requireSourceSyncAdmin） ────────────────────────────────

func TestSourceSyncHandlers_UnavailableWithoutAdmin(t *testing.T) {
	svc := NewKnowledgeService(nil, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	cases := []struct {
		name   string
		method string
		call   func(w http.ResponseWriter, r *http.Request)
	}{
		{"collection", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectors(w, r) }},
		{"connector", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnector(w, r, "id") }},
		{"sync", http.MethodPost, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorSync(w, r, "id") }},
		{"runs", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorRuns(w, r, "id") }},
		{"items", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorItems(w, r, "id") }},
		{"credential", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorCredential(w, r, "id") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/v1/knowledge/connectors", nil)
			tc.call(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("got %d, want 503 unavailable", rec.Code)
			}
		})
	}
}

// ── 405 方法分派（两个 handler 的 default 分支在触碰 repo 之前返回） ──────

func TestSourceSyncHandlers_MethodNotAllowed(t *testing.T) {
	// admin 非 nil（repo 全空）：仅 405 分支不会触碰 repo。
	admin := knowledgesync.NewAdminService(knowledgesync.AdminDeps{
		Log: loggateway.NewNoop(),
	})
	svc := NewKnowledgeService(nil, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc.SetSourceSyncAdmin(admin)

	cases := []struct {
		name   string
		method string
		call   func(w http.ResponseWriter, r *http.Request)
	}{
		{"collection_PUT", http.MethodPut, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectors(w, r) }},
		{"collection_DELETE", http.MethodDelete, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectors(w, r) }},
		{"sync_GET", http.MethodGet, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorSync(w, r, "id") }},
		{"sync_PATCH", http.MethodPatch, func(w http.ResponseWriter, r *http.Request) { svc.ServeSourceConnectorSync(w, r, "id") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/v1/knowledge/connectors", nil)
			tc.call(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("got %d, want 405", rec.Code)
			}
		})
	}
}

// ── config_json 双形态归一 ───────────────────────────────────────────────

func TestConfigJSONString_DualForm(t *testing.T) {
	obj := json.RawMessage(`{"path":"/data","API_KEY":"x"}`)
	got, err := configJSONString(obj)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"path":"/data","API_KEY":"x"}` {
		t.Fatalf("object form: got %q", got)
	}

	str := json.RawMessage(`"{\"path\":\"/data\"}"`)
	got, err = configJSONString(str)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"path":"/data"}` {
		t.Fatalf("string form: got %q", got)
	}

	// 空/缺失/null → 空串（由调用方决定继承或拒绝）。
	for _, raw := range []json.RawMessage{nil, json.RawMessage(""), json.RawMessage("  "), json.RawMessage("null")} {
		got, err = configJSONString(raw)
		if err != nil || got != "" {
			t.Fatalf("empty-ish %q: got %q err=%v, want empty nil", raw, got, err)
		}
	}

	// 数字/数组 → 按对象原样落入（ValidateConfig 或下游会拒非对象形状）。
	num := json.RawMessage(`42`)
	if got, err = configJSONString(num); err != nil || got != "42" {
		t.Fatalf("scalar passthrough: got %q err=%v", got, err)
	}
}

// ── PATCH 合并语义 ───────────────────────────────────────────────────────

func TestMergeConnectorBody_InheritsUnspecified(t *testing.T) {
	disabled := false
	cur := bizsync.Connector{
		ID:              "ss:conn:1",
		Kind:            bizsync.KindSharedDir,
		Name:            "lib",
		CollectionID:    "col1",
		ConfigJSON:      `{"path":"/data"}`,
		IntervalSeconds: 1800,
		Enabled:         true,
		MaxItemsPerRun:  500,
	}
	// 空 PATCH：全部继承既有值（含 Enabled 保真，非强制 true）。
	in := mergeConnectorBody(sourceConnectorBody{}, cur)
	if in.Kind != string(cur.Kind) || in.Name != cur.Name || in.CollectionID != cur.CollectionID {
		t.Fatalf("identity fields drifted: %+v", in)
	}
	if in.ConfigJSON != cur.ConfigJSON {
		t.Fatalf("config drifted: %q", in.ConfigJSON)
	}
	if in.IntervalSeconds != cur.IntervalSeconds {
		t.Fatalf("interval drifted: %d", in.IntervalSeconds)
	}
	if in.Enabled == nil || *in.Enabled != true {
		t.Fatalf("enabled not preserved: %+v", in.Enabled)
	}
	if in.MaxItemsPerRun != cur.MaxItemsPerRun {
		t.Fatalf("max items drifted: %d", in.MaxItemsPerRun)
	}
	_ = disabled

	// 局部 PATCH：仅指名字段改写；nil 字段仍继承。
	newName := "renamed"
	newInterval := 3600
	in = mergeConnectorBody(sourceConnectorBody{Name: &newName, IntervalSeconds: &newInterval}, cur)
	if in.Name != "renamed" || in.IntervalSeconds != 3600 {
		t.Fatalf("named fields not applied: %+v", in)
	}
	if in.ConfigJSON != cur.ConfigJSON || in.CollectionID != cur.CollectionID {
		t.Fatalf("unset fields drifted: %+v", in)
	}

	// 显式禁能：*false 透传（与「未提供」区分）。
	in = mergeConnectorBody(sourceConnectorBody{Enabled: &disabled}, cur)
	if in.Enabled == nil || *in.Enabled {
		t.Fatalf("explicit disable lost: %+v", in.Enabled)
	}

	// config_json 提供空对象 = 未提供（不覆盖既有）；非空则替换。
	in = mergeConnectorBody(sourceConnectorBody{ConfigJSON: json.RawMessage("{}")}, cur)
	if in.ConfigJSON != cur.ConfigJSON {
		t.Fatalf("empty object should inherit: %q", in.ConfigJSON)
	}
	in = mergeConnectorBody(sourceConnectorBody{ConfigJSON: json.RawMessage(`{"path":"/new"}`)}, cur)
	if in.ConfigJSON != `{"path":"/new"}` {
		t.Fatalf("explicit config not applied: %q", in.ConfigJSON)
	}
}

// ── queryLimit 钳制 ──────────────────────────────────────────────────────

func TestQueryLimit_Grading(t *testing.T) {
	mk := func(q string) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/v1/knowledge/connectors/x/runs"+q, nil)
	}
	if got := queryLimit(mk(""), 50); got != 50 {
		t.Fatalf("default: got %d", got)
	}
	if got := queryLimit(mk("?limit=200"), 50); got != 200 {
		t.Fatalf("pass-through: got %d", got)
	}
	for _, bad := range []string{"?limit=0", "?limit=-3", "?limit=abc", "?limit="} {
		if got := queryLimit(mk(bad), 50); got != 50 {
			t.Fatalf("bad %q: got %d, want default 50", bad, got)
		}
	}
}

// ── 审计条目形状（③ "<verb>.source_connector"；summary 绝不带 secret 由
//    调用方保证——本钉只锁 Action/Resource 规范形，warn 级走 MonitorUsecase 内部） ──

func TestBizMonitorEntry_ActionShape(t *testing.T) {
	e := bizMonitorEntry("credentials", "ss:conn:1", "upsert key=GITHUB_TOKEN")
	if e.Action != "credentials.source_connector" {
		t.Fatalf("action: got %q", e.Action)
	}
	if e.Resource != "source_connector" || e.ResourceID != "ss:conn:1" {
		t.Fatalf("resource pair: %+v", e)
	}
	// ③ 铁律：summary 只含 credential_key，secret 永不落审计。
	if strings.Contains(e.Summary, "ghp_") || strings.Contains(strings.ToLower(e.Summary), "secret=") {
		t.Fatalf("summary leaks secret material: %q", e.Summary)
	}
}

// ── view 形状：timeStr 零值空串 + RFC3339 输出 ──────────────────────────

func TestSourceSyncViews_TimeEncoding(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 30, 0, 0, time.UTC)
	run := bizsync.Run{ID: "r1", ConnectorID: "c1", StartedAt: now, Status: bizsync.RunStatusSuccess}
	v := runToView(run)
	if v.StartedAt != "2026-10-23T12:30:00Z" {
		t.Fatalf("started_at: got %q", v.StartedAt)
	}
	if v.FinishedAt != "" {
		t.Fatalf("zero finished_at must serialize empty: %q", v.FinishedAt)
	}
	item := bizsync.Item{ConnectorID: "c1", ExternalID: "a.md", FirstSeenAt: now}
	iv := itemToView(item)
	if iv.FirstSeenAt != "2026-10-23T12:30:00Z" || iv.LastSeenAt != "" {
		t.Fatalf("item time encoding: %+v", iv)
	}
}

// ── decodeSourceBody 严格解码（未知字段拒） ───────────────────────────────

func TestDecodeSourceBody_Strict(t *testing.T) {
	svc := NewKnowledgeService(nil, nil, KnowledgeSearchDeps{}, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	admin := knowledgesync.NewAdminService(knowledgesync.AdminDeps{Log: loggateway.NewNoop()})
	svc.SetSourceSyncAdmin(admin)

	// POST 带未知字段 → 400（custom JSON 契约无向后兼容包袱）。
	body := `{"kind":"shared_dir","name":"x","unknown_field":1}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/knowledge/connectors", bytes.NewBufferString(body))
	svc.ServeSourceConnectors(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: got %d, want 400", rec.Code)
	}

	// 非法 JSON → 400。
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/knowledge/connectors", bytes.NewBufferString(`{`))
	svc.ServeSourceConnectors(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed: got %d, want 400", rec.Code)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────

// biz.MonitorUsecase 引用保真（编译期锁 recordAudit 形参未漂移）。
var _ = biz.AdminAuditEntry{}
