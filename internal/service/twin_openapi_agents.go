// Package service — twin_openapi_agents.go
//
// TwinOpenAPI 门面的 Agent 目录/写面处理器（自 twin_openapi_compat.go 拆出，
// AS-COG-01 文件行数治理，2026-11-02）。
// 租户门禁（P0 多调用方改造）：
//   - 读面（GET /agents）：系统级 caller 全量；租户 caller = 共享 + 自有
//     （AgentListQuery.WorkspaceID 既有语义）；
//   - 写面（POST/PUT）：租户 caller 创建强制归属自有 workspace，更新仅允许
//     自有 Agent（不存在/它租户一律 404，不泄漏存在性）。
package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (s *TwinOpenAPICompatService) handleListAgents(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	res, err := s.agents.List(r.Context(), biz.AgentListQuery{Limit: 1000, WorkspaceID: caller.WorkspaceID})
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 注：Agent 清单保持全量返回（不做 ?source= 命名空间过滤）——twin 种子同步
	// 按名匹配的 12 个预设 Agent 实为 aranea 侧 pkg 预设资产，无 twin 标签载体
	// （agents 表无 metadata_json 列，config_json 被 tools/skills 投影占用）；
	// 且 Agent 侧无覆盖写路径，全量可见无正确性风险。命名空间隔离仅落 Graph 侧。
	items := make([]map[string]any, 0, len(res.Items))
	for _, a := range res.Items {
		def := twinAgentDefinition(a)
		// 附带真实生效面（runtime settings 的 tools_allow），供 twinmonitor 漂移
		// 比对「声明 vs 生效」；读失败不阻塞清单返回（降级为缺省空面，会触发
		// 一次漂移更新自愈）。
		if st, serr := s.agents.GetAgentRuntimeSettings(r.Context(), a.ID); serr == nil {
			var allow []string
			if json.Unmarshal([]byte(st.ToolsAllowJSON), &allow) == nil {
				def["tools_allow"] = allow
			}
		}
		items = append(items, map[string]any{
			"id":          a.ID,
			"name":        a.DisplayName,
			"description": a.AgentDescription,
			"definition":  def,
		})
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"items": items})
}

// twinToolKeyAliases 把 TwinMonitor aiops 内部 MCP 工具名（alarm.query 等）
// 映射为 aranea 目录内的 twin_* 工具 key（internal/tools/twinops）。
// 2026-09-01 修复：此前种子同步仅把 tool_whitelist 写入 metadata_json 展示字段，
// 从未落 agent_runtime_settings.tools_allow_json，导致全部预设 Agent 在 aranea 侧
// effective-tools 面为空——spirit 组队无兵可用、tool_search 搜不到 twin_*。
// 未映射的 aiops 工具名（server.exec_command / network.* / db.* / diagnose.* 等
// 进程内 MCP 工具）跳过并记 warn，待 twinops 补齐对应工具后扩表。
var twinToolKeyAliases = map[string][]string{
	"alarm.query":        {"twin_alarm_query"},
	"alarm.get":          {"twin_alarm_get"},
	"alarm.ack":          {"twin_alarm_ack"},
	"alarm.rule_get":     {"twin_alarm_rule_get"},
	"asset.get":          {"twin_device_get", "twin_device_search"},
	"asset.search":       {"twin_device_search"},
	"metric.query":       {"twin_device_metrics"},
	"line.status":        {"twin_line_status"},
	"line.events":        {"twin_line_events"},
	"line.probe":         {"twin_line_probe"},
	"collector.status":   {"twin_collector_status"},
	"inspection.query":   {"twin_inspection_query"},
	"remediation.status": {"twin_remediation_status"},
	"config.diff":        {"twin_config_diff"},
	"config.push":        {"twin_config_push"},
	"config.rollback":    {"twin_config_rollback"},
	// knowledge.search → aranea 自有 knowledge_search（知识检索语义一致）。
	"knowledge.search": {"knowledge_search"},
}

// mapTwinToolWhitelist 转换 aiops 工具白名单为 aranea 工具 key 集（去重、保序）。
func mapTwinToolWhitelist(whitelist []string, lg loggateway.Logger) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(whitelist))
	// datetime 基础时间感知对诊断/巡检类专项必备，随专项工具一并授予。
	seen["datetime"] = true
	out = append(out, "datetime")
	for _, raw := range whitelist {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		mapped, ok := twinToolKeyAliases[key]
		if !ok {
			if lg != nil {
				lg.Warn("twin seed tool_whitelist key has no aranea twin_* mapping, skipped",
					loggateway.Str("tool_key", key))
			}
			continue
		}
		for _, m := range mapped {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// twinAgentDefinition 组装漂移比对用的 Agent 定义视图。
// 种子字段（description/system_prompt/role/model_policy/temperature/tool_whitelist/tags）
// 存于 metadata_json（handleCreateAgent 写入），须透传给 twinmonitor 漂移比对。
func twinAgentDefinition(a biz.Agent) map[string]any {
	def := map[string]any{
		"agent_key": a.AgentKey,
		"provider":  a.Provider,
		"model":     a.Model,
		"roles":     a.Roles,
		"status":    a.Status,
	}
	if a.ConfigJSON != "" {
		var cfg map[string]any
		if json.Unmarshal([]byte(a.ConfigJSON), &cfg) == nil {
			def["config"] = cfg
		}
	}
	// 种子元数据透传：twinmonitor seedAgentDrifted 逐字段比对依赖这些键。
	if a.MetadataJSON != "" {
		var meta map[string]any
		if json.Unmarshal([]byte(a.MetadataJSON), &meta) == nil {
			for _, k := range []string{"description", "system_prompt", "role", "model_policy", "temperature", "tool_whitelist", "tags"} {
				if v, ok := meta[k]; ok {
					def[k] = v
				}
			}
		}
	}
	return def
}

// twinAgentInput twinmonitor Agent 种子同步载荷（对齐 biz.AraneaAgentInput）。
type twinAgentInput struct {
	Name          string         `json:"name"`
	Role          string         `json:"role"`
	Description   string         `json:"description"`
	SystemPrompt  string         `json:"system_prompt"`
	ModelPolicy   string         `json:"model_policy"`
	Temperature   float64        `json:"temperature"`
	ToolWhitelist []string       `json:"tool_whitelist"`
	Tags          []string       `json:"tags"`
	Metadata      map[string]any `json:"metadata"`
	// AgentKey TwinMonitor 预设 Agent 稳定键（ops_*）；图节点 agent_name 按 agent_key 解析，
	// 缺失时 CreateWithFilesAndSettings 校验拒绝（2026-09-01 种子同步 12/13 失败根因）。
	AgentKey string `json:"agent_key"`
}

func (s *TwinOpenAPICompatService) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	var in twinAgentInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeTwinError(w, http.StatusBadRequest, "name is required")
		return
	}
	meta, _ := json.Marshal(map[string]any{
		"source":         caller.Name,
		"model_policy":   in.ModelPolicy,
		"temperature":    in.Temperature,
		"tool_whitelist": in.ToolWhitelist,
		"tags":           in.Tags,
		"metadata":       in.Metadata,
	})
	agent := biz.Agent{
		AgentKey:         strings.TrimSpace(in.AgentKey),
		DisplayName:      in.Name,
		AgentDescription: in.Description,
		MetadataJSON:     string(meta),
		// Kind/Source 显式归类：TwinMonitor 种子同步的预设 Agent 属生态预设（不可直接删除，
		// 由种子同步全生命周期管理）；缺省时 agent_repo_write 无条件 SetKind("") 会冲掉
		// ent 默认值触发枚举校验失败（2026-09-01 种子同步 12/13 失败根因之二）。
		Kind:   "ecosystem_preset",
		Source: "imported",
		// 租户 caller 创建强制归属自有 workspace；系统级 caller 保持空（平台资产）。
		WorkspaceID: caller.WorkspaceID,
	}
	if in.Role != "" {
		agent.Roles = []string{in.Role}
	}
	// 工具白名单落 runtime settings（effective-tools 生效面），而非仅 metadata 展示：
	// profile=minimal 空集起步，专项工具全部由 allow 显式授予（对齐 org-invariants
	// 「岗位专项自带工具面」铁律；datetime 由映射函数附带）。
	allowKeys := mapTwinToolWhitelist(in.ToolWhitelist, s.lg)
	allowJSON, _ := json.Marshal(allowKeys)
	settings := &biz.AgentRuntimeSettings{
		ToolsEnabled:   true,
		ToolsProfile:   "minimal",
		ToolsAllowJSON: string(allowJSON),
	}
	var files []biz.AgentPromptFile
	if strings.TrimSpace(in.SystemPrompt) != "" {
		files = append(files, biz.AgentPromptFile{Name: "system.md", Body: in.SystemPrompt})
	}
	created, err := s.agents.CreateWithFilesAndSettings(r.Context(), agent, files, settings)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"id": created.ID})
}

func (s *TwinOpenAPICompatService) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	id := r.PathValue("id")
	var in twinAgentInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	// 租户写面门禁：仅允许更新自有 Agent；它租户/不存在一律 404（不泄漏存在性）。
	current, err := s.agents.Get(r.Context(), id)
	if err != nil {
		writeTwinError(w, http.StatusNotFound, "agent not found")
		return
	}
	if !caller.isSystem() && current.WorkspaceID != caller.WorkspaceID {
		writeTwinError(w, http.StatusNotFound, "agent not found")
		return
	}
	// 同步 metadata（含 tool_whitelist 等种子字段），供漂移检测下轮比对。
	meta, _ := json.Marshal(map[string]any{
		"source":         caller.Name,
		"model_policy":   in.ModelPolicy,
		"temperature":    in.Temperature,
		"tool_whitelist": in.ToolWhitelist,
		"tags":           in.Tags,
		"metadata":       in.Metadata,
	})
	patch := biz.Agent{
		DisplayName:      in.Name,
		AgentDescription: in.Description,
		MetadataJSON:     string(meta),
	}
	if in.Role != "" {
		patch.Roles = []string{in.Role}
	}
	if _, err := s.agents.Update(r.Context(), id, patch); err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 同步工具白名单授权（与 create 路径同口径：minimal 空集 + allow 显式授予）。
	// 种子漂移更新时授权一并收敛，避免存量 Agent 停在旧空工具面。
	allowKeys := mapTwinToolWhitelist(in.ToolWhitelist, s.lg)
	if _, _, terr := s.agents.UpdateAgentToolPolicy(r.Context(), id, biz.AgentToolPolicyInput{
		ToolsEnabled: true,
		Profile:      "minimal",
		Allow:        allowKeys,
	}); terr != nil {
		writeTwinError(w, http.StatusInternalServerError, terr.Error())
		return
	}
	if strings.TrimSpace(in.SystemPrompt) != "" {
		// 幂等替换 system.md：存在则更新，否则新建。
		replaced := false
		for _, f := range current.Files {
			if f.Name == "system.md" {
				_, _ = s.agents.UpdatePromptFile(r.Context(), biz.AgentPromptFile{
					ID: f.ID, AgentID: id, Name: "system.md", Body: in.SystemPrompt,
				})
				replaced = true
				break
			}
		}
		if !replaced {
			_, _ = s.agents.CreatePromptFile(r.Context(), biz.AgentPromptFile{
				AgentID: id, Name: "system.md", Body: in.SystemPrompt,
			})
		}
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"ok": true})
}
