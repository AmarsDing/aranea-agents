package service

// twin_openapi_security_test.go — P0 机器面安全件测试（2026-11-02）：
// 多调用方注册表、常量时间认证、webhook SSRF 双闸、租户读写面门禁、
// run 级租户反查、幂等键 caller 隔离与重放命中。

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// caller 注册表
// ---------------------------------------------------------------------------

func TestLoadTwinCallers(t *testing.T) {
	lg := loggateway.NewNoop()

	t.Run("空环境返回空注册表", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "")
		t.Setenv(TwinOpenAPICallersEnv, "")
		if got := loadTwinCallers(lg); len(got) != 0 {
			t.Fatalf("want empty, got %d callers", len(got))
		}
	})

	t.Run("存量单 token 注册为系统级 twinmonitor", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "legacy-tok")
		t.Setenv(TwinOpenAPICallersEnv, "")
		got := loadTwinCallers(lg)
		if len(got) != 1 || got[0].Name != twinLegacyCallerName || !got[0].isSystem() {
			t.Fatalf("legacy caller wrong: %+v", got)
		}
	})

	t.Run("JSON 注册表解析（含租户）", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "")
		t.Setenv(TwinOpenAPICallersEnv, `[
			{"name":"hotcraftflow","token":"tok-hcf","workspace_id":"ws-hcf"},
			{"name":"novatrader","token":"tok-nt","workspace_id":"ws-nt"}
		]`)
		got := loadTwinCallers(lg)
		if len(got) != 2 || got[0].Name != "hotcraftflow" || got[0].WorkspaceID != "ws-hcf" {
			t.Fatalf("parsed callers wrong: %+v", got)
		}
		if got[0].isSystem() {
			t.Fatal("tenant caller must not be system")
		}
	})

	t.Run("非法条目跳过（空 token / 重名），合法条目保留", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "")
		t.Setenv(TwinOpenAPICallersEnv, `[
			{"name":"a","token":"tok-a"},
			{"name":"b","token":""},
			{"name":"a","token":"tok-a2"}
		]`)
		got := loadTwinCallers(lg)
		if len(got) != 1 || got[0].Token != "tok-a" {
			t.Fatalf("want only first valid entry, got %+v", got)
		}
	})

	t.Run("JSON 解析失败整段忽略，存量 token 兜底", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "legacy-tok")
		t.Setenv(TwinOpenAPICallersEnv, `{not json`)
		got := loadTwinCallers(lg)
		if len(got) != 1 || got[0].Name != twinLegacyCallerName {
			t.Fatalf("bad json should fall back to legacy only, got %+v", got)
		}
	})

	t.Run("CALLERS 显式定义 twinmonitor 时存量 token 不重复注册", func(t *testing.T) {
		t.Setenv(TwinOpenAPITokenEnv, "legacy-tok")
		t.Setenv(TwinOpenAPICallersEnv, `[{"name":"twinmonitor","token":"explicit-tok"}]`)
		got := loadTwinCallers(lg)
		if len(got) != 1 || got[0].Token != "explicit-tok" {
			t.Fatalf("explicit caller should win over legacy, got %+v", got)
		}
	})
}

// TestTwinGuardMultiCaller 多调用方认证：合法 token 命中对应 caller 入上下文。
func TestTwinGuardMultiCaller(t *testing.T) {
	s := newTwinFacadeForTest("legacy-tok")
	s.sec.callers = append(s.sec.callers,
		&twinCaller{Name: "hotcraftflow", Token: "tok-hcf", WorkspaceID: "ws-hcf"})

	var gotName, gotWs string
	h := s.guard(func(w http.ResponseWriter, r *http.Request) {
		c := twinCallerFrom(r.Context())
		gotName, gotWs = c.Name, c.WorkspaceID
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer tok-hcf")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("tenant token should authenticate, got %d", rec.Code)
	}
	if gotName != "hotcraftflow" || gotWs != "ws-hcf" {
		t.Fatalf("caller context wrong: name=%q ws=%q", gotName, gotWs)
	}

	// 未知 token 拒绝。
	req = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer tok-unknown")
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown token should be 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// webhook SSRF 双闸
// ---------------------------------------------------------------------------

func TestWebhookAllowPolicy(t *testing.T) {
	lg := loggateway.NewNoop()

	t.Run("默认档：提交闸放行 http(s)，拨号闸仅内网", func(t *testing.T) {
		t.Setenv(TwinOpenAPIWebhookAllowEnv, "")
		p := loadWebhookAllowPolicy(lg)
		if err := p.allows("http://hotcraftflow.internal:8080/api/v1/webhooks/aranea"); err != nil {
			t.Fatalf("default policy should pass intranet url at submit: %v", err)
		}
		if err := p.allows("ftp://x/hook"); err == nil {
			t.Fatal("non-http(s) scheme must be rejected at submit")
		}
		if err := p.allows("http://"); err == nil {
			t.Fatal("empty host must be rejected")
		}
		// 拨号闸：内网/环回放行，公网/链路本地（云元数据）拒绝。
		for ip, want := range map[string]bool{
			"192.168.4.44":    true,
			"10.0.0.5":        true,
			"127.0.0.1":       true,
			"8.8.8.8":         false,
			"169.254.169.254": false,
		} {
			if got := p.dialAllowed(net.ParseIP(ip)); got != want {
				t.Fatalf("dialAllowed(%s) = %v, want %v", ip, got, want)
			}
		}
	})

	t.Run("严格白名单档：host 精确命中与 CIDR", func(t *testing.T) {
		t.Setenv(TwinOpenAPIWebhookAllowEnv, "ok.internal, host:port.example:8443, 10.0.0.0/8")
		p := loadWebhookAllowPolicy(lg)
		if !p.strict {
			t.Fatal("entries should enable strict mode")
		}
		if err := p.allows("http://ok.internal/hook"); err != nil {
			t.Fatalf("listed host should pass: %v", err)
		}
		if err := p.allows("http://host:port.example:8443/hook"); err != nil {
			t.Fatalf("listed host:port should pass: %v", err)
		}
		if err := p.allows("http://10.1.2.3:9090/hook"); err != nil {
			t.Fatalf("CIDR-covered ip should pass: %v", err)
		}
		if err := p.allows("http://evil.internal/hook"); err == nil {
			t.Fatal("unlisted host must be rejected")
		}
		if err := p.allows("http://11.0.0.1/hook"); err == nil {
			t.Fatal("ip outside CIDR must be rejected")
		}
	})

	t.Run("显式 * 放开", func(t *testing.T) {
		t.Setenv(TwinOpenAPIWebhookAllowEnv, "*")
		p := loadWebhookAllowPolicy(lg)
		if !p.allowAll {
			t.Fatal("star entry should set allowAll")
		}
		if err := p.allows("http://anything.example/hook"); err != nil {
			t.Fatalf("allowAll should pass submit: %v", err)
		}
		if !p.dialAllowed(net.ParseIP("8.8.8.8")) {
			t.Fatal("allowAll should pass dial")
		}
	})
}

// ---------------------------------------------------------------------------
// 租户读写面门禁（纯函数矩阵）
// ---------------------------------------------------------------------------

func TestTwinGraphTenantAccess(t *testing.T) {
	system := &twinCaller{Name: "twinmonitor"}
	tenantA := &twinCaller{Name: "hcf", WorkspaceID: "ws-a"}
	shared := &biz.GraphDefinition{ID: "g-shared", WorkspaceID: ""}
	own := &biz.GraphDefinition{ID: "g-own", WorkspaceID: "ws-a"}
	foreign := &biz.GraphDefinition{ID: "g-foreign", WorkspaceID: "ws-b"}

	cases := []struct {
		name              string
		def               *biz.GraphDefinition
		caller            *twinCaller
		wantRead, wantWrt bool
	}{
		{"系统级读共享", shared, system, true, true},
		{"系统级读它租户", foreign, system, true, true},
		{"租户读共享", shared, tenantA, true, false},
		{"租户读自有", own, tenantA, true, true},
		{"租户读它租户", foreign, tenantA, false, false},
		{"nil 定义", nil, tenantA, false, false},
		{"nil 定义系统级", nil, system, false, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := twinGraphReadable(tt.def, tt.caller); got != tt.wantRead {
				t.Fatalf("readable = %v, want %v", got, tt.wantRead)
			}
			if got := twinGraphWritable(tt.def, tt.caller); got != tt.wantWrt {
				t.Fatalf("writable = %v, want %v", got, tt.wantWrt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// run 面：租户反查 + 幂等 + webhook 提交闸
// ---------------------------------------------------------------------------

// twinFakeIdemStore 幂等存储假实现（按 caller|key 隔离）。
type twinFakeIdemStore struct {
	rows   map[string]string
	saved  []string
	looked []string
}

func (f *twinFakeIdemStore) LookupRunID(_ context.Context, caller, key string) (string, error) {
	f.looked = append(f.looked, caller+"|"+key)
	return f.rows[caller+"|"+key], nil
}

func (f *twinFakeIdemStore) SaveRunID(_ context.Context, caller, key, runID, _ string) error {
	f.saved = append(f.saved, caller+"|"+key+"|"+runID)
	f.rows[caller+"|"+key] = runID
	return nil
}

// newTwinRunFacadeForTest 构造带真实 GraphUsecase（idor stub 仓储）的门面。
func newTwinRunFacadeForTest(callers []*twinCaller, idem biz.TwinIdempotencyStore) *TwinOpenAPICompatService {
	repo := &idorGraphRepo{defs: map[string]*biz.GraphDefinition{
		"graph-own-a":  {ID: "graph-own-a", Name: "own", WorkspaceID: "ws-a"},
		"graph-own-b":  {ID: "graph-own-b", Name: "foreign", WorkspaceID: "ws-b"},
		"graph-shared": {ID: "graph-shared", Name: "shared", WorkspaceID: ""},
	}}
	runRepo := &idorGraphRunRepo{execs: map[string]*biz.GraphExecution{
		"exec-a1": biz.NewGraphExecution(context.Background(), "exec-a1", "graph-own-a", "sess-1", string(biz.GraphExecRunning)),
		"exec-b1": biz.NewGraphExecution(context.Background(), "exec-b1", "graph-own-b", "sess-2", string(biz.GraphExecRunning)),
	}}
	uc := biz.NewGraphUsecase(biz.GraphUsecaseDeps{Repo: repo, RunRepo: runRepo, Lg: loggateway.NewNoop()})
	return &TwinOpenAPICompatService{
		graphs:  uc,
		lg:      twinNoopLogger{},
		sec:     &twinSecurity{callers: callers, idem: idem},
		subs:    make(map[string]*twinRunSub),
		idempot: make(map[string]string),
	}
}

var twinTenantA = &twinCaller{Name: "hcf", Token: "tok-a", WorkspaceID: "ws-a"}
var twinTenantB = &twinCaller{Name: "nt", Token: "tok-b", WorkspaceID: "ws-b"}

func doTwinRequest(s *TwinOpenAPICompatService, method, target, token, body string, pathVals map[string]string) *httptest.ResponseRecorder {
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, rdr)
	for k, v := range pathVals {
		req.SetPathValue(k, v)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestTwinRunTenantGuards(t *testing.T) {
	s := newTwinRunFacadeForTest([]*twinCaller{twinTenantA, twinTenantB}, nil)
	s.routes()

	t.Run("GetRun 它租户 404", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodGet, "/api/v1/runs/exec-a1", "tok-b", "", map[string]string{"id": "exec-a1"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant run must 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GetRun 自有 200", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodGet, "/api/v1/runs/exec-a1", "tok-a", "", map[string]string{"id": "exec-a1"})
		if rec.Code != http.StatusOK {
			t.Fatalf("own run should be 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var view map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatalf("view unmarshal: %v", err)
		}
		if view["run_id"] != "exec-a1" || view["graph_id"] != "graph-own-a" {
			t.Fatalf("view ids wrong: %v", view)
		}
	})

	t.Run("CancelRun 它租户 404", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodPost, "/api/v1/runs/exec-a1/cancel", "tok-b", "", map[string]string{"id": "exec-a1"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant cancel must 404, got %d", rec.Code)
		}
	})

	t.Run("ListRuns 缺 graph_id 400", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodGet, "/api/v1/runs", "tok-a", "", nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("missing graph_id must 400, got %d", rec.Code)
		}
	})

	t.Run("ListRuns 它租户图 404", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodGet, "/api/v1/runs?graph_id=graph-own-a", "tok-b", "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant graph list must 404, got %d", rec.Code)
		}
	})

	t.Run("ListRuns 自有图 200", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodGet, "/api/v1/runs?graph_id=graph-own-a", "tok-a", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("own graph list should be 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var out struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("list unmarshal: %v", err)
		}
	})

	t.Run("共享图对租户不可执行（写面仅自有）", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodPost, "/api/v1/runs", "tok-a",
			`{"graph_id":"graph-shared","input":"hi"}`, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("shared graph run by tenant must 404, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("CreateRun 它租户图 404", func(t *testing.T) {
		rec := doTwinRequest(s, http.MethodPost, "/api/v1/runs", "tok-b",
			`{"graph_id":"graph-own-a","input":"hi"}`, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("cross-tenant create run must 404, got %d", rec.Code)
		}
	})
}

// TestTwinCreateRunIdempotentReplay 幂等键命中：直接返回原 run_id，
// 不触达图执行；键空间按 caller 隔离。
func TestTwinCreateRunIdempotentReplay(t *testing.T) {
	idem := &twinFakeIdemStore{rows: map[string]string{
		"hcf|k-1": "run-existing",
	}}
	s := newTwinRunFacadeForTest([]*twinCaller{twinTenantA, twinTenantB}, idem)
	s.routes()

	rec := doTwinRequest(s, http.MethodPost, "/api/v1/runs", "tok-a",
		`{"graph_id":"graph-own-a","idempotency_key":"k-1"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay should be 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("replay unmarshal: %v", err)
	}
	if out["run_id"] != "run-existing" || out["idempotent_replay"] != true {
		t.Fatalf("replay response wrong: %v", out)
	}
	// caller 隔离：同键它 caller 不命中（会继续走图校验/执行路径）。
	if len(idem.looked) != 1 || idem.looked[0] != "hcf|k-1" {
		t.Fatalf("lookup key should be caller-scoped, got %v", idem.looked)
	}
}

// TestTwinCreateRunWebhookSubmitGate webhook 提交闸：严格白名单下
// 未列名地址 400（在图校验通过之后、执行启动之前）。
func TestTwinCreateRunWebhookSubmitGate(t *testing.T) {
	s := newTwinRunFacadeForTest([]*twinCaller{twinTenantA}, nil)
	s.sec.webhook = &webhookAllowPolicy{strict: true, hosts: map[string]bool{"ok.internal": true}}
	s.routes()

	rec := doTwinRequest(s, http.MethodPost, "/api/v1/runs", "tok-a",
		`{"graph_id":"graph-own-a","webhook_url":"http://evil.internal/hook"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unlisted webhook must 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	// 非 http(s) 方案在默认档同样 400。
	s2 := newTwinRunFacadeForTest([]*twinCaller{twinTenantA}, nil)
	s2.routes()
	rec = doTwinRequest(s2, http.MethodPost, "/api/v1/runs", "tok-a",
		`{"graph_id":"graph-own-a","webhook_url":"ftp://x/hook"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-http scheme must 400, got %d", rec.Code)
	}
}

// TestTwinMemoryFactCallerAttribution 记忆回写按 caller 归因：
// SourceKind 取 caller 名，metadata 附带 source_caller。
func TestTwinMemoryFactCallerAttribution(t *testing.T) {
	writer := &mfpFakeWriter{}
	s := newTwinFacadeForTest("legacy-tok")
	s.sec.callers = append(s.sec.callers, twinTenantA)
	// factWriter 单依赖即可走通 UpsertFactRow（fake 返回空 raw → 索引同步与
	// 冲突检测早退，不触达 vec/admin）。
	s.memory = biz.NewMemoryAdminUsecase(nil, &biz.MemoryUsecase{}, nil, writer, loggateway.NewNoop())
	s.routes()

	rec := doTwinRequest(s, http.MethodPost, "/api/v1/memory/facts", "tok-a",
		`{"scope":"global","key":"k","content":"v","metadata":{"origin":"agt"}}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("write fact should be 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(writer.upserts) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(writer.upserts))
	}
	up := writer.upserts[0]
	if up.SourceKind != "hcf" {
		t.Fatalf("SourceKind should be caller name, got %q", up.SourceKind)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(up.MetadataJSON), &meta); err != nil {
		t.Fatalf("metadata unmarshal: %v", err)
	}
	if meta["source_caller"] != "hcf" || meta["origin"] != "agt" {
		t.Fatalf("metadata should carry source_caller + caller metadata, got %v", meta)
	}
}
