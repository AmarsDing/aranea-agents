// Package cataloghttp 提供 Catalog 边界的 HTTP 适配器（L4 自进化等）。
package cataloghttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

type listResponse[T any] struct {
	Items []T `json:"items"`
}

// EvolutionHTTP 暴露 /api/v1/agent-evolution/ 与 handleAgentByID 下的进化别名路径。
// 由 transport 注入共享的 writeErr/writeJSON 等，以保持错误格式与 P7 前一致。
type EvolutionHTTP struct {
	evolution        func() *service.AgentEvolutionService
	audit            *service.AuditService
	writeJSON        func(http.ResponseWriter, int, any)
	writeErr         func(http.ResponseWriter, int, error)
	decodeBody       func(http.ResponseWriter, *http.Request, any) bool
	methodNotAllowed func(http.ResponseWriter)
	parsePositiveInt func(string, int) int
}

// NewEvolutionHTTP 从 transport 的依赖与辅助函数构建；evolution 为 *HTTPHandler.evolutionService 方法值或等价闭包。
func NewEvolutionHTTP(
	evolution func() *service.AgentEvolutionService,
	audit *service.AuditService,
	writeJSON func(http.ResponseWriter, int, any),
	writeErr func(http.ResponseWriter, int, error),
	decodeBody func(http.ResponseWriter, *http.Request, any) bool,
	methodNotAllowed func(http.ResponseWriter),
	parsePositiveInt func(string, int) int,
) *EvolutionHTTP {
	return &EvolutionHTTP{
		evolution:        evolution,
		audit:            audit,
		writeJSON:        writeJSON,
		writeErr:         writeErr,
		decodeBody:       decodeBody,
		methodNotAllowed: methodNotAllowed,
		parsePositiveInt: parsePositiveInt,
	}
}

// Register 在 mux 上注册 /api/v1/agent-evolution/ 前缀的处理器。
func (e *EvolutionHTTP) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/agent-evolution/", e.handleAgentEvolution)
}

func (e *EvolutionHTTP) evolutionService() *service.AgentEvolutionService {
	if e.evolution == nil {
		return nil
	}
	return e.evolution()
}

// handleAgentEvolution 分发 /api/v1/agent-evolution/{agent_id}/... 路径。
func (e *EvolutionHTTP) handleAgentEvolution(w http.ResponseWriter, r *http.Request) {
	svc := e.evolutionService()
	if svc == nil {
		e.writeErr(w, http.StatusServiceUnavailable, errors.New("agent evolution service is not configured"))
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/agent-evolution/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		e.writeErr(w, http.StatusBadRequest, errors.New("agent_id and sub-resource are required"))
		return
	}
	agentID := parts[0]
	switch parts[1] {
	case "identity":
		e.handleAgentIdentity(w, r, svc, agentID)
	case "strategy":
		e.handleAgentStrategy(w, r, svc, agentID)
	case "proposals":
		e.handleAgentProposals(w, r, svc, agentID, parts[2:])
	case "events":
		e.handleAgentEvents(w, r, svc, agentID, parts[2:])
	case "skill-stats":
		e.handleAgentSkillStats(w, r, svc, agentID)
	case "scan":
		e.handleAgentEvolutionScan(w, r, svc, agentID)
	case "metrics":
		e.handleAgentEvolutionMetrics(w, r, svc, agentID)
	case "suggestions":
		e.handleAgentEvolutionSuggestions(w, r, svc, agentID)
	case "training-data":
		e.handleAgentEvolutionTrainingData(w, r, svc, agentID)
	default:
		e.writeErr(w, http.StatusNotFound, errors.New("unknown evolution sub-resource"))
	}
}

// HandleAgentEvolutionAgentPath 分发 /api/v1/agents/{id}/... 下的 identity、strategy、evolution/... 别名。若已处理则返回 true。
func (e *EvolutionHTTP) HandleAgentEvolutionAgentPath(w http.ResponseWriter, r *http.Request, pathSuffix string) bool {
	svc := e.evolutionService()
	if svc == nil {
		e.writeErr(w, http.StatusServiceUnavailable, errors.New("agent evolution service is not configured"))
		return true
	}
	parts := strings.Split(strings.Trim(pathSuffix, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return false
	}
	agentID := parts[0]
	switch parts[1] {
	case "identity":
		e.handleAgentIdentity(w, r, svc, agentID)
	case "strategy":
		e.handleAgentStrategy(w, r, svc, agentID)
	case "skill-stats":
		e.handleAgentSkillStats(w, r, svc, agentID)
	case "evolution":
		if len(parts) < 3 {
			e.writeErr(w, http.StatusBadRequest, errors.New("evolution sub-resource is required"))
			return true
		}
		switch parts[2] {
		case "events":
			e.handleAgentEvents(w, r, svc, agentID, parts[3:])
		case "proposals":
			e.handleAgentProposals(w, r, svc, agentID, parts[3:])
		case "scan":
			e.handleAgentEvolutionScan(w, r, svc, agentID)
		case "metrics":
			e.handleAgentEvolutionMetrics(w, r, svc, agentID)
		case "suggestions":
			e.handleAgentEvolutionSuggestions(w, r, svc, agentID)
		case "training-data":
			e.handleAgentEvolutionTrainingData(w, r, svc, agentID)
		default:
			e.writeErr(w, http.StatusNotFound, errors.New("unknown evolution sub-resource"))
		}
	default:
		return false
	}
	return true
}

func (e *EvolutionHTTP) handleAgentIdentity(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		identity, err := svc.GetIdentity(r.Context(), agentID)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		e.writeJSON(w, http.StatusOK, identity)
	case http.MethodPatch:
		var patch service.IdentityPatch
		if !e.decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateIdentity(r.Context(), agentID, patch)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = e.audit.Log("agent.evolution.identity.update", "agent_identity", agentID, r.Header.Get("X-Request-Id"), patch.Reason)
		e.writeJSON(w, http.StatusOK, updated)
	default:
		e.methodNotAllowed(w)
	}
}

func (e *EvolutionHTTP) handleAgentStrategy(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		profile, err := svc.GetStrategy(r.Context(), agentID)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		e.writeJSON(w, http.StatusOK, profile)
	case http.MethodPatch:
		var patch service.StrategyPatch
		if !e.decodeBody(w, r, &patch) {
			return
		}
		updated, err := svc.UpdateStrategy(r.Context(), agentID, patch)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = e.audit.Log("agent.evolution.strategy.update", "agent_strategy_profile", agentID, r.Header.Get("X-Request-Id"), patch.Reason)
		e.writeJSON(w, http.StatusOK, updated)
	default:
		e.methodNotAllowed(w)
	}
}

func (e *EvolutionHTTP) handleAgentProposals(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string, tail []string) {
	if len(tail) == 0 {
		switch r.Method {
		case http.MethodGet:
			status := r.URL.Query().Get("status")
			limit := e.parsePositiveInt(r.URL.Query().Get("limit"), 50)
			offset := e.parsePositiveInt(r.URL.Query().Get("offset"), 0)
			out, err := svc.ListProposals(r.Context(), agentID, status, limit, offset)
			if err != nil {
				e.writeErr(w, http.StatusBadRequest, err)
				return
			}
			e.writeJSON(w, http.StatusOK, out)
		case http.MethodPost:
			var in service.ProposalInput
			if !e.decodeBody(w, r, &in) {
				return
			}
			in.AgentID = agentID
			prop, err := svc.Propose(r.Context(), in)
			if err != nil {
				e.writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = e.audit.Log("agent.evolution.proposal.create", "agent_evolution_proposals", prop.ID, r.Header.Get("X-Request-Id"), in.Source)
			e.writeJSON(w, http.StatusCreated, prop)
		default:
			e.methodNotAllowed(w)
		}
		return
	}
	if tail[0] == "" {
		e.writeErr(w, http.StatusBadRequest, errors.New("proposal id is required"))
		return
	}
	proposalID := tail[0]
	if len(tail) == 1 {
		if r.Method != http.MethodGet {
			e.methodNotAllowed(w)
			return
		}
		prop, err := svc.GetProposal(r.Context(), proposalID)
		if err != nil {
			e.writeErr(w, http.StatusNotFound, err)
			return
		}
		e.writeJSON(w, http.StatusOK, prop)
		return
	}
	switch tail[1] {
	case "approve":
		e.handleProposalApprove(w, r, svc, proposalID)
	case "reject":
		e.handleProposalReject(w, r, svc, proposalID)
	default:
		e.writeErr(w, http.StatusNotFound, errors.New("unknown proposal action"))
	}
}

func (e *EvolutionHTTP) handleProposalApprove(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, proposalID string) {
	if r.Method != http.MethodPost {
		e.methodNotAllowed(w)
		return
	}
	var in struct {
		By string `json:"by"`
	}
	_ = e.decodeBody(w, r, &in)
	event, err := svc.Approve(r.Context(), proposalID, in.By)
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = e.audit.Log("agent.evolution.proposal.approve", "agent_evolution_proposals", proposalID, r.Header.Get("X-Request-Id"), in.By)
	e.writeJSON(w, http.StatusOK, event)
}

func (e *EvolutionHTTP) handleProposalReject(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, proposalID string) {
	if r.Method != http.MethodPost {
		e.methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = e.decodeBody(w, r, &in)
	if err := svc.Reject(r.Context(), proposalID, in.By, in.Reason); err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = e.audit.Log("agent.evolution.proposal.reject", "agent_evolution_proposals", proposalID, r.Header.Get("X-Request-Id"), in.Reason)
	w.WriteHeader(http.StatusNoContent)
}

func (e *EvolutionHTTP) handleAgentEvents(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string, tail []string) {
	if len(tail) == 0 {
		if r.Method != http.MethodGet {
			e.methodNotAllowed(w)
			return
		}
		kind := r.URL.Query().Get("kind")
		limit := e.parsePositiveInt(r.URL.Query().Get("limit"), 50)
		offset := e.parsePositiveInt(r.URL.Query().Get("offset"), 0)
		out, err := svc.ListEvents(r.Context(), agentID, kind, limit, offset)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		e.writeJSON(w, http.StatusOK, out)
		return
	}
	if tail[0] == "" {
		e.writeErr(w, http.StatusBadRequest, errors.New("event id is required"))
		return
	}
	eventID := tail[0]
	if len(tail) == 1 {
		if r.Method != http.MethodGet {
			e.methodNotAllowed(w)
			return
		}
		event, err := svc.GetEvent(r.Context(), eventID)
		if err != nil {
			e.writeErr(w, http.StatusNotFound, err)
			return
		}
		e.writeJSON(w, http.StatusOK, event)
		return
	}
	switch tail[1] {
	case "revert":
		e.handleEventRevert(w, r, svc, eventID)
	default:
		e.writeErr(w, http.StatusNotFound, errors.New("unknown event action"))
	}
}

func (e *EvolutionHTTP) handleEventRevert(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, eventID string) {
	if r.Method != http.MethodPost {
		e.methodNotAllowed(w)
		return
	}
	var in struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	_ = e.decodeBody(w, r, &in)
	event, err := svc.Revert(r.Context(), eventID, in.By, in.Reason)
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = e.audit.Log("agent.evolution.event.revert", "agent_evolution_events", eventID, r.Header.Get("X-Request-Id"), in.Reason)
	e.writeJSON(w, http.StatusOK, event)
}

func (e *EvolutionHTTP) handleAgentSkillStats(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	switch r.Method {
	case http.MethodGet:
		limit := e.parsePositiveInt(r.URL.Query().Get("limit"), 50)
		stats, err := svc.ListSkillStats(r.Context(), agentID, limit)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		if stats == nil {
			stats = []domain.AgentSkillStat{}
		}
		e.writeJSON(w, http.StatusOK, listResponse[domain.AgentSkillStat]{Items: stats})
	case http.MethodPost:
		var in domain.AgentSkillStat
		if !e.decodeBody(w, r, &in) {
			return
		}
		in.AgentID = agentID
		stat, err := svc.UpsertSkillStat(r.Context(), in)
		if err != nil {
			e.writeErr(w, http.StatusBadRequest, err)
			return
		}
		e.writeJSON(w, http.StatusOK, stat)
	default:
		e.methodNotAllowed(w)
	}
}

func (e *EvolutionHTTP) handleAgentEvolutionScan(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodPost {
		e.methodNotAllowed(w)
		return
	}
	report, err := svc.RunEvolutionScan(r.Context(), agentID)
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = e.audit.Log("agent.evolution.scan", "agent_identity", agentID, r.Header.Get("X-Request-Id"), report.Note)
	e.writeJSON(w, http.StatusOK, report)
}

func (e *EvolutionHTTP) handleAgentEvolutionMetrics(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		e.methodNotAllowed(w)
		return
	}
	report, err := svc.Metrics(r.Context(), agentID, r.URL.Query().Get("range"))
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	e.writeJSON(w, http.StatusOK, report)
}

func (e *EvolutionHTTP) handleAgentEvolutionSuggestions(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		e.methodNotAllowed(w)
		return
	}
	limit := e.parsePositiveInt(r.URL.Query().Get("limit"), 20)
	items, err := svc.Suggestions(r.Context(), agentID, r.URL.Query().Get("range"), limit)
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if items == nil {
		items = []service.EvolutionSuggestion{}
	}
	e.writeJSON(w, http.StatusOK, listResponse[service.EvolutionSuggestion]{Items: items})
}

func (e *EvolutionHTTP) handleAgentEvolutionTrainingData(w http.ResponseWriter, r *http.Request, svc *service.AgentEvolutionService, agentID string) {
	if r.Method != http.MethodGet {
		e.methodNotAllowed(w)
		return
	}
	limit := e.parsePositiveInt(r.URL.Query().Get("limit"), 100)
	items, err := svc.TrainingData(r.Context(), agentID, limit)
	if err != nil {
		e.writeErr(w, http.StatusBadRequest, err)
		return
	}
	if r.URL.Query().Get("format") == "jsonl" {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		enc := json.NewEncoder(w)
		for _, item := range items {
			_ = enc.Encode(item)
		}
		return
	}
	if items == nil {
		items = []service.EvolutionTrainingExample{}
	}
	e.writeJSON(w, http.StatusOK, listResponse[service.EvolutionTrainingExample]{Items: items})
}
