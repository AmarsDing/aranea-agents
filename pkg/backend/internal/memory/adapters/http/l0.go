package memoryhttp

import (
	mem "arenea/backend/internal/memory/domain"

	"errors"
	"net/http"
)

// HandleL0Snapshots 处理 GET /api/v1/sessions/{id}/l0/snapshots。列表供聊天 UI
// 「上下文」页与智能体进化仪表盘使用。有意原样返回快照行，由 UI / 分析侧自行解析
// `segments_json` / `metadata_json`。
func (m *MemoryHTTP) HandleL0Snapshots(w http.ResponseWriter, r *http.Request, sessionID string) {
	if sessionID == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("session id is required"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 20)
	svc := m.l0()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	snapshots, err := svc.ListSnapshots(r.Context(), sessionID, limit)
	if err != nil {
		m.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	m.writeJSON(w, http.StatusOK, map[string]any{
		"items": snapshots,
		"total": len(snapshots),
	})
}

// l0PreviewRequest 为提示调试器提供最小载荷，可在不落库快照的情况下复现 L0 组装。
// 保留字段与运行时一致，使调试输出与模型所见一致。
type l0PreviewRequest struct {
	SessionID         string `json:"session_id"`
	AgentID           string `json:"agent_id"`
	TeamID            string `json:"team_id"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	ContextWindow     int    `json:"context_window"`
	ReservedForOutput int    `json:"reserved_for_output"`
	UserMessage       string `json:"user_message"`
}

// HandleL0Preview 处理 POST /api/v1/l0/preview。请求体解码为上述最小结构；响应携带脱敏片段
//（仅预览），使 UI 可渲染组装后的提示，且不会泄露 `messages` 已暴露之外的历史内容。
func (m *MemoryHTTP) HandleL0Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in l0PreviewRequest
	if !m.decodeBody(w, r, &in) {
		return
	}
	if in.SessionID == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("session_id is required"))
		return
	}
	svc := m.l0()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	req := mem.L0AssemblyRequest{
		SessionID:         in.SessionID,
		AgentID:           in.AgentID,
		TeamID:            in.TeamID,
		Provider:          in.Provider,
		Model:             in.Model,
		ContextWindow:     in.ContextWindow,
		ReservedForOutput: in.ReservedForOutput,
		UserMessage:       in.UserMessage,
	}
	result, err := svc.Preview(r.Context(), req)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, result)
}

// HandleL0SnapshotByID 处理 GET /api/v1/l0/snapshots/{id}。作为快照列表的「深链」变体，
// 供聊天 UI 直接跳转到某次组装的完整片段表。
func (m *MemoryHTTP) HandleL0SnapshotByID(w http.ResponseWriter, r *http.Request) {
	id := idFromPath(r.URL.Path, "/api/v1/l0/snapshots/")
	if id == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("snapshot id is required"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	svc := m.l0()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L0 service is not configured"))
		return
	}
	snap, err := svc.GetSnapshot(r.Context(), id)
	if err != nil {
		m.writeErr(w, http.StatusNotFound, err)
		return
	}
	m.writeJSON(w, http.StatusOK, snap)
}
