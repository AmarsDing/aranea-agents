// CeoboardService 单元测试(整合 P2-7):可见性裁剪 / 平台 id 公开映射 /
// 事件游标与 filter 哈希 / payload 脱敏 / 审计窗口与单飞导出 / 审计 fail-closed 口径。
// 存储走 storetest(modernc sqlite 纯 Go,无 cgo,F 盘 PG 环境窗口外可跑)。
package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "aranea-agents/api/kratos/ceoboard/v1"
	"aranea-agents/internal/biz/ceoboard"
	"aranea-agents/internal/biz/ceoboard/agg"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/biz/ceoboard/store/storetest"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// 测试声明:一条 viewer 可见(含 nats 节点,subjects 带版本后缀)+ 一条仅 ceo 可见。
const ceoboardTestWorkflowViewer = `
api_version: ceoboard/v1
kind: Workflow
id: wf.test-viewer
name: 测试 viewer 流
biz_line: test
owner: qa
version: 1
permissions: [ceo, viewer]
nodes:
  - id: n1
    name: 下单
    order: 1
    enter: "event(subject='hcf.order.created.v2')"
    exit: "event(subject='hcf.order.done.v2')"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:14222"
      stream: HCF_ORDERS
      subjects: ["hcf.order.created.v2", "hcf.order.done.v2"]
      field_map: { entity_id: order_id }
`

const ceoboardTestWorkflowCEOOnly = `
api_version: ceoboard/v1
kind: Workflow
id: wf.test-ceo
name: 测试 ceo 专属流
biz_line: test
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: n1
    name: 机密节点
    order: 1
    enter: "event(subject='hcf.secret.v1')"
    exit: "event(subject='hcf.secret.v1')"
    source:
      adapter: nats-jetstream
      endpoint: "nats://10.0.0.1:14222"
      stream: HCF_SECRET
      subjects: ["hcf.secret.v1"]
      field_map: { entity_id: id }
`

var ceoboardTestNATSPlatforms = map[string]string{"10.0.0.1:14222": "hcf"}

// newCeoboardTestService 在临时目录 + 内存 sqlite 上装配完整运行时与 BFF。
func newCeoboardTestService(t *testing.T, yamls map[string]string) (*CeoboardService, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	regDir := filepath.Join(dir, "workflows")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range yamls {
		if err := os.WriteFile(filepath.Join(regDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	client, _ := storetest.Open(t)
	st := store.New(client, nil)
	rt, err := ceoboard.NewRuntime(&conf.Ceoboard{
		RegistryDir:   regDir,
		SpoolDir:      filepath.Join(dir, "spool"),
		ArchiveDir:    filepath.Join(dir, "archive"),
		NatsPlatforms: ceoboardTestNATSPlatforms,
	}, st, nil)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	svc := NewCeoboardService(rt, nil)
	if svc == nil {
		t.Fatal("NewCeoboardService 返回 nil")
	}
	return svc, st
}

func viewerCtx() context.Context {
	return auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "user", Roles: []string{"viewer"}})
}

func insertCeoboardEvent(t *testing.T, st *store.Store, uid, platform, subject string, receivedAt time.Time, payload string) {
	t.Helper()
	ok, err := st.InsertEvent(context.Background(), &store.Event{
		UID: uid, Platform: platform, Subject: subject, SubjectKey: subject,
		EventID: uid, OccurredAt: receivedAt, ReceivedAt: receivedAt,
		Payload: payload, PayloadComplete: true,
	})
	if err != nil || !ok {
		t.Fatalf("InsertEvent(%s): ok=%v err=%v", uid, ok, err)
	}
}

// ---------- 纯函数:可见性 ----------

func TestCeoboardVisible(t *testing.T) {
	if visible([]string{"ceo"}, nil) {
		t.Fatal("空角色集不得可见(默认拒绝)")
	}
	if visible(nil, []string{"viewer"}) {
		t.Fatal("未声明 permissions 默认仅 ceo 可见")
	}
	if !visible(nil, []string{"ceo"}) {
		t.Fatal("未声明 permissions 时 ceo 应可见")
	}
	if !visible([]string{"ceo", "oncall"}, []string{"oncall"}) {
		t.Fatal("交集命中应可见")
	}
}

func TestCeoboardFilterOverview(t *testing.T) {
	ov := &agg.Overview{
		Lanes: []agg.LaneView{{
			BizLine: "test",
			Workflows: []agg.WorkflowView{
				{ID: "wf.a", Permissions: []string{"ceo"}},
				{ID: "wf.b", Permissions: []string{"viewer"}},
			},
		}},
		Alerts: []agg.AlertView{
			{ID: "wf/wf.a/n1", Text: "a 的告警"},
			{ID: "wf/wf.b/n1", Text: "b 的告警"},
			{ID: "platform/hcf", Text: "平台级告警"},
		},
	}
	ov.Registry.Loaded = []string{"wf.a", "wf.b"}
	ov.Registry.Skipped = []registry.SkipEntry{{Workflow: "wf.a", Field: "x"}, {Workflow: "wf.b", Field: "y"}}

	filterOverview(ov, []string{"viewer"})

	if len(ov.Lanes) != 1 || len(ov.Lanes[0].Workflows) != 1 || ov.Lanes[0].Workflows[0].ID != "wf.b" {
		t.Fatalf("viewer 应只见 wf.b:%+v", ov.Lanes)
	}
	for _, al := range ov.Alerts {
		if strings.HasPrefix(al.ID, "wf/wf.a") {
			t.Fatalf("不可见工作流的告警应被裁剪:%+v", ov.Alerts)
		}
	}
	if len(ov.Alerts) != 2 {
		t.Fatalf("平台级告警与可见工作流告警应保留:%+v", ov.Alerts)
	}
	if len(ov.Registry.Loaded) != 1 || ov.Registry.Loaded[0] != "wf.b" {
		t.Fatalf("registry.loaded 应同步裁剪:%+v", ov.Registry.Loaded)
	}
	if len(ov.Registry.Skipped) != 1 || ov.Registry.Skipped[0].Workflow != "wf.b" {
		t.Fatalf("registry.skipped 应同步裁剪:%+v", ov.Registry.Skipped)
	}
	if ov.Notice != "" {
		t.Fatal("存在可见工作流时不应给兜底提示")
	}

	// 全不可见 → 兜底提示,不泄露业务数据。
	ov2 := &agg.Overview{Lanes: []agg.LaneView{{BizLine: "t", Workflows: []agg.WorkflowView{{ID: "wf.a", Permissions: []string{"ceo"}}}}}}
	filterOverview(ov2, []string{"viewer"})
	if ov2.Notice == "" {
		t.Fatal("全部不可见时必须给兜底提示")
	}
	if len(ov2.Lanes) != 0 {
		t.Fatal("全部不可见时 lanes 应清空")
	}
}

// ---------- 纯函数:平台 id 公开映射 ----------

func TestCeoboardPlatformIDMapping(t *testing.T) {
	if got := publicPlatformID("hcf"); got != "hotcraft" {
		t.Fatalf("hcf → %s", got)
	}
	if got := publicPlatformID("novatrader"); got != "novatrader" {
		t.Fatalf("非 hcf 原样透出:%s", got)
	}
	if got := eventPlatformID("hotcraft"); got != "hcf" {
		t.Fatalf("hotcraft → %s", got)
	}
	if got := eventPlatformID("aranea"); got != "aranea" {
		t.Fatalf("aranea 原样:%s", got)
	}
}

// ---------- 纯函数:游标编解码 ----------

func TestCeoboardEventCursorCodec(t *testing.T) {
	now := time.Now()
	filter := store.EventFilter{Platform: "hcf", AllowedSubjects: []string{"a", "b"}, From: now.Add(-time.Hour), To: now}
	hash := eventFilterHash(filter)
	raw, err := encodeEventCursor(store.Event{UID: "u1", ReceivedAt: now}, filter, hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := decodeEventCursor(raw)
	if err != nil {
		t.Fatal(err)
	}
	if token.UID != "u1" || token.FilterHash != hash || !token.ReceivedAt.Equal(now) {
		t.Fatalf("游标往返失真:%+v", token)
	}
	if _, err := decodeEventCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("非法 base64 应报错")
	}
	if _, err := decodeEventCursor("e30"); err == nil { // "{}" 缺必填字段
		t.Fatal("缺字段的游标应报错")
	}
	// filter 哈希随筛选条件变化(防翻页中途改筛)。
	filter2 := filter
	filter2.Subject = "a"
	if eventFilterHash(filter2) == hash {
		t.Fatal("改筛后哈希必须变化")
	}
}

// ---------- 纯函数:payload 脱敏 ----------

func TestCeoboardRedactPayload(t *testing.T) {
	var fields []string
	in := map[string]any{
		"order_id": "A1",
		"password": "top-secret",
		"nested":   map[string]any{"token": "t", "ok": 1},
		"list":     []any{map[string]any{"Authorization": "Bearer x"}},
	}
	out := redactPayload(in, "payload", &fields).(map[string]any)
	if out["password"] != "[REDACTED]" {
		t.Fatal("顶层敏感键未脱敏")
	}
	if out["nested"].(map[string]any)["token"] != "[REDACTED]" {
		t.Fatal("嵌套敏感键未脱敏")
	}
	if out["list"].([]any)[0].(map[string]any)["Authorization"] != "[REDACTED]" {
		t.Fatal("数组内敏感键未脱敏(大小写归一)")
	}
	if out["order_id"] != "A1" {
		t.Fatal("非敏感键不得改动")
	}
	want := map[string]bool{"payload.password": true, "payload.nested.token": true, "payload.list[0].Authorization": true}
	if len(fields) != len(want) {
		t.Fatalf("脱敏路径清单不符:%v", fields)
	}
	for _, f := range fields {
		if !want[f] {
			t.Fatalf("意外脱敏路径:%s", f)
		}
	}
}

// ---------- 纯函数:审计窗口 / 健康度折叠 / subject 提取 ----------

func TestCeoboardParseAuditWindow(t *testing.T) {
	now := time.Now()
	from, to, err := parseAuditWindow(nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if d := to.Sub(from); d < 24*time.Hour || d > 24*time.Hour+2*time.Nanosecond {
		t.Fatalf("默认窗口应≈24h:%v", d)
	}
	bad := timestamppb.New(now.Add(-40 * 24 * time.Hour))
	if _, _, err := parseAuditWindow(bad, nil, now); err == nil {
		t.Fatal("超 31d 窗口应拒绝")
	}
	inverted := timestamppb.New(now.Add(time.Hour))
	if _, _, err := parseAuditWindow(inverted, timestamppb.New(now), now); err == nil {
		t.Fatal("from>to 应拒绝")
	}
}

func TestCeoboardWorstHealth(t *testing.T) {
	if worstHealth(v1.Health_HEALTH_GREEN, v1.Health_HEALTH_RED) != v1.Health_HEALTH_RED {
		t.Fatal("red 必须压过 green")
	}
	if worstHealth(v1.Health_HEALTH_YELLOW, v1.Health_HEALTH_GREEN) != v1.Health_HEALTH_YELLOW {
		t.Fatal("yellow 必须压过 green")
	}
	if worstHealth(v1.Health_HEALTH_GRAY, v1.Health_HEALTH_GREEN) != v1.Health_HEALTH_GREEN {
		t.Fatal("green 必须压过 gray(组初始档)")
	}
}

func TestCeoboardNodeSubjectKeys(t *testing.T) {
	n := &registry.Node{
		Enter: "event(subject='hcf.order.created.v2')",
		Exit:  "poll(ok)",
		Source: &registry.Source{
			Adapter:  "nats-jetstream",
			Subjects: []string{"hcf.order.created.v2", "hcf.order.done.v9"},
		},
	}
	keys := nodeSubjectKeys(n)
	// 版本后缀剥离:.v2/.v9 → 稳定键。
	for _, want := range []string{"hcf.order.created", "hcf.order.done"} {
		if !keys[want] {
			t.Fatalf("缺稳定键 %s:%v", want, keys)
		}
	}
	raws := rawNodeSubjects(n)
	if len(raws) != 3 { // source.subjects ×2 + enter 提取 ×1(版本保留,字面值白名单⑦)
		t.Fatalf("raw subject 应为字面值 ×3:%v", raws)
	}
	skippedNode := &registry.Node{Source: &registry.Source{Skipped: true}}
	if rawNodeSubjects(skippedNode) != nil {
		t.Fatal("skipped 源不得产出 subject")
	}
}

// ---------- store 支撑:GetOverview 可见性(经 auth 主体) ----------

func TestCeoboardGetOverviewVisibility(t *testing.T) {
	svc, _ := newCeoboardTestService(t, map[string]string{
		"wf.test-viewer.yaml": ceoboardTestWorkflowViewer,
		"wf.test-ceo.yaml":    ceoboardTestWorkflowCEOOnly,
	})

	full, err := svc.GetOverview(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(full.GetLanes()[0].GetWorkflows()); got != 2 {
		t.Fatalf("无主体(dev/test)应全可见:%d", got)
	}

	viewer, err := svc.GetOverview(viewerCtx(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wfs := viewer.GetLanes()[0].GetWorkflows()
	if len(wfs) != 1 || wfs[0].GetId() != "wf.test-viewer" {
		t.Fatalf("viewer 仅见 wf.test-viewer:%v", wfs)
	}
}

func TestCeoboardGetWorkflowForbidden(t *testing.T) {
	svc, _ := newCeoboardTestService(t, map[string]string{"wf.test-ceo.yaml": ceoboardTestWorkflowCEOOnly})
	if _, err := svc.GetWorkflow(viewerCtx(), &v1.GetWorkflowRequest{Id: "wf.test-ceo"}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("viewer 访问 ceo 专属流应 Forbidden:%v", err)
	}
	if _, err := svc.GetWorkflow(viewerCtx(), &v1.GetWorkflowRequest{Id: "wf.missing"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("不存在的工作流应 NotFound:%v", err)
	}
}

// ---------- store 支撑:事件流水 ----------

func TestCeoboardListEvents(t *testing.T) {
	svc, st := newCeoboardTestService(t, map[string]string{"wf.test-viewer.yaml": ceoboardTestWorkflowViewer})
	now := time.Now()
	insertCeoboardEvent(t, st, "hcf:e1", "hcf", "hcf.order.created.v2", now.Add(-3*time.Minute), `{"order_id":"A1"}`)
	insertCeoboardEvent(t, st, "hcf:e2", "hcf", "hcf.order.created.v2", now.Add(-2*time.Minute), `{"order_id":"A2"}`)
	insertCeoboardEvent(t, st, "hcf:e3", "hcf", "hcf.order.done.v2", now.Add(-time.Minute), `{"order_id":"A3"}`)
	// 白名单外 subject:同平台但当前权限不可见,必须被过滤。
	insertCeoboardEvent(t, st, "hcf:e9", "hcf", "hcf.other.secret", now.Add(-time.Minute), `{}`)

	if _, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("缺 platform 应 BadRequest:%v", err)
	}
	if _, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{Platform: "hotcraft", Limit: 201}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("limit>200 应 BadRequest:%v", err)
	}
	if _, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{Platform: "hotcraft", Subject: "hcf.other.secret"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("白名单外 subject 应 BadRequest:%v", err)
	}

	// 公开平台 id(hotcraft)入参,内部 hcf 查询,输出回映射公开 id。
	page1, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{Platform: "hotcraft", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.GetItems()) != 2 {
		t.Fatalf("白名单内事件应为 2 条(白名单外被滤):%d", len(page1.GetItems()))
	}
	for _, it := range page1.GetItems() {
		if it.GetPlatform() != "hotcraft" {
			t.Fatalf("输出平台 id 应为公开 id hotcraft:%s", it.GetPlatform())
		}
	}
	if page1.GetNextCursor() == "" {
		t.Fatal("满页应给下一页游标")
	}

	page2, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{Platform: "hotcraft", Limit: 2, Cursor: page1.GetNextCursor()})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.GetItems()) != 1 {
		t.Fatalf("第二页应剩 1 条:%d", len(page2.GetItems()))
	}
	if page2.GetNextCursor() != "" {
		t.Fatal("末页不应给游标")
	}

	// 翻页中途改筛 → filter 哈希不符,拒绝。
	if _, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{
		Platform: "hotcraft", Limit: 2, Cursor: page1.GetNextCursor(), Subject: "hcf.order.created.v2",
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("改筛翻页应 BadRequest:%v", err)
	}
	// 伪造游标。
	if _, err := svc.ListEvents(context.Background(), &v1.ListEventsRequest{Platform: "hotcraft", Cursor: "@@@"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("非法游标应 BadRequest:%v", err)
	}
}

func TestCeoboardGetEventRedaction(t *testing.T) {
	svc, st := newCeoboardTestService(t, map[string]string{"wf.test-viewer.yaml": ceoboardTestWorkflowViewer})
	now := time.Now()
	insertCeoboardEvent(t, st, "hcf:sec", "hcf", "hcf.order.created.v2", now,
		`{"order_id":"A1","password":"p","nested":{"token":"t"}}`)
	insertCeoboardEvent(t, st, "hcf:forbidden", "hcf", "hcf.other.secret", now, `{}`)
	insertCeoboardEvent(t, st, "hcf:notjson", "hcf", "hcf.order.created.v2", now, `not-json`)

	detail, err := svc.GetEvent(context.Background(), &v1.GetEventRequest{Uid: "hcf:sec"})
	if err != nil {
		t.Fatal(err)
	}
	if got := detail.GetRedactedFields(); len(got) != 2 || got[0] != "payload.nested.token" || got[1] != "payload.password" {
		t.Fatalf("脱敏字段清单不符(应排序):%v", got)
	}
	m := detail.GetPayload().GetStructValue()
	if m.GetFields()["password"].GetStringValue() != "[REDACTED]" {
		t.Fatal("password 未脱敏")
	}
	if m.GetFields()["order_id"].GetStringValue() != "A1" {
		t.Fatal("order_id 应原样保留")
	}

	if _, err := svc.GetEvent(context.Background(), &v1.GetEventRequest{Uid: "hcf:forbidden"}); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("白名单外事件应 Forbidden:%v", err)
	}
	if _, err := svc.GetEvent(context.Background(), &v1.GetEventRequest{Uid: "hcf:missing"}); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("不存在的事件应 NotFound:%v", err)
	}
	if _, err := svc.GetEvent(context.Background(), &v1.GetEventRequest{Uid: "hcf:notjson"}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("非 JSON payload 应 BadRequest(原 422 归并):%v", err)
	}
}

// ---------- store 支撑:审计与导出 ----------

func TestCeoboardListAudit(t *testing.T) {
	svc, st := newCeoboardTestService(t, map[string]string{"wf.test-viewer.yaml": ceoboardTestWorkflowViewer})
	ctx := context.Background()
	if err := st.AddAudit(ctx, "user:7", "deadletter-replay", "42", false, "intent:准备重放死信"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddAudit(ctx, "dev", "audit-export", "", true, "rows=3"); err != nil {
		t.Fatal(err)
	}

	resp, err := svc.ListAudit(ctx, &v1.ListAuditRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetItems()) != 2 {
		t.Fatalf("默认窗口应见 2 条:%d", len(resp.GetItems()))
	}
	found := false
	for _, it := range resp.GetItems() {
		if it.GetActor() == "user:7" && it.GetAction() == "deadletter-replay" && it.GetTarget() == "42" && !it.GetOk() {
			found = true
		}
	}
	if !found {
		t.Fatalf("审计条目映射失真:%+v", resp.GetItems())
	}
	if _, err := svc.ListAudit(ctx, &v1.ListAuditRequest{Limit: 501}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("limit>500 应 BadRequest:%v", err)
	}
	now := time.Now()
	if _, err := svc.ListAudit(ctx, &v1.ListAuditRequest{
		From: timestamppb.New(now.Add(-40 * 24 * time.Hour)),
	}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("超 31d 窗口应 BadRequest:%v", err)
	}
}

func TestCeoboardExportAuditSingleFlight(t *testing.T) {
	svc, st := newCeoboardTestService(t, map[string]string{"wf.test-viewer.yaml": ceoboardTestWorkflowViewer})
	ctx := viewerCtx()
	if err := st.AddAudit(ctx, "user:7", "deadletter-replay", "42", true, "key=k1"); err != nil {
		t.Fatal(err)
	}

	// 单飞:占住导出闸,并发请求必须 RateLimit。
	svc.exportMu.Lock()
	svc.exportActive = true
	svc.exportMu.Unlock()
	if _, err := svc.ExportAudit(ctx, &v1.ExportAuditRequest{}); !apierror.IsCode(err, apierror.CodeRateLimit) {
		t.Fatalf("导出单飞冲突应 RateLimit:%v", err)
	}
	svc.exportMu.Lock()
	svc.exportActive = false
	svc.exportMu.Unlock()

	resp, err := svc.ExportAudit(ctx, &v1.ExportAuditRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetCsv(), "user:7") || !strings.Contains(resp.GetCsv(), "deadletter-replay") {
		t.Fatalf("CSV 应含审计行:\n%s", resp.GetCsv())
	}
	// 导出本身落审计(actor 由 JWT 主体推导)。
	audits, err := svc.ListAudit(ctx, &v1.ListAuditRequest{Action: "audit-export"})
	if err != nil {
		t.Fatal(err)
	}
	if len(audits.GetItems()) != 1 || audits.GetItems()[0].GetActor() != "user:7" || !audits.GetItems()[0].GetOk() {
		t.Fatalf("导出审计缺失或 actor 失真:%+v", audits.GetItems())
	}
}

// ---------- store 支撑:死信重放审计链(fail-closed 口径) ----------

func TestCeoboardReplayDeadLetterAuditTrail(t *testing.T) {
	svc, _ := newCeoboardTestService(t, map[string]string{"wf.test-viewer.yaml": ceoboardTestWorkflowViewer})
	ctx := viewerCtx()

	// 重放不存在的死信:意图审计必须先落库,执行失败再补失败审计,返回 Conflict。
	if _, err := svc.ReplayDeadLetter(ctx, &v1.ReplayDeadLetterRequest{Id: 999}); !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("重放不存在的死信应 Conflict:%v", err)
	}
	audits, err := svc.ListAudit(ctx, &v1.ListAuditRequest{Action: "deadletter-replay"})
	if err != nil {
		t.Fatal(err)
	}
	var intent, failure bool
	for _, it := range audits.GetItems() {
		if it.GetActor() != "user:7" || it.GetTarget() != "999" {
			continue
		}
		if !it.GetOk() && strings.Contains(it.GetDetail(), "intent") {
			intent = true
		}
		if !it.GetOk() && !strings.Contains(it.GetDetail(), "intent") {
			failure = true
		}
	}
	if !intent || !failure {
		t.Fatalf("意图+失败双审计链不完整:%+v", audits.GetItems())
	}
}

// ---------- 装配灰闸 ----------

func TestNewCeoboardServiceNilRuntime(t *testing.T) {
	if got := NewCeoboardService(nil, nil); got != nil {
		t.Fatal("rt 为 nil 时必须返回 nil(装配层跳过注册)")
	}
}
