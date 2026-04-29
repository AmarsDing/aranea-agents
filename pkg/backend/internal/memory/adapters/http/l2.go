// transport/memory_l2.go 暴露 L2 情景记忆 HTTP 接口，见 `aranea/docs/14 memory-L2-episodic.md` §6.2–§6.6。
// 路由在 sessions.go（handleSessionByID）中将 `/l2/` 后缀转发至此。
// 管理端点（合并 / 保留策略）在 handler.go 中通过 registerMemoryL2AdminRoutes 挂载。
package memoryhttp

import (
	mem "arenea/backend/internal/memory/domain"

	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/service"
)

// registerMemoryL2AdminRoutes 挂载管理作用域的保留与合并端点，位于 /api/v1/admin/memory/l2/。
func (m *MemoryHTTP) registerMemoryL2AdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/admin/memory/l2/retention/run", m.handleL2RetentionRun)
}

// HandleL2Routes 分发 /api/v1/sessions/{sid}/l2/<resource>... 请求。
// 资源：
//   - events                         (GET)
//   - events/{ref_kind}/{ref_id}     (GET, Phase 2)
//   - episodes                       (GET, POST)
//   - episodes/{id}                  (GET, PATCH, DELETE)
//   - episodes/{id}/reindex          (POST)
//   - episodes/{id}/consolidate      (POST, Phase 3 stub)
//   - marks                          (GET, POST)
//   - marks/{id}                     (DELETE)
//   - recall                         (POST)
func (m *MemoryHTTP) HandleL2Routes(w http.ResponseWriter, r *http.Request, sessionID, suffix string) {
	svc := m.l2()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L2 service is not configured"))
		return
	}
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] == "" {
		m.writeErr(w, http.StatusNotFound, errors.New("unknown l2 path"))
		return
	}
	switch parts[0] {
	case "events":
		m.handleL2Events(w, r, svc, sessionID, parts[1:])
	case "episodes":
		m.handleL2Episodes(w, r, svc, sessionID, parts[1:])
	case "marks":
		m.handleL2Marks(w, r, svc, sessionID, parts[1:])
	case "recall":
		m.handleL2Recall(w, r, svc, sessionID)
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown l2 resource"))
	}
}

// --- 事件 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL2Events(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	if len(parts) > 0 && parts[0] != "" {
		// `events/{ref_kind}/{ref_id}` 会进入此分支；第一阶段未实现，客户端可通过
		// ref_table / ref_id 经现有按表端点回查。
		m.writeErr(w, http.StatusNotImplemented, errors.New("event detail endpoint not implemented in phase 1"))
		return
	}
	if r.Method != http.MethodGet {
		m.methodNotAllowed(w)
		return
	}
	q := mem.MemoryL2EventQuery{
		SessionID:    sessionID,
		TurnID:       r.URL.Query().Get("turn_id"),
		SpanID:       r.URL.Query().Get("span_id"),
		Kinds:        splitCSV(r.URL.Query().Get("kinds")),
		ActorIDs:     splitCSV(r.URL.Query().Get("actor_id")),
		StatusIn:     splitCSV(r.URL.Query().Get("status")),
		StartTimeUTC: r.URL.Query().Get("start_time"),
		EndTimeUTC:   r.URL.Query().Get("end_time"),
		Keyword:      r.URL.Query().Get("keyword"),
		Limit:        m.parsePositiveInt(r.URL.Query().Get("limit"), 100),
		Offset:       m.parsePositiveInt(r.URL.Query().Get("offset"), 0),
	}
	result, err := svc.ListEvents(r.Context(), q)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	m.writeJSON(w, http.StatusOK, result)
}

// --- 片段 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL2Episodes(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	switch len(parts) {
	case 0:
		m.handleL2EpisodeCollection(w, r, svc, sessionID)
	case 1:
		m.handleL2EpisodeItem(w, r, svc, sessionID, parts[0])
	case 2:
		m.handleL2EpisodeAction(w, r, svc, sessionID, parts[0], parts[1])
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown episode path"))
	}
}

func (m *MemoryHTTP) handleL2EpisodeCollection(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		kind := r.URL.Query().Get("kind")
		limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 50)
		offset := m.parsePositiveInt(r.URL.Query().Get("offset"), 0)
		result, err := svc.ListEpisodes(r.Context(), sessionID, kind, limit, offset)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var in service.CreateEpisodeInput
		if !m.decodeBody(w, r, &in) {
			return
		}
		in.SessionID = sessionID
		ep, err := svc.CreateMilestoneEpisode(r.Context(), in)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l2.create_episode", "memory_episodes", ep.ID, r.Header.Get("X-Request-Id"), ep.Title)
		m.writeJSON(w, http.StatusCreated, ep)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL2EpisodeItem(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID, episodeID string) {
	if episodeID == "" {
		m.writeErr(w, http.StatusBadRequest, errors.New("episode id is required"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		detail, err := svc.GetEpisode(r.Context(), episodeID)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		if detail.Episode.SessionID != sessionID {
			m.writeErr(w, http.StatusNotFound, errors.New("episode not found in session"))
			return
		}
		m.writeJSON(w, http.StatusOK, detail)
	case http.MethodPatch:
		current, err := svc.GetEpisode(r.Context(), episodeID)
		if err != nil {
			m.writeErr(w, http.StatusNotFound, err)
			return
		}
		if current.Episode.SessionID != sessionID {
			m.writeErr(w, http.StatusNotFound, errors.New("episode not found in session"))
			return
		}
		var in struct {
			Title          *string                `json:"title,omitempty"`
			Goal           *string                `json:"goal,omitempty"`
			Outcome        *string                `json:"outcome,omitempty"`
			OutcomeSummary *string                `json:"outcome_summary,omitempty"`
			ResultPreview  *string                `json:"result_preview,omitempty"`
			FailureReason  *string                `json:"failure_reason,omitempty"`
			Importance     *float64               `json:"importance,omitempty"`
			Confidence     *float64               `json:"confidence,omitempty"`
			UserFeedback   *string                `json:"user_feedback,omitempty"`
			CriticScore    *float64               `json:"critic_score,omitempty"`
			Metadata       map[string]any         `json:"metadata,omitempty"`
		}
		if !m.decodeBody(w, r, &in) {
			return
		}
		ep := current.Episode
		if in.Title != nil {
			ep.Title = *in.Title
		}
		if in.Goal != nil {
			ep.Goal = *in.Goal
		}
		if in.Outcome != nil {
			ep.Outcome = *in.Outcome
		}
		if in.OutcomeSummary != nil {
			ep.OutcomeSummary = *in.OutcomeSummary
		}
		if in.ResultPreview != nil {
			ep.ResultPreview = *in.ResultPreview
		}
		if in.FailureReason != nil {
			ep.FailureReason = *in.FailureReason
		}
		if in.Importance != nil {
			ep.Importance = clampFloat(*in.Importance, 0, 1)
		}
		if in.Confidence != nil {
			ep.Confidence = clampFloat(*in.Confidence, 0, 1)
		}
		if in.UserFeedback != nil {
			ep.UserFeedback = *in.UserFeedback
		}
		if in.CriticScore != nil {
			ep.CriticScore = *in.CriticScore
		}
		if in.Metadata != nil {
			ep.MetadataJSON = encodeJSONOrEmpty(in.Metadata)
		}
		updated, err := svc.UpdateEpisode(r.Context(), ep)
		if err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		m.writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := svc.DeleteEpisode(r.Context(), episodeID); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l2.delete_episode", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		m.methodNotAllowed(w)
	}
}

func (m *MemoryHTTP) handleL2EpisodeAction(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID, episodeID, action string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	switch action {
	case "reindex":
		if err := svc.BuildIndexFor(r.Context(), episodeID); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = m.audit.Log("l2.reindex", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusAccepted)
	case "consolidate":
		// 第三阶段：合并工作者将拾取该片段。当前仅写入审计轨迹，供前端确认请求已到达后端。
		_ = m.audit.Log("l2.consolidate_request", "memory_episodes", episodeID, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusAccepted)
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown episode action"))
	}
	_ = sessionID
}

// --- 标记 --------------------------------------------------------------------

func (m *MemoryHTTP) handleL2Marks(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string, parts []string) {
	switch len(parts) {
	case 0:
		switch r.Method {
		case http.MethodGet:
			markType := r.URL.Query().Get("type")
			limit := m.parsePositiveInt(r.URL.Query().Get("limit"), 100)
			marks, err := svc.ListMarks(r.Context(), sessionID, markType, limit)
			if err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
			if marks == nil {
				marks = []mem.MemoryEventMark{}
			}
			m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryEventMark]{Items: marks})
		case http.MethodPost:
			var in service.MarkInput
			if !m.decodeBody(w, r, &in) {
				return
			}
			stored, err := svc.Mark(r.Context(), in)
			if err != nil {
				m.writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = m.audit.Log("l2.mark", "memory_event_marks", stored.ID, r.Header.Get("X-Request-Id"), stored.MarkType)
			m.writeJSON(w, http.StatusCreated, stored)
		default:
			m.methodNotAllowed(w)
		}
	case 1:
		if r.Method != http.MethodDelete {
			m.methodNotAllowed(w)
			return
		}
		if err := svc.UnMark(r.Context(), parts[0]); err != nil {
			m.writeErr(w, http.StatusBadRequest, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		m.writeErr(w, http.StatusNotFound, errors.New("unknown marks path"))
	}
}

// --- 回忆 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL2Recall(w http.ResponseWriter, r *http.Request, svc *service.MemoryL2Service, sessionID string) {
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	var in mem.MemoryL2RecallQuery
	if !m.decodeBody(w, r, &in) {
		return
	}
	in.SessionID = sessionID
	results, err := svc.RecallByQuery(r.Context(), in)
	if err != nil {
		m.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if results == nil {
		results = []mem.MemoryL2RecallResult{}
	}
	m.writeJSON(w, http.StatusOK, listResponse[mem.MemoryL2RecallResult]{Items: results})
}

// --- 管理 -------------------------------------------------------------------

func (m *MemoryHTTP) handleL2RetentionRun(w http.ResponseWriter, r *http.Request) {
	svc := m.l2()
	if svc == nil {
		m.writeErr(w, http.StatusServiceUnavailable, errors.New("memory L2 service is not configured"))
		return
	}
	if r.Method != http.MethodPost {
		m.methodNotAllowed(w)
		return
	}
	report, err := svc.ApplyRetention(r.Context())
	if err != nil {
		m.writeErr(w, http.StatusInternalServerError, err)
		return
	}
	m.writeJSON(w, http.StatusOK, report)
}

// --- 辅助 -------------------------------------------------------------------


func splitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func encodeJSONOrEmpty(value any) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}
