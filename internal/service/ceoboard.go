// CeoboardService 实现 CEO 驾驶舱 BFF(整合 P2-7):api/kratos/ceoboard/v1 的
// 12 个 rpc(view 面 8 + ops 面 4),语义逐条移植自 ceo-dashboard/internal/api
// (server.go / readonly.go / platform.go / auth.go)。
//
// 鉴权模型差异(与原系统的对应关系):
//   - 原 sessions+账号名 → aranea P1 JWT(pkg/auth.FromContext);
//   - 原「未配置 users = 全放开」→ 无主体(dev/test 环境中间件放行)= 全可见;
//   - 原 ops 面 in-handler「仅 ceo」→ permission 中间件 ceoboard.ops 硬判
//     (permission_codes.go 已登记 12/12);
//   - 审计 actor:原账号名 → "user:<uid>"(JWT 无姓名 claim;无主体时记 "dev");
//   - 原 422(payload 非 JSON)→ aranea 错误模型无 422,归 BadRequest;
//   - 原导出流式写 HTTP → proto 契约 {string csv},缓冲进 strings.Builder
//     (单飞 429 与 ≤60s 超时口径不变,死线内未完成即整体失败)。
//
// 保留的高危语义:事件 subject 字面值白名单(风险点⑦)、审计前门 fail-closed
// (风险点⑥)、payload UseNumber 保真 + 敏感键脱敏、游标 filter 哈希防翻页改筛。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "aranea-agents/api/kratos/ceoboard/v1"
	"aranea-agents/internal/biz/ceoboard"
	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/agg"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	loggateway "aranea-agents/pkg/loggateway"
)

const ceoboardDomain = "CEOBOARD"

// CeoboardService 驾驶舱 BFF。
type CeoboardService struct {
	v1.UnimplementedCeoboardServiceServer

	rt *ceoboard.Runtime
	lg loggateway.Logger

	exportMu     sync.Mutex
	exportActive bool
}

// NewCeoboardService 装配驾驶舱 service;rt 为 nil 时返回 nil(装配层跳过注册)。
func NewCeoboardService(rt *ceoboard.Runtime, lg loggateway.Logger) *CeoboardService {
	if rt == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &CeoboardService{rt: rt, lg: lg}
}

// WebhookHandler 暴露 webhook 机器入口(custom route 直挂,见 server/http.go;
// X-CEO-Key 共享密钥校验在 receiver 内,原始 body 字节保真)。
func (s *CeoboardService) WebhookHandler(source string) http.HandlerFunc {
	return s.rt.Receiver.Handler(source)
}

// ---------- 主体与可见性 ----------

// principal 取请求主体的角色集;full = 无主体(dev/test 中间件放行,
// 与原 auth-disabled「全放开」分支同义)。
func principal(ctx context.Context) (roles []string, full bool) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, true
	}
	return a.Roles, false
}

// actorOf 推导审计 actor(JWT 无姓名 claim,用稳定 uid;无主体 = dev)。
func actorOf(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok {
		return "user:" + strconv.FormatInt(a.UserID, 10)
	}
	return "dev"
}

// visible 判定工作流对角色集是否可见:声明 permissions ∩ 用户 roles ≠ ∅。
// 声明未配 permissions 视为仅 ceo 可见(安全默认,防漏配导致越权展示)。
func visible(perms []string, roles []string) bool {
	if len(roles) == 0 {
		return false
	}
	if len(perms) == 0 {
		perms = []string{"ceo"}
	}
	for _, p := range perms {
		for _, r := range roles {
			if p == r {
				return true
			}
		}
	}
	return false
}

// canSee 报告当前请求是否可见某工作流;wf 为 nil 时按不存在处理。
func (s *CeoboardService) canSee(ctx context.Context, wfID string) (*registry.Workflow, bool) {
	wf := s.rt.Reg.Get(wfID)
	if wf == nil {
		return nil, false
	}
	roles, full := principal(ctx)
	if full {
		return wf, true
	}
	return wf, visible(wf.Permissions, roles)
}

// filterOverview 服务端权限裁剪:不可见工作流整体移除;全部不可见时给兜底提示(不泄露业务数据)。
func filterOverview(ov *agg.Overview, roles []string) {
	lanes := ov.Lanes[:0]
	visibleIDs := map[string]bool{}
	for _, lane := range ov.Lanes {
		ws := lane.Workflows[:0]
		for _, wv := range lane.Workflows {
			if visible(wv.Permissions, roles) {
				for i := range wv.Nodes {
					filterNodeDrills(&wv.Nodes[i], roles)
				}
				ws = append(ws, wv)
				visibleIDs[wv.ID] = true
			}
		}
		if len(ws) > 0 {
			lane.Workflows = ws
			lanes = append(lanes, lane)
		}
	}
	ov.Lanes = lanes
	// 告警同步裁剪:只保留可见工作流的告警(平台级告警保留——不泄露业务细节)
	filtered := ov.Alerts[:0]
	for _, al := range ov.Alerts {
		if strings.HasPrefix(al.ID, "wf/") {
			parts := strings.SplitN(strings.TrimPrefix(al.ID, "wf/"), "/", 2)
			if len(parts) > 0 && !visibleIDs[parts[0]] {
				continue
			}
		}
		filtered = append(filtered, al)
	}
	ov.Alerts = filtered
	// 注册表报告裁剪:skipped 按工作流可见性过滤(failed 仅文件名,不含业务数据)
	skipped := ov.Registry.Skipped[:0]
	for _, sk := range ov.Registry.Skipped {
		if visibleIDs[sk.Workflow] {
			skipped = append(skipped, sk)
		}
	}
	ov.Registry.Skipped = skipped
	loaded := ov.Registry.Loaded[:0]
	for _, id := range ov.Registry.Loaded {
		if visibleIDs[id] {
			loaded = append(loaded, id)
		}
	}
	ov.Registry.Loaded = loaded

	if len(visibleIDs) == 0 {
		ov.Notice = "当前账号无任何工作流可见权限(可能为权限配置缺失),请联系管理员在声明中补充 permissions 或调整账号角色"
	}
}

// filterNodeDrills 裁剪声明了角色限制的下钻,空 permission 保持公开。
func filterNodeDrills(nv *agg.NodeView, roles []string) {
	drills := nv.Drill[:0]
	for _, d := range nv.Drill {
		if d.Permission == "" || containsStr(roles, d.Permission) {
			drills = append(drills, d)
		}
	}
	nv.Drill = drills
}

func containsStr(ss []string, x string) bool {
	for _, s := range ss {
		if s == x {
			return true
		}
	}
	return false
}

// ---------- 平台 id 公开映射(原 platform.go 逐字) ----------

func publicPlatformID(internal string) string {
	if internal == "hcf" {
		return "hotcraft"
	}
	return internal
}

func eventPlatformID(public string) string {
	if public == "hotcraft" {
		return "hcf"
	}
	return public
}

// ---------- view 面 rpc ----------

// GetOverview 整屏数据(按主体角色做工作流可见性裁剪)。
func (s *CeoboardService) GetOverview(ctx context.Context, _ *emptypb.Empty) (*v1.Overview, error) {
	ov := s.rt.Agg.Build(ctx, time.Now())
	roles, full := principal(ctx)
	if !full {
		filterOverview(ov, roles)
	}
	return mapOverview(ov), nil
}

// GetWorkflow 单工作流(含裁剪后的 drill)。
func (s *CeoboardService) GetWorkflow(ctx context.Context, req *v1.GetWorkflowRequest) (*v1.WorkflowView, error) {
	wf, ok := s.canSee(ctx, req.GetId())
	if !ok {
		if s.rt.Reg.Get(req.GetId()) == nil {
			return nil, apierror.NotFound(ceoboardDomain, "workflow not found: %s", req.GetId())
		}
		return nil, apierror.Forbidden(ceoboardDomain, "无权访问该工作流")
	}
	ov := s.rt.Agg.Build(ctx, time.Now())
	roles, full := principal(ctx)
	for _, lane := range ov.Lanes {
		for _, wv := range lane.Workflows {
			if wv.ID == wf.ID {
				if !full {
					for i := range wv.Nodes {
						filterNodeDrills(&wv.Nodes[i], roles)
					}
				}
				return mapWorkflowView(wv), nil
			}
		}
	}
	return nil, apierror.NotFound(ceoboardDomain, "workflow not aggregated: %s", req.GetId())
}

// GetNode 节点抽屉明细。
func (s *CeoboardService) GetNode(ctx context.Context, req *v1.GetNodeRequest) (*v1.NodeDetail, error) {
	id, nid := req.GetWorkflowId(), req.GetNodeId()
	wf, ok := s.canSee(ctx, id)
	if !ok {
		if s.rt.Reg.Get(id) == nil {
			return nil, apierror.NotFound(ceoboardDomain, "workflow not found: %s", id)
		}
		return nil, apierror.Forbidden(ceoboardDomain, "无权访问该工作流")
	}
	ov := s.rt.Agg.Build(ctx, time.Now())
	roles, full := principal(ctx)
	for _, lane := range ov.Lanes {
		for _, wv := range lane.Workflows {
			if wv.ID != id {
				continue
			}
			for _, nv := range wv.Nodes {
				if nv.ID != nid {
					continue
				}
				if !full {
					filterNodeDrills(&nv, roles)
				}
				d := &v1.NodeDetail{
					Node: mapNodeView(&nv), WorkflowId: wf.ID, WorkflowName: wf.Name,
					AsOf: pbTime(ov.AsOf),
				}
				node := &wf.Nodes[indexOfNode(wf, nid)]
				d.Enter, d.Exit = node.Enter, node.Exit
				// 下钻事件流水按节点声明的 subject 收窄(防同平台其他工作流的事件跨权限泄漏)
				allowed := nodeSubjectKeys(node)
				if nv.Source != nil && nv.Source.RestPath != "" {
					if st := s.rt.Agg.Poller.Lookup(agg.PollKey(id, nid)); st != nil {
						d.PollSnippet = st.RawSnippet
					}
				}
				if nv.Source != nil && nv.Source.Adapter == adapter.KindWebhook && len(allowed) > 0 {
					for _, ev := range s.rt.Receiver.Events(ctx, "aranea", 50) {
						if allowed[adapter.SubjectKey(ev.Event)] {
							d.Events = append(d.Events, mapWebhookEventView(ev))
							if len(d.Events) >= 20 {
								break
							}
						}
					}
				}
				if nv.Source != nil && (nv.Source.Adapter == adapter.KindNATS || nv.Source.Adapter == adapter.KindHybrid) && s.rt.Store != nil && len(allowed) > 0 {
					platform := s.rt.Agg.PlatformOfNodePublic(node.Source.Endpoint)
					keys := make([]string, 0, len(allowed))
					for k := range allowed {
						keys = append(keys, k)
					}
					evs, err := s.rt.Store.RecentEventsBySubjects(ctx, platform, keys, 20)
					if err == nil {
						for _, ev := range evs {
							d.NatsEvents = append(d.NatsEvents, mapEventSummary(ev))
						}
					}
				}
				return d, nil
			}
		}
	}
	return nil, apierror.NotFound(ceoboardDomain, "node not found: %s", nid)
}

func indexOfNode(wf *registry.Workflow, nid string) int {
	for i := range wf.Nodes {
		if wf.Nodes[i].ID == nid {
			return i
		}
	}
	return 0
}

// GetPlatform 平台详情(仅携带当前角色可见的关联工作流)。
func (s *CeoboardService) GetPlatform(ctx context.Context, req *v1.GetPlatformRequest) (*v1.PlatformDetail, error) {
	id := req.GetId()
	ov := s.rt.Agg.Build(ctx, time.Now())
	roles, full := principal(ctx)
	if !full {
		filterOverview(ov, roles)
	}
	for _, platform := range ov.Platforms {
		if publicPlatformID(platform.ID) != id {
			continue
		}
		detail := &v1.PlatformDetail{Platform: mapPlatformView(platform), AsOf: pbTime(ov.AsOf), Workflows: []*v1.WorkflowView{}}
		for _, lane := range ov.Lanes {
			for _, workflow := range lane.Workflows {
				if wf := s.rt.Reg.Get(workflow.ID); wf != nil && s.workflowUsesPlatform(wf, id) {
					detail.Workflows = append(detail.Workflows, mapWorkflowView(workflow))
				}
			}
		}
		return detail, nil
	}
	return nil, apierror.NotFound(ceoboardDomain, "platform not found: %s", id)
}

func (s *CeoboardService) workflowUsesPlatform(wf *registry.Workflow, publicID string) bool {
	internalID := eventPlatformID(publicID)
	for i := range wf.Nodes {
		src := wf.Nodes[i].Source
		if src == nil {
			continue
		}
		platform := ""
		switch src.Adapter {
		case adapter.KindNATS, adapter.KindHybrid:
			platform = s.rt.Agg.PlatformOfNodePublic(src.Endpoint)
		case adapter.KindWebhook:
			platform = "aranea"
		}
		if platform == internalID {
			return true
		}
	}
	return false
}

// ListConsumers NATS consumer 状态(按可见工作流过滤)。
func (s *CeoboardService) ListConsumers(ctx context.Context, req *v1.ListConsumersRequest) (*v1.ListConsumersResponse, error) {
	platform := eventPlatformID(req.GetPlatform())
	workflowFilter, nodeFilter := req.GetWorkflowId(), req.GetNodeId()
	items := []*v1.ConsumerState{}
	for _, state := range s.rt.NATS.ConsumerStates() {
		parts := strings.SplitN(state.NodeKey, "/", 2)
		if len(parts) != 2 || (platform != "" && state.Platform != platform) {
			continue
		}
		if workflowFilter != "" && parts[0] != workflowFilter || nodeFilter != "" && parts[1] != nodeFilter {
			continue
		}
		if _, ok := s.canSee(ctx, parts[0]); !ok {
			continue
		}
		items = append(items, &v1.ConsumerState{
			NodeKey: state.NodeKey, Platform: publicPlatformID(state.Platform),
			Stream: state.Stream, Durable: state.Durable, Subject: state.Subject,
			NumPending: int32(state.NumPending), Connected: state.Connected,
			AsOf: pbTime(state.AsOf), SampleError: state.SampleError,
		})
	}
	return &v1.ListConsumersResponse{Items: items}, nil
}

// GetRegistryReport 注册表加载报告(裁剪后)。
func (s *CeoboardService) GetRegistryReport(ctx context.Context, _ *emptypb.Empty) (*v1.RegistryReport, error) {
	cur := s.rt.Rep.Get()
	roles, full := principal(ctx)
	if full {
		return mapRegistryReport(cur), nil
	}
	// 裁剪:skipped/loaded 只露可见工作流
	rep := *cur
	loaded := []string{}
	for _, id := range rep.Loaded {
		if wf := s.rt.Reg.Get(id); wf != nil && visible(wf.Permissions, roles) {
			loaded = append(loaded, id)
		}
	}
	rep.Loaded = loaded
	skipped := []registry.SkipEntry{}
	for _, sk := range rep.Skipped {
		if wf := s.rt.Reg.Get(sk.Workflow); wf != nil && visible(wf.Permissions, roles) {
			skipped = append(skipped, sk)
		}
	}
	rep.Skipped = skipped
	return mapRegistryReport(&rep), nil
}

// ---------- 事件流水(原 readonly.go) ----------

var (
	eventSubjectRe = regexp.MustCompile(`event\(subject='([^']+)'\)`)
	webhookEventRe = regexp.MustCompile(`webhook\(event='([^']+)'\)`)
)

func rawNodeSubjects(n *registry.Node) []string {
	if n.Source == nil || n.Source.Skipped {
		return nil
	}
	out := append([]string{}, n.Source.Subjects...)
	for _, condition := range []string{n.Enter, n.Exit} {
		if match := eventSubjectRe.FindStringSubmatch(condition); len(match) == 2 {
			out = append(out, match[1])
		}
		if match := webhookEventRe.FindStringSubmatch(condition); len(match) == 2 {
			out = append(out, match[1])
		}
	}
	return out
}

// nodeSubjectKeys 汇总节点声明关联的 subject_key 集合:source.subjects(版本剥离)
// + enter/exit 中 event(subject='...') 的提取。供下钻事件流水按节点收窄,
// 防同平台其他工作流的事件跨权限泄漏(扫漏项)。
func nodeSubjectKeys(n *registry.Node) map[string]bool {
	out := map[string]bool{}
	if n.Source != nil {
		for _, s := range n.Source.Subjects {
			out[adapter.SubjectKey(s)] = true
		}
	}
	for _, cond := range []string{n.Enter, n.Exit} {
		if m := eventSubjectRe.FindStringSubmatch(cond); len(m) == 2 {
			out[adapter.SubjectKey(m[1])] = true
		}
	}
	return out
}

// allowedEventSubjects 从可见工作流声明生成 raw subject 白名单;通配符只作为字面值保留(风险点⑦)。
func (s *CeoboardService) allowedEventSubjects(ctx context.Context, publicPlatform, workflowFilter, nodeFilter string) []string {
	internalPlatform := eventPlatformID(publicPlatform)
	allowed := map[string]bool{}
	for _, wf := range s.rt.Reg.List() {
		if workflowFilter != "" && wf.ID != workflowFilter {
			continue
		}
		if _, ok := s.canSee(ctx, wf.ID); !ok {
			continue
		}
		for i := range wf.Nodes {
			n := &wf.Nodes[i]
			if nodeFilter != "" && n.ID != nodeFilter || n.Source == nil {
				continue
			}
			platform := ""
			switch n.Source.Adapter {
			case adapter.KindNATS, adapter.KindHybrid:
				platform = s.rt.Agg.PlatformOfNodePublic(n.Source.Endpoint)
			case adapter.KindWebhook:
				platform = "aranea"
			}
			if platform != internalPlatform {
				continue
			}
			for _, subject := range rawNodeSubjects(n) {
				allowed[subject] = true
			}
		}
	}
	out := make([]string, 0, len(allowed))
	for subject := range allowed {
		out = append(out, subject)
	}
	sort.Strings(out)
	return out
}

type eventCursorToken struct {
	ReceivedAt time.Time `json:"received_at"`
	UID        string    `json:"uid"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	FilterHash string    `json:"filter_sha256"`
}

type eventFilterIdentity struct {
	Platform        string   `json:"platform"`
	AllowedSubjects []string `json:"allowed_subjects"`
	Subject         string   `json:"subject"`
	EventID         string   `json:"event_id"`
	WorkflowID      string   `json:"workflow_id"`
	NodeID          string   `json:"node_id"`
	From            int64    `json:"from"`
	To              int64    `json:"to"`
}

func eventFilterHash(filter store.EventFilter) string {
	raw, _ := json.Marshal(eventFilterIdentity{
		Platform: filter.Platform, AllowedSubjects: filter.AllowedSubjects, Subject: filter.Subject,
		EventID: filter.EventID, WorkflowID: filter.WorkflowID, NodeID: filter.NodeID,
		From: filter.From.UnixNano(), To: filter.To.UnixNano(),
	})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func encodeEventCursor(ev store.Event, filter store.EventFilter, filterHash string) (string, error) {
	raw, err := json.Marshal(eventCursorToken{
		ReceivedAt: ev.ReceivedAt, UID: ev.UID, From: filter.From, To: filter.To, FilterHash: filterHash,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeEventCursor(raw string) (*eventCursorToken, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("游标编码非法")
	}
	var token eventCursorToken
	if err := json.Unmarshal(decoded, &token); err != nil || token.UID == "" || token.ReceivedAt.IsZero() || token.From.IsZero() || token.To.IsZero() {
		return nil, fmt.Errorf("游标内容非法")
	}
	return &token, nil
}

// ListEvents 事件流水(subject 白名单由可见工作流动态生成)。
func (s *CeoboardService) ListEvents(ctx context.Context, req *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	publicPlatform := req.GetPlatform()
	if publicPlatform == "" {
		return nil, apierror.BadRequest(ceoboardDomain, "platform 必填")
	}
	var cursorToken *eventCursorToken
	if raw := req.GetCursor(); raw != "" {
		token, decodeErr := decodeEventCursor(raw)
		if decodeErr != nil {
			return nil, apierror.BadRequest(ceoboardDomain, "%s", decodeErr.Error())
		}
		cursorToken = token
	}
	now := time.Now()
	// 查询上界为排他;增加 1ns 避免低精度时钟下入库时间与 now 相同而漏掉首屏事件。
	toFallback := now.Add(time.Nanosecond)
	fromFallback := time.Time{}
	if cursorToken != nil {
		toFallback, fromFallback = cursorToken.To, cursorToken.From
	}
	to := toFallback
	if req.To != nil {
		to = req.To.AsTime()
	}
	if fromFallback.IsZero() {
		fromFallback = to.Add(-time.Hour)
	}
	from := fromFallback
	if req.From != nil {
		from = req.From.AsTime()
	}
	if from.After(to) || to.Sub(from) > 7*24*time.Hour {
		return nil, apierror.BadRequest(ceoboardDomain, "时间窗口非法,默认最近 1h,最大 7d")
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 50
	}
	if limit < 0 || limit > 200 {
		return nil, apierror.BadRequest(ceoboardDomain, "limit 必须为 1..200")
	}
	filter := store.EventFilter{
		Platform: eventPlatformID(publicPlatform), AllowedSubjects: s.allowedEventSubjects(ctx, publicPlatform, req.GetWorkflowId(), req.GetNodeId()),
		Subject: req.GetSubject(), EventID: req.GetEventId(), WorkflowID: req.GetWorkflowId(), NodeID: req.GetNodeId(),
		From: from, To: to, Limit: limit,
	}
	if filter.Subject != "" && !containsStr(filter.AllowedSubjects, filter.Subject) {
		return nil, apierror.BadRequest(ceoboardDomain, "subject 必须精确属于当前权限白名单")
	}
	filterHash := eventFilterHash(filter)
	if cursorToken != nil {
		if cursorToken.FilterHash != filterHash {
			return nil, apierror.BadRequest(ceoboardDomain, "游标与当前筛选条件不匹配")
		}
		filter.Cursor = &store.EventCursor{ReceivedAt: cursorToken.ReceivedAt, UID: cursorToken.UID}
	}
	events, err := s.rt.Store.QueryEvents(ctx, filter)
	if err != nil {
		s.lg.Warn("ceoboard 事件查询失败", loggateway.Err(err))
		return nil, apierror.Internal(ceoboardDomain, "事件查询失败")
	}
	items := make([]*v1.EventSummary, 0, len(events))
	for _, ev := range events {
		items = append(items, mapEventSummary(ev))
	}
	nextCursor := ""
	if len(events) == limit {
		nextCursor, err = encodeEventCursor(events[len(events)-1], filter, filterHash)
		if err != nil {
			return nil, apierror.Internal(ceoboardDomain, "游标生成失败")
		}
	}
	return &v1.ListEventsResponse{Items: items, NextCursor: nextCursor, AsOf: pbTime(now)}, nil
}

// GetEvent 事件详情(payload 脱敏)。
// getEventDetail 是 GetEvent 的共享实现(gRPC GetEvent 与 D2 自定义查询参数
// 路由 GetEventByUIDHTTP 同体;uid 为完整 event_uid,不受路径段字符约束)。
func (s *CeoboardService) getEventDetail(ctx context.Context, uid string) (*v1.EventDetail, error) {
	ev, err := s.rt.Store.GetEvent(ctx, uid)
	if ent.IsNotFound(err) {
		return nil, apierror.NotFound(ceoboardDomain, "event not found")
	}
	if err != nil {
		s.lg.Warn("ceoboard 事件详情查询失败", loggateway.Err(err))
		return nil, apierror.Internal(ceoboardDomain, "事件详情查询失败")
	}
	allowed := s.allowedEventSubjects(ctx, publicPlatformID(ev.Platform), "", "")
	if !containsStr(allowed, ev.Subject) {
		return nil, apierror.Forbidden(ceoboardDomain, "无权访问该事件")
	}
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(ev.Payload))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, apierror.BadRequest(ceoboardDomain, "事件 payload 非 JSON,无法安全脱敏")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, apierror.BadRequest(ceoboardDomain, "事件 payload 含多余 JSON 或尾部数据")
	}
	redactedFields := []string{}
	payload := redactPayload(decoded, "payload", &redactedFields)
	sort.Strings(redactedFields)
	// structpb 数字只有 double;经 JSON 往返归一(契约层既定偏差,P2-1 拍板 Value)。
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, apierror.Internal(ceoboardDomain, "payload 序列化失败")
	}
	pv := &structpb.Value{}
	if err := pv.UnmarshalJSON(raw); err != nil {
		return nil, apierror.Internal(ceoboardDomain, "payload 转换失败")
	}
	return &v1.EventDetail{Summary: mapEventSummary(*ev), Payload: pv, RedactedFields: redactedFields}, nil
}

func (s *CeoboardService) GetEvent(ctx context.Context, req *v1.GetEventRequest) (*v1.EventDetail, error) {
	return s.getEventDetail(ctx, req.GetUid())
}

// GetEventByUIDHTTP 是 GET /v1/ceoboard/event?uid=<event_uid> 的自定义路由
// 处理函数(评审 2026-11-09 D2):proto 路由 GetEvent 的 {uid} 是路径段,event_uid
// 含 '/'、'?' 等保留字符时无法经 HTTP 网关寻址,故提供查询参数出口。响应体与
// proto EventDetail 的 protojson 形态一致(前端可按同一契约解析)。
//
// 鉴权:本路由不在 permission 注册表(注册表键必须存在于 proto,arch 测试
// TestRegisteredKeysExistInProto 硬约束),故 ceoboard.view 在 handler 内硬判,
// 语义对齐中间件顺序——有主体必须持 ceoboard.view;无主体仅 dev/test 放行
// (与 Permission 中间件的 carve-out 对齐,生产无凭据请求在认证过滤器已被拒)。
func (s *CeoboardService) GetEventByUIDHTTP(w http.ResponseWriter, r *http.Request) {
	uid := r.URL.Query().Get("uid")
	if uid == "" {
		writeCeoboardHTTPError(w, http.StatusBadRequest, "uid query parameter is required")
		return
	}
	if a, ok := auth.FromContext(r.Context()); ok && a != nil {
		if !a.HasPermission("ceoboard.view") {
			writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
			return
		}
	} else if !auth.DevOrTestEnv() {
		writeCeoboardHTTPError(w, http.StatusForbidden, "ceoboard.view required")
		return
	}
	detail, err := s.getEventDetail(r.Context(), uid)
	if err != nil {
		switch {
		case apierror.IsCode(err, apierror.CodeNotFound):
			writeCeoboardHTTPError(w, http.StatusNotFound, "event not found")
		case apierror.IsCode(err, apierror.CodeForbidden):
			writeCeoboardHTTPError(w, http.StatusForbidden, "无权访问该事件")
		case apierror.IsCode(err, apierror.CodeBadRequest):
			writeCeoboardHTTPError(w, http.StatusBadRequest, "事件 payload 非 JSON,无法安全脱敏")
		default:
			writeCeoboardHTTPError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	raw, err := protojson.Marshal(detail)
	if err != nil {
		writeCeoboardHTTPError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func writeCeoboardHTTPError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func sensitivePayloadKey(key string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(key), "-", "_")
	switch normalized {
	case "password", "pass", "token", "secret", "authorization", "api_key":
		return true
	default:
		return false
	}
}

func redactPayload(value any, path string, fields *[]string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if sensitivePayloadKey(key) {
				typed[key] = "[REDACTED]"
				*fields = append(*fields, childPath)
				continue
			}
			typed[key] = redactPayload(child, childPath, fields)
		}
	case []any:
		for i, child := range typed {
			typed[i] = redactPayload(child, fmt.Sprintf("%s[%d]", path, i), fields)
		}
	}
	return value
}

// ---------- ops 面 rpc(permission 中间件 ceoboard.ops 硬判;审计口径不变) ----------

// ListDeadLetters 死信列表。
// 评审 2026-11-09 D3:列表端不回传 payload 原文——死信原文是未脱敏的消息字节
// (可能含密钥/个人信息),列表是批量面,逐条原文既放大泄漏面也无浏览价值;
// 列表只保留定位要素(kind/source/ref_key/reason/failed_at)。原文仅两条合规
// 出口:① 重放(ReplayDeadLetter 经 store.GetDeadLetter 取原文重投,不回传);
// ② 直查数据库(库层权限)。proto 字段保留(契约兼容),列表侧恒置空。
func (s *CeoboardService) ListDeadLetters(ctx context.Context, req *v1.ListDeadLettersRequest) (*v1.ListDeadLettersResponse, error) {
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 200
	}
	if limit < 0 || limit > 200 {
		return nil, apierror.BadRequest(ceoboardDomain, "limit 必须为 1..200")
	}
	dls, err := s.rt.Store.ListDeadLetters(ctx, limit)
	if err != nil {
		s.lg.Warn("ceoboard 死信列表查询失败", loggateway.Err(err))
		return nil, apierror.Internal(ceoboardDomain, "死信列表查询失败")
	}
	pending, _ := s.rt.Store.PendingDeadLetters(ctx)
	items := make([]*v1.DeadLetter, 0, len(dls))
	for _, dl := range dls {
		item := mapDeadLetter(dl)
		item.Payload = "" // D3:列表端省略原文,见上注释
		items = append(items, item)
	}
	return &v1.ListDeadLettersResponse{Pending: int32(pending), Items: items}, nil
}

// ReplayDeadLetter 死信重放(审计前门 fail-closed,风险点⑥)。
func (s *CeoboardService) ReplayDeadLetter(ctx context.Context, req *v1.ReplayDeadLetterRequest) (*v1.ReplayDeadLetterResponse, error) {
	actor := actorOf(ctx)
	target := strconv.FormatInt(req.GetId(), 10)
	// 执行前门:意图落库失败即拒绝执行(不裸奔)。
	if err := s.rt.Store.AddAudit(ctx, actor, "deadletter-replay", target, false, "intent:准备重放死信"); err != nil {
		s.lg.Warn("ceoboard 审计意图写入失败,重放未执行", loggateway.Err(err))
		return nil, apierror.Unavailable(ceoboardDomain, "审计不可用,操作未执行")
	}
	ev, err := s.rt.Receiver.ReplayDeadLetter(ctx, req.GetId())
	if err != nil {
		s.audit(ctx, actor, "deadletter-replay", target, false, err.Error())
		return nil, apierror.Conflict(ceoboardDomain, "%s", err.Error())
	}
	s.audit(ctx, actor, "deadletter-replay", target, true, "key="+ev.Key)
	return &v1.ReplayDeadLetterResponse{Replayed: true, Key: ev.Key, Deduped: ev.Deduped, Status: ev.Status}, nil
}

// parseAuditWindow 审计窗口:默认最近 24h,最大 31d。
func parseAuditWindow(from, to *timestamppb.Timestamp, now time.Time) (time.Time, time.Time, error) {
	toV := now.Add(time.Nanosecond)
	if to != nil {
		toV = to.AsTime()
	}
	fromV := toV.Add(-24 * time.Hour)
	if from != nil {
		fromV = from.AsTime()
	}
	if fromV.After(toV) || toV.Sub(fromV) > 31*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("审计时间窗口非法,默认最近 24h,最大 31d")
	}
	return fromV, toV, nil
}

// ListAudit 审计查询。
func (s *CeoboardService) ListAudit(ctx context.Context, req *v1.ListAuditRequest) (*v1.ListAuditResponse, error) {
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	if limit < 0 || limit > 500 {
		return nil, apierror.BadRequest(ceoboardDomain, "limit 必须为 1..500")
	}
	from, to, err := parseAuditWindow(req.From, req.To, time.Now())
	if err != nil {
		return nil, apierror.BadRequest(ceoboardDomain, "%s", err.Error())
	}
	items, err := s.rt.Store.ListAuditWindow(ctx, limit, req.GetAction(), from, to)
	if err != nil {
		s.lg.Warn("ceoboard 审计查询失败", loggateway.Err(err))
		return nil, apierror.Internal(ceoboardDomain, "审计查询失败")
	}
	out := make([]*v1.AuditEntry, 0, len(items))
	for _, it := range items {
		out = append(out, mapAuditEntry(it))
	}
	return &v1.ListAuditResponse{Items: out}, nil
}

// ExportAudit 审计导出 CSV(独立只读快照;全服务同时只允许一个导出)。
func (s *CeoboardService) ExportAudit(ctx context.Context, req *v1.ExportAuditRequest) (*v1.ExportAuditResponse, error) {
	now := time.Now()
	from, to, err := parseAuditWindow(req.From, req.To, now)
	if err != nil {
		return nil, apierror.BadRequest(ceoboardDomain, "%s", err.Error())
	}
	maxRows := s.rt.ExportMaxRows
	if maxRows <= 0 || maxRows > 100000 {
		maxRows = 100000
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = maxRows
	}
	if limit < 0 || limit > maxRows {
		return nil, apierror.BadRequest(ceoboardDomain, "limit 必须为 1..%d", maxRows)
	}

	s.exportMu.Lock()
	if s.exportActive {
		s.exportMu.Unlock()
		return nil, apierror.RateLimit(ceoboardDomain, "已有审计导出进行中")
	}
	s.exportActive = true
	s.exportMu.Unlock()
	defer func() {
		s.exportMu.Lock()
		s.exportActive = false
		s.exportMu.Unlock()
	}()

	timeout := s.rt.ExportTimeout
	if timeout <= 0 || timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	ectx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	actor, action := actorOf(ctx), req.GetAction()
	var sb strings.Builder
	rows, truncated, err := s.rt.Store.ExportAudit(ectx, &sb, store.AuditExportFilter{From: from, To: to, Action: action, Limit: limit})
	if err != nil {
		s.audit(ctx, actor, "audit-export", action, false, err.Error())
		return nil, apierror.Unavailable(ceoboardDomain, "审计导出失败")
	}
	// D4:触字节预算的导出以截断标记收尾(CSV 尾部 # truncated 行),
	// 审计 detail 留痕,调用方据此收窄窗口重试。
	s.audit(ctx, actor, "audit-export", action, true, fmt.Sprintf("rows=%d truncated=%v", rows, truncated))
	return &v1.ExportAuditResponse{Csv: sb.String()}, nil
}

// audit 是高危操作审计(尽力而为;前门走 AddAudit 直调)。
func (s *CeoboardService) audit(ctx context.Context, actor, action, target string, ok bool, detail string) {
	if err := s.rt.Store.AddAudit(ctx, actor, action, target, ok, detail); err != nil {
		s.lg.Warn("ceoboard 审计写入失败", loggateway.Err(err), loggateway.Str("action", action))
	}
}

// ---------- 视图模型映射(agg/store DTO → proto) ----------

func pbHealth(h string) v1.Health {
	switch h {
	case agg.HealthRed:
		return v1.Health_HEALTH_RED
	case agg.HealthYellow:
		return v1.Health_HEALTH_YELLOW
	case agg.HealthGreen:
		return v1.Health_HEALTH_GREEN
	case agg.HealthGray:
		return v1.Health_HEALTH_GRAY
	default:
		return v1.Health_HEALTH_UNSPECIFIED
	}
}

func pbTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func pbTimePtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func mapOverview(ov *agg.Overview) *v1.Overview {
	out := &v1.Overview{
		AsOf:           pbTime(ov.AsOf),
		Notice:         ov.Notice,
		RegistryNotice: ov.RegistryNotice,
		Registry:       mapRegistryView(&ov.Registry),
	}
	for _, p := range ov.Platforms {
		out.Platforms = append(out.Platforms, mapPlatformView(p))
	}
	for _, al := range ov.Alerts {
		out.Alerts = append(out.Alerts, &v1.AlertView{
			Id: al.ID, Level: pbHealth(al.Level), Target: al.Target, Text: al.Text, Since: pbTime(al.Since),
		})
	}
	out.Approvals = mapApprovalsView(&ov.Approvals)
	for _, lane := range ov.Lanes {
		out.Lanes = append(out.Lanes, mapLaneView(lane))
	}
	return out
}

func mapPlatformView(p agg.PlatformView) *v1.PlatformView {
	return &v1.PlatformView{
		Id: publicPlatformID(p.ID), Name: p.Name, Health: pbHealth(p.Health), Probe: p.Probe,
		LastOkAt: pbTimePtr(p.LastOKAt), LastErr: p.LastErr, LatencyMs: p.LatencyMs,
	}
}

func mapApprovalsView(a *agg.ApprovalsView) *v1.ApprovalsView {
	out := &v1.ApprovalsView{
		Health: pbHealth(a.Health), LastOkAt: pbTimePtr(a.LastOKAt),
		Count: int32(a.Count), Note: a.Note,
	}
	for _, it := range a.Items {
		out.Items = append(out.Items, &v1.ApprovalItem{Lane: it.Lane, Id: it.ID, Title: it.Title, Extra: it.Extra})
	}
	return out
}

// mapLaneView 组健康度 = 组内工作流最差档(原前端 healthRank 铁律的服务端表达)。
func mapLaneView(lane agg.LaneView) *v1.LaneView {
	out := &v1.LaneView{BizLine: lane.BizLine, Collapsible: lane.Collapsible, Health: v1.Health_HEALTH_GRAY}
	for _, wv := range lane.Workflows {
		pb := mapWorkflowView(wv)
		out.Health = worstHealth(out.Health, pb.GetHealth())
		out.Workflows = append(out.Workflows, pb)
	}
	return out
}

func worstHealth(a, b v1.Health) v1.Health {
	rank := func(h v1.Health) int {
		switch h {
		case v1.Health_HEALTH_RED:
			return 0
		case v1.Health_HEALTH_YELLOW:
			return 1
		case v1.Health_HEALTH_GREEN:
			return 2
		default:
			return 3
		}
	}
	if rank(a) <= rank(b) {
		return a
	}
	return b
}

func mapWorkflowView(wv agg.WorkflowView) *v1.WorkflowView {
	out := &v1.WorkflowView{
		Id: wv.ID, Name: wv.Name, BizLine: wv.BizLine, Owner: wv.Owner,
		Version: int32(wv.Version), Permissions: wv.Permissions, Health: pbHealth(wv.Health),
		Alerts: int32(wv.Alerts), Volume_1H: int64(wv.Volume1h),
	}
	for _, nv := range wv.Nodes {
		out.Nodes = append(out.Nodes, mapNodeView(&nv))
	}
	return out
}

func mapNodeView(nv *agg.NodeView) *v1.NodeView {
	out := &v1.NodeView{
		Id: nv.ID, Name: nv.Name, Order: int32(nv.Order), Health: pbHealth(nv.Health), Reason: nv.Reason,
	}
	for _, mv := range nv.Metrics {
		m := &v1.MetricView{
			Id: mv.ID, Unit: mv.Unit, Window: mv.Window, Value: mv.Value,
			Display: mv.Display, Health: pbHealth(mv.Health), Note: mv.Note, Origin: mv.Origin,
		}
		// proto 契约:pending 非空 = [待补](原因文本在 Note;此处同步给占位标记)。
		if mv.Pending {
			m.Pending = mv.Note
			if m.Pending == "" {
				m.Pending = "待补"
			}
		}
		out.Metrics = append(out.Metrics, m)
	}
	if nv.Source != nil {
		out.Source = mapSourceView(nv.Source)
	}
	for _, d := range nv.Drill {
		out.Drill = append(out.Drill, &v1.DrillView{
			Label: d.Label, Type: d.Type, Path: d.Path,
			EmptyState: d.EmptyState, Note: d.Note, Permission: d.Permission,
		})
	}
	return out
}

func mapSourceView(sv *agg.SourceView) *v1.SourceView {
	return &v1.SourceView{
		Adapter: sv.Adapter, RestPath: sv.RestPath, Skipped: sv.Skipped, SkipReason: sv.SkipReason,
		Unwired: sv.Unwired, Origin: sv.Origin, Stream: sv.Stream,
		Consuming: sv.Consuming, Connected: sv.Connected,
		MsgCount: sv.MsgCount, DupCount: sv.DupCount,
		StreamMissing: sv.StreamMissing, StreamOptional: sv.StreamOptional, Backlog: sv.Backlog,
		LastEventAt: pbTimePtr(sv.LastEventAt), AsOf: pbTimePtr(sv.AsOf), LastOkAt: pbTimePtr(sv.LastOKAt),
		LastErr: sv.LastErr, FailCount: int32(sv.FailCount), LatencyMs: sv.LatencyMs,
	}
}

func mapRegistryView(rv *agg.RegistryView) *v1.RegistryReport {
	return &v1.RegistryReport{
		Dir: rv.Dir, LoadedAt: pbTime(rv.LoadedAt), Loaded: rv.Loaded,
		Failed: mapFailEntries(rv.Failed), Skipped: mapSkipEntries(rv.Skipped),
	}
}

func mapRegistryReport(rep *registry.Report) *v1.RegistryReport {
	return &v1.RegistryReport{
		Dir: rep.Dir, LoadedAt: pbTime(rep.LoadedAt), Loaded: rep.Loaded,
		Failed: mapFailEntries(rep.Failed), Skipped: mapSkipEntries(rep.Skipped),
	}
}

func mapFailEntries(in []registry.FailEntry) []*v1.RegistryFailEntry {
	out := make([]*v1.RegistryFailEntry, 0, len(in))
	for _, f := range in {
		out = append(out, &v1.RegistryFailEntry{File: f.File, Errors: f.Errors})
	}
	return out
}

func mapSkipEntries(in []registry.SkipEntry) []*v1.RegistrySkipEntry {
	out := make([]*v1.RegistrySkipEntry, 0, len(in))
	for _, sk := range in {
		out = append(out, &v1.RegistrySkipEntry{Workflow: sk.Workflow, Node: sk.Node, Metric: sk.Metric, Field: sk.Field, Reason: sk.Reason})
	}
	return out
}

func mapEventSummary(ev store.Event) *v1.EventSummary {
	return &v1.EventSummary{
		Uid: ev.UID, Platform: publicPlatformID(ev.Platform), Subject: ev.Subject, SubjectKey: ev.SubjectKey,
		EventId: ev.EventID, EntityId: ev.EntityID,
		OccurredAt: pbTime(ev.OccurredAt), ReceivedAt: pbTime(ev.ReceivedAt),
		ImportedFromSpool: ev.ImportedFromSpool, PayloadComplete: ev.PayloadComplete,
	}
}

func mapWebhookEventView(ev adapter.WebhookEvent) *v1.WebhookEventView {
	out := &v1.WebhookEventView{
		Source: ev.Source, Event: ev.Event, Key: ev.Key, EntityId: ev.EntityID,
		At: pbTime(ev.At), Digest: ev.Digest, Deduped: ev.Deduped, Status: ev.Status, ErrMsg: ev.ErrMsg,
	}
	if len(ev.Fields) > 0 {
		if st, err := structpb.NewStruct(ev.Fields); err == nil {
			out.Fields = st
		}
	}
	return out
}

func mapDeadLetter(dl store.DeadLetter) *v1.DeadLetter {
	return &v1.DeadLetter{
		Id: dl.ID, Kind: dl.Kind, Source: dl.Source, RefKey: dl.RefKey,
		Payload: dl.Payload, Reason: dl.Reason,
		FailedAt: pbTime(dl.FailedAt), ReplayedAt: pbTimePtr(dl.ReplayedAt),
	}
}

func mapAuditEntry(a store.AuditEntry) *v1.AuditEntry {
	return &v1.AuditEntry{
		Id: a.ID, At: pbTime(a.At), Actor: a.Actor, Action: a.Action,
		Target: a.Target, Ok: a.OK, Detail: a.Detail,
	}
}
