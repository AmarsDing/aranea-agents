// Package service — twin_openapi_graphs.go
//
// TwinOpenAPI 门面的 Graph 目录/写面处理器（自 twin_openapi_compat.go 拆出，
// AS-COG-01 文件行数治理，2026-11-02）。
// 租户门禁（P0 多调用方改造）：
//   - 读面（GET /graphs、GET /graphs/{id}）：共享（workspace_id 空）+ 自有；
//   - 写面（POST/PUT）：租户 caller 创建强制归属自有 workspace，更新仅允许
//     自有图（不存在/它租户一律 404，不泄漏存在性）。
//
// 命名空间打标（metadata.source=caller 名）保留：存量 twinmonitor 种子漂移
// 比对语义不变（系统级 caller 名即 "twinmonitor"）。
package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
)

// twinGraphView 图清单/详情视图（definition 为原始定义，供漂移比对与执行参数推断）。
func twinGraphView(def *biz.GraphDefinition) map[string]any {
	nodes := make([]map[string]any, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		nodes = append(nodes, map[string]any{
			"id":   n.ID,
			"type": n.Type,
			"name": twinNodeDisplayName(n),
		})
	}
	return map[string]any{
		"id":      def.ID,
		"name":    def.Name,
		"ref":     def.Name, // graph 引用键：aranea 以名称作为稳定 ref
		"version": fmt.Sprintf("%d", def.Version),
		"definition": map[string]any{
			"description": def.Description,
			"nodes":       nodes,
			"entry_point": def.EntryPoint,
			"metadata":    def.Metadata,
		},
	}
}

// twinNodeDisplayName 节点显示名（描述优先，回退 ID）。
func twinNodeDisplayName(n biz.NodeDef) string {
	if s := strings.TrimSpace(n.Description); s != "" {
		return s
	}
	if s := strings.TrimSpace(n.AgentName); s != "" {
		return s
	}
	return n.ID
}

// twinSourceTag 存量命名空间标签（系统级 caller=twinmonitor 的历史值）。
// 命名空间隔离的实际载体已迁移到 workspace_id（P0 多调用方改造）；
// source 标保留用于 twinmonitor 种子漂移比对兼容。
const twinSourceTag = "twinmonitor"

// twinGraphInNamespace 判定 Graph 是否属于指定来源命名空间（P1 命名空间隔离）。
// 命中条件：metadata.source 一致；或查询 twinmonitor 命名空间时带有 twin_seed 种子戳
// （兼容打标机制上线前已注册的存量种子图，其首次漂移更新会被补打 source 标）。
func twinGraphInNamespace(meta map[string]any, source string) bool {
	if meta == nil {
		return false
	}
	if s, _ := meta["source"].(string); s == source {
		return true
	}
	if source == twinSourceTag {
		_, ok := meta["twin_seed"].(map[string]any)
		return ok
	}
	return false
}

// ensureTwinGraphSource 强制打标 Graph 命名空间（合并保留 twin_seed 等既有键）。
// 打标值取 caller 名——系统级 caller 为 "twinmonitor"，与存量数据一致。
func ensureTwinGraphSource(def *biz.GraphDefinition, callerName string) {
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	def.Metadata["source"] = callerName
}

// twinGraphReadable 租户读面：共享（workspace_id 空）+ 自有。
func twinGraphReadable(def *biz.GraphDefinition, c *twinCaller) bool {
	if def == nil || c.isSystem() {
		return def != nil
	}
	return def.WorkspaceID == "" || def.WorkspaceID == c.WorkspaceID
}

// twinGraphWritable 租户写面（更新/执行）：仅自有；系统级不受限。
func twinGraphWritable(def *biz.GraphDefinition, c *twinCaller) bool {
	if def == nil || c.isSystem() {
		return def != nil
	}
	return def.WorkspaceID == c.WorkspaceID
}

func (s *TwinOpenAPICompatService) handleListGraphs(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	defs, _, err := s.graphs.ListGraphsByWorkspace(r.Context(), 1000, "", caller.WorkspaceID)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	items := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		if source != "" && !twinGraphInNamespace(d.Metadata, source) {
			continue
		}
		items = append(items, twinGraphView(d))
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *TwinOpenAPICompatService) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	def, err := s.graphs.GetGraph(r.Context(), r.PathValue("id"))
	if err != nil || def == nil || !twinGraphReadable(def, caller) {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}
	writeTwinJSON(w, http.StatusOK, twinGraphView(def))
}

// handleCreateGraph 导入 Graph JSON（场景导入注册；body 为 GraphDefinition JSON）。
func (s *TwinOpenAPICompatService) handleCreateGraph(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	body, err := readTwinBody(w, r, 4<<20)
	if err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	var def biz.GraphDefinition
	if err := json.Unmarshal(body, &def); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid graph json: "+err.Error())
		return
	}
	// 租户 caller 创建强制归属自有 workspace（请求体内的 workspace_id 以 caller 为准）。
	def.WorkspaceID = caller.WorkspaceID
	ensureTwinGraphSource(&def, caller.Name)
	created, err := s.graphs.CreateGraph(r.Context(), &def)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, map[string]any{"id": created.ID})
}

// handleUpdateGraph 原位更新 Graph 定义（twin 种子漂移更新用；ID 不变，版本历史追加）。
// 名称保护：ref=name 是 twin 侧种子同步的匹配键，改名会导致下次同步重复建图，故拒绝改名。
func (s *TwinOpenAPICompatService) handleUpdateGraph(w http.ResponseWriter, r *http.Request) {
	caller := twinCallerFrom(r.Context())
	id := r.PathValue("id")
	existing, err := s.graphs.GetGraph(r.Context(), id)
	if err != nil || existing == nil || !twinGraphWritable(existing, caller) {
		writeTwinError(w, http.StatusNotFound, "graph not found")
		return
	}
	body, err := readTwinBody(w, r, 4<<20)
	if err != nil {
		writeTwinError(w, http.StatusBadRequest, err.Error())
		return
	}
	var def biz.GraphDefinition
	if err := json.Unmarshal(body, &def); err != nil {
		writeTwinError(w, http.StatusBadRequest, "invalid graph json: "+err.Error())
		return
	}
	if def.Name != "" && def.Name != existing.Name {
		writeTwinError(w, http.StatusBadRequest, "graph name is immutable via this API (ref=name 同步口径)")
		return
	}
	def.ID = id
	def.Name = existing.Name
	// workspace 归属不可经更新迁移（防租户把共享图改写为私有、或反向窃取命名空间）。
	def.WorkspaceID = existing.WorkspaceID
	ensureTwinGraphSource(&def, caller.Name)
	saved, err := s.graphs.UpdateGraph(r.Context(), &def)
	if err != nil {
		writeTwinError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeTwinJSON(w, http.StatusOK, twinGraphView(saved))
}
