// Package diagnostics 是运行时体检（doctor）的 biz 聚合面
// （79-runtime-governance R8 / P5.2）：GET /api/v1/admin/diagnostics。
//
// 检查项与 design §9.1 对齐：
//
//	model_providers  模型 provider 可达性（轻量 HEAD ping 刷新 + 目录状态）
//	mcp_servers      MCP server 连接态（metadata_json.health_status）
//	tool_assembly    工具装配对账（C2 单源 = biz.AgentUsecase.ReconcileToolAssembly）
//	memory_stack     记忆写审批 pending 积压（阈值与 audit.py §六同源）
//	cache_baseline   缓存命中率 vs Phase 0 基线漂移（近 24h P50）
//	config_graph     配置图谱健康（C9 聚合；querier 未装配时该 item 缺席）
//
// 每项独立容错：单项依赖错误只把该项置 fail，不影响其余项与整体 200 响应
// （与 P5.1 stats API「降级不 500」一致）。前端「运行时体检」面板按
// detail_ref 跳对应管理页。
//
// 本包独立于 biz 顶层（configgraph 已 import biz，顶层再 import
// configgraph 会成环）；依赖方向 diagnostics → configgraph/usage → biz。
package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/configgraph"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

const (
	KeyModelProviders = "model_providers"
	KeyMCPServers     = "mcp_servers"
	KeyToolAssembly   = "tool_assembly"
	KeyMemoryStack    = "memory_stack"
	KeyCacheBaseline  = "cache_baseline"
	KeyConfigGraph    = "config_graph"
)

const (
	StatusPass = "pass"
	StatusWarn = "warn"
	StatusFail = "fail"
)

// 检查阈值。记忆积压三档与 test/agent-audit/audit.py §六（P3.7）同源；
// 缓存两档相对 Phase 0 基线（2026-08-25 全局 chat 45.1%）：<30% 约为基线
// 2/3（warn），<15% 约为基线 1/3（fail）。
const (
	cacheWindow        = 24 * time.Hour
	cacheMinSamples    = 5
	cacheWarnRatio     = 0.30
	cacheFailRatio     = 0.15
	pendingBacklogWarn = 20
	pendingStaleWarn   = 24 * time.Hour
	pendingStaleFail   = 72 * time.Hour
	mcpListLimit       = 500
	pendingListLimit   = 200
	maxNamesInSummary  = 3
)

// Item 是单项检查结果（design §9.2 契约）。
type Item struct {
	Key       string `json:"key"`
	Status    string `json:"status"` // pass | warn | fail
	Summary   string `json:"summary"`
	DetailRef string `json:"detail_ref"` // 前端管理页路由
}

// Report 是 doctor 全量响应。
type Report struct {
	Items []Item `json:"items"`
}

// 窄接口（CS-B7）：由各域 usecase 直接满足，wire 按依赖注入。

// ProviderModelSource 由 *biz.LlmProviderModelUsecase 满足。
type ProviderModelSource interface {
	// RunHealthChecks 对启用模型发轻量 ping 并刷新 status（best-effort）。
	RunHealthChecks(ctx context.Context) error
	List(ctx context.Context) ([]biz.ProviderModel, error)
}

// MCPServerSource 由 *biz.MCPServerUsecase 满足。
type MCPServerSource interface {
	List(ctx context.Context, q biz.MCPListQuery) ([]biz.MCPServer, error)
}

// ToolAssemblySource 由 *biz.AgentUsecase 满足。
type ToolAssemblySource interface {
	ReconcileToolAssembly(ctx context.Context) (biz.ToolAssemblyReport, error)
}

// CacheStatsSource 由 *usage.Usecase 满足。
type CacheStatsSource interface {
	CacheHitRatioStats(ctx context.Context, window time.Duration) ([]usage.CacheHitRatioStat, error)
}

// ConfigGraphSource 由 *configgraph.Querier 满足。
type ConfigGraphSource interface {
	Health(ctx context.Context) (*configgraph.HealthReport, error)
}

// UsecaseDeps 聚合 doctor 依赖；除 ProviderModels/ToolAssembly 外均可缺省
// （缺省项降级为 pass「未装配」或缺席，不阻断其余检查）。
type UsecaseDeps struct {
	ProviderModels ProviderModelSource
	MCPServers     MCPServerSource
	ToolAssembly   ToolAssemblySource
	MemPending     biz.MemoryFactPendingStore
	CacheStats     CacheStatsSource
	ConfigGraph    ConfigGraphSource
	Lg             loggateway.Logger
	// Now 可注入（测试）；nil 用 time.Now。
	Now func() time.Time
}

// Usecase 是运行时体检聚合 usecase。
type Usecase struct {
	providerModels ProviderModelSource
	mcpServers     MCPServerSource
	toolAssembly   ToolAssemblySource
	memPending     biz.MemoryFactPendingStore
	cacheStats     CacheStatsSource
	configGraph    ConfigGraphSource
	lg             loggateway.Logger
	now            func() time.Time
}

func NewUsecase(d UsecaseDeps) *Usecase {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	lg := d.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Usecase{
		providerModels: d.ProviderModels,
		mcpServers:     d.MCPServers,
		toolAssembly:   d.ToolAssembly,
		memPending:     d.MemPending,
		cacheStats:     d.CacheStats,
		configGraph:    d.ConfigGraph,
		lg:             lg.With(loggateway.Domain("diagnostics")),
		now:            now,
	}
}

// Run 顺序执行全部检查项（各轻量、无交叉依赖，串行足够；config_graph 仅
// 在 querier 装配时追加——C9 聚合对未启用部署透明）。
func (u *Usecase) Run(ctx context.Context) Report {
	items := []Item{
		u.checkModelProviders(ctx),
		u.checkMCPServers(ctx),
		u.checkToolAssembly(ctx),
		u.checkMemoryStack(ctx),
		u.checkCacheBaseline(ctx),
	}
	if u.configGraph != nil {
		items = append(items, u.checkConfigGraph(ctx))
	}
	return Report{Items: items}
}

// ToolAssemblyReport 透出工具装配对账全量明细（issues + 逐 agent 行 +
// dead_tools）。消费方：GET /api/v1/admin/tool-assembly/reconcile ——
// audit.py 离线复算下线后（design §9.1 ADR C2）经该端点取服务侧单源结果。
func (u *Usecase) ToolAssemblyReport(ctx context.Context) (biz.ToolAssemblyReport, error) {
	if u.toolAssembly == nil {
		return biz.ToolAssemblyReport{}, errors.New("diagnostics: 工具装配对账源未装配")
	}
	return u.toolAssembly.ReconcileToolAssembly(ctx)
}

// checkModelProviders：ping 刷新后读目录 status。degraded / 无启用模型 → fail。
func (u *Usecase) checkModelProviders(ctx context.Context) Item {
	item := Item{Key: KeyModelProviders, Status: StatusPass, DetailRef: "/models"}
	if u.providerModels == nil {
		item.Status = StatusFail
		item.Summary = "模型目录源未装配"
		return item
	}
	// best-effort ping：HEAD 探测失败只翻转 status，不阻断读取。
	if err := u.providerModels.RunHealthChecks(ctx); err != nil {
		u.lg.Warn("diagnostics: provider health check refresh failed", loggateway.Err(err))
	}
	models, err := u.providerModels.List(ctx)
	if err != nil {
		item.Status = StatusFail
		item.Summary = "模型目录读取失败: " + err.Error()
		return item
	}
	enabled := 0
	var degraded []string
	for _, m := range models {
		if !m.Enabled || strings.TrimSpace(m.DeletedAt) != "" {
			continue
		}
		enabled++
		if strings.EqualFold(strings.TrimSpace(m.Status), "degraded") {
			degraded = append(degraded, strings.TrimSpace(m.Provider+"/"+m.Model))
		}
	}
	sort.Strings(degraded)
	switch {
	case enabled == 0:
		item.Status = StatusFail
		item.Summary = "无启用状态的模型，对话不可用"
	case len(degraded) > 0:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("%d/%d 启用模型不可达: %s", len(degraded), enabled, joinCapped(degraded))
	default:
		item.Summary = fmt.Sprintf("%d 个启用模型全部可达", enabled)
	}
	return item
}

// checkMCPServers：解析启用服务器 metadata_json.health_status。
// error → fail；auth_required/degraded → warn；unknown（未探测）不扣分。
func (u *Usecase) checkMCPServers(ctx context.Context) Item {
	item := Item{Key: KeyMCPServers, Status: StatusPass, DetailRef: "/mcp-servers"}
	if u.mcpServers == nil {
		item.Summary = "MCP 源未装配"
		return item
	}
	servers, err := u.mcpServers.List(ctx, biz.MCPListQuery{Limit: mcpListLimit})
	if err != nil {
		item.Status = StatusFail
		item.Summary = "MCP 服务器列表读取失败: " + err.Error()
		return item
	}
	enabled := 0
	var errNames, degradedNames []string
	for _, s := range servers {
		if !s.Enabled || strings.TrimSpace(s.DeletedAt) != "" {
			continue
		}
		enabled++
		switch mcpHealthStatusOf(s.MetadataJSON) {
		case "error":
			errNames = append(errNames, s.Name)
		case "auth_required", "degraded":
			degradedNames = append(degradedNames, s.Name)
		}
	}
	sort.Strings(errNames)
	sort.Strings(degradedNames)
	switch {
	case len(errNames) > 0:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("%d/%d 启用服务器连接错误: %s", len(errNames), enabled, joinCapped(errNames))
	case len(degradedNames) > 0:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("%d/%d 启用服务器降级/待授权: %s", len(degradedNames), enabled, joinCapped(degradedNames))
	default:
		item.Summary = fmt.Sprintf("%d 个启用服务器连接正常", enabled)
	}
	return item
}

// mcpHealthStatusOf 读取 metadata_json.health_status（key 与
// internal/mcp/metadata.KeyHealthStatus 对齐；biz 不反向依赖 mcp 包，
// 仅解析该已知 key）。空/非法 JSON 返回 ""。
func mcpHealthStatusOf(metadataJSON string) string {
	var m map[string]any
	if json.Unmarshal([]byte(metadataJSON), &m) != nil {
		return ""
	}
	s, _ := m["health_status"].(string)
	return strings.TrimSpace(s)
}

// checkToolAssembly：HIGH → fail；MID → warn；LOW 及以下不升级。
func (u *Usecase) checkToolAssembly(ctx context.Context) Item {
	item := Item{Key: KeyToolAssembly, Status: StatusPass, DetailRef: "/tools"}
	if u.toolAssembly == nil {
		item.Status = StatusFail
		item.Summary = "工具装配对账源未装配"
		return item
	}
	report, err := u.toolAssembly.ReconcileToolAssembly(ctx)
	if err != nil {
		item.Status = StatusFail
		item.Summary = "工具装配对账失败: " + err.Error()
		return item
	}
	high, mid := 0, 0
	for _, is := range report.Issues {
		switch is.Severity {
		case biz.ToolAssemblySeverityHigh:
			high++
		case biz.ToolAssemblySeverityMid:
			mid++
		}
	}
	switch {
	case high > 0:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("%d HIGH / %d MID 装配问题（%d agents 受检）", high, mid, report.AgentsChecked)
	case mid > 0:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("%d MID 装配问题（%d agents 受检）", mid, report.AgentsChecked)
	default:
		item.Summary = fmt.Sprintf("%d agents 装配无异常", report.AgentsChecked)
	}
	return item
}

// checkMemoryStack：pending 审批积压（阈值与 audit.py §六同源）。计数优先
// 走独立 COUNT 窄能力（P5.2：ListPending newest-first + limit 截断——超
// limit 时总数失真、最老 stale 行被截漏成恒 0 假阴性）；store 无该能力时
// 回落列表口径（截断风险接受，仅旧测试替身/精简装配走到）。
func (u *Usecase) checkMemoryStack(ctx context.Context) Item {
	item := Item{Key: KeyMemoryStack, Status: StatusPass, DetailRef: "/memory"}
	if u.memPending == nil {
		item.Summary = "记忆审批存储未装配（R3 未启用）"
		return item
	}
	if counter, ok := u.memPending.(biz.MemoryFactPendingCounter); ok {
		total, staleWarn, staleFail, err := counter.CountPendingByAge(ctx,
			int64(pendingStaleWarn/time.Second), int64(pendingStaleFail/time.Second), u.now().Unix())
		if err != nil {
			item.Status = StatusFail
			item.Summary = "pending 审批计数失败: " + err.Error()
			return item
		}
		return memoryStackItem(item, total, staleWarn, staleFail)
	}
	rows, err := u.memPending.ListPending(ctx, "", biz.MemoryFactPendingStatusPending, pendingListLimit)
	if err != nil {
		item.Status = StatusFail
		item.Summary = "pending 审批读取失败: " + err.Error()
		return item
	}
	now := u.now().Unix()
	staleWarn, staleFail := 0, 0
	for _, r := range rows {
		age := time.Duration(now-r.CreatedAt) * time.Second
		switch {
		case age > pendingStaleFail:
			staleFail++
		case age > pendingStaleWarn:
			staleWarn++
		}
	}
	return memoryStackItem(item, int64(len(rows)), int64(staleWarn), int64(staleFail))
}

// memoryStackItem 按积压总数与 stale 分档生成检查项（counter/列表两口径
// 共用同一判定阶梯）。
func memoryStackItem(item Item, total, staleWarn, staleFail int64) Item {
	switch {
	case staleFail > 0:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("%d 条记忆写审批积压超 72h（pending 共 %d 条）", staleFail, total)
	case total > pendingBacklogWarn:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("记忆写审批积压 %d 条（>%d）", total, pendingBacklogWarn)
	case staleWarn > 0:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("%d 条记忆写审批积压超 24h（pending 共 %d 条）", staleWarn, total)
	default:
		item.Summary = fmt.Sprintf("记忆写审批 pending %d 条，无积压", total)
	}
	return item
}

// checkCacheBaseline：近 24h P50 命中率对 Phase 0 基线漂移。样本不足
// （<cacheMinSamples）的 (provider,model) 组不参与判定。
func (u *Usecase) checkCacheBaseline(ctx context.Context) Item {
	item := Item{Key: KeyCacheBaseline, Status: StatusPass, DetailRef: "/usage/events"}
	if u.cacheStats == nil {
		item.Summary = "缓存统计源未装配"
		return item
	}
	stats, err := u.cacheStats.CacheHitRatioStats(ctx, cacheWindow)
	if err != nil {
		item.Status = StatusFail
		item.Summary = "缓存命中统计读取失败: " + err.Error()
		return item
	}
	evaluated := 0
	worstRatio := 1.0
	worstGroup := ""
	for _, s := range stats {
		if s.Samples < cacheMinSamples {
			continue
		}
		evaluated++
		if s.P50Ratio < worstRatio {
			worstRatio = s.P50Ratio
			worstGroup = strings.TrimSpace(s.Provider + "/" + s.Model)
		}
	}
	switch {
	case evaluated == 0:
		item.Summary = fmt.Sprintf("近 %v 窗口无可评估样本（≥%d turns/组）", cacheWindow, cacheMinSamples)
	case worstRatio < cacheFailRatio:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("缓存命中率严重偏离基线: %s P50=%.1f%%（基线≈45%%）", worstGroup, worstRatio*100)
	case worstRatio < cacheWarnRatio:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("缓存命中率低于基线: %s P50=%.1f%%（基线≈45%%）", worstGroup, worstRatio*100)
	default:
		item.Summary = fmt.Sprintf("缓存命中率正常（%d 组最差 P50=%.1f%%）", evaluated, worstRatio*100)
	}
	return item
}

// checkConfigGraph：C9 聚合——环/断边 → fail；god node/重复 prompt → warn；
// ErrNotReady（首启未建图）→ warn 而非 fail。
func (u *Usecase) checkConfigGraph(ctx context.Context) Item {
	item := Item{Key: KeyConfigGraph, Status: StatusPass, DetailRef: "/graphs"}
	report, err := u.configGraph.Health(ctx)
	if errors.Is(err, configgraph.ErrNotReady) {
		item.Status = StatusWarn
		item.Summary = "配置图谱尚未构建（首启建图中）"
		return item
	}
	if err != nil {
		item.Status = StatusFail
		item.Summary = "配置图谱健康检查失败: " + err.Error()
		return item
	}
	broken := 0
	for _, g := range report.BrokenByType {
		broken += g.Count
	}
	switch {
	case len(report.Cycles) > 0 || broken > 0:
		item.Status = StatusFail
		item.Summary = fmt.Sprintf("引用环 %d 条 / 断边 %d 条（gen=%d）", len(report.Cycles), broken, report.Generation)
	case len(report.GodNodes) > 0 || len(report.DuplicatePrompts) > 0:
		item.Status = StatusWarn
		item.Summary = fmt.Sprintf("god node %d 个 / 重复 prompt %d 组（gen=%d）", len(report.GodNodes), len(report.DuplicatePrompts), report.Generation)
	default:
		item.Summary = fmt.Sprintf("图谱健康（gen=%d）", report.Generation)
	}
	return item
}

// joinCapped 截断长名单，避免 summary 过长（超出的以 "+N more" 收尾）。
func joinCapped(names []string) string {
	if len(names) <= maxNamesInSummary {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s +%d more", strings.Join(names[:maxNamesInSummary], ", "), len(names)-maxNamesInSummary)
}
