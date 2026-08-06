package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// evalServer exposes the Agent Memory Challenge Add/Search contract over HTTP.
// It is a thin protocol bridge: all memory logic lives behind
// biz.EvalMemoryStore, and this layer only handles contract validation,
// authentication, and error-code mapping.
type evalServer struct {
	store biz.EvalMemoryStore
	token string
	lg    loggateway.Logger
}

func newEvalServer(store biz.EvalMemoryStore, token string, lg loggateway.Logger) *evalServer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &evalServer{store: store, token: token, lg: lg.With(loggateway.Domain("memoryeval"))}
}

func (s *evalServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("POST /v1/memory/add", s.auth(http.HandlerFunc(s.handleAdd)))
	mux.Handle("POST /v1/memory/search", s.auth(http.HandlerFunc(s.handleSearch)))
	return mux
}

// auth enforces the Memory System Key when configured. Both
// "Authorization: Bearer <key>" and "X-Api-Key: <key>" are accepted per the
// platform contract. An empty configured token disables auth (documented in
// the run instructions; only acceptable inside the platform sandbox).
func (s *evalServer) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" {
			next.ServeHTTP(w, r)
			return
		}
		if extractToken(r) != s.token {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if h != "" {
		return h
	}
	return strings.TrimSpace(r.Header.Get("X-Api-Key"))
}

// ---------- Add ----------

type addRequest struct {
	RequestID string       `json:"request_id"`
	UserID    string       `json:"user_id"`
	SessionID string       `json:"session_id"`
	Messages  []rawMessage `json:"messages"`
}

// rawMessage tolerates the platform's message shape variants: text may arrive
// as "content" or "text"; id as "id" or "message_id".
type rawMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
	Timestamp string `json:"timestamp"`
}

func (m rawMessage) body() string {
	if strings.TrimSpace(m.Content) != "" {
		return m.Content
	}
	return m.Text
}

func (m rawMessage) msgID() string {
	if m.MessageID != "" {
		return m.MessageID
	}
	return m.ID
}

type addResponse struct {
	Success   bool   `json:"success"`
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
	Stored    int    `json:"stored,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *evalServer) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, Error: "invalid JSON body"})
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, Error: "user_id is required"})
		return
	}
	msgs := make([]biz.EvalMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		body := strings.TrimSpace(m.body())
		if body == "" {
			continue
		}
		msgs = append(msgs, biz.EvalMessage{
			Role:      strings.TrimSpace(m.Role),
			Content:   body,
			MessageID: m.msgID(),
			Timestamp: strings.TrimSpace(m.Timestamp),
		})
	}
	if len(msgs) == 0 {
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, Error: "messages must contain at least one non-empty entry"})
		return
	}
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}
	n, err := s.store.AddMessages(r.Context(), req.UserID, strings.TrimSpace(req.SessionID), msgs)
	if err != nil {
		// K2: error path — 5xx is platform-retryable.
		s.lg.Error("eval add failed",
			loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", req.UserID), loggateway.Err(err))
		writeJSON(w, http.StatusInternalServerError, addResponse{Success: false, RequestID: req.RequestID, Error: "add failed"})
		return
	}
	writeJSON(w, http.StatusOK, addResponse{
		Success:   true,
		RequestID: req.RequestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Stored:    n,
	})
}

// ---------- Search ----------

type searchRequest struct {
	Query   string          `json:"query"`
	UserID  string          `json:"user_id"`
	TopK    int32           `json:"top_k"`
	Options json.RawMessage `json:"options"`
}

type searchResponse struct {
	Data  []biz.EvalMemoryItem `json:"data"`
	Error string               `json:"error,omitempty"`
}

func (s *evalServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "invalid JSON body"})
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	req.Query = strings.TrimSpace(req.Query)
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "user_id is required"})
		return
	}
	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "query is required"})
		return
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 100 // platform default
	}
	items, err := s.store.SearchMemories(r.Context(), req.UserID, req.Query, topK)
	if err != nil {
		s.lg.Error("eval search failed",
			loggateway.StepID("memoryeval.search"), loggateway.Str("user_id", req.UserID), loggateway.Err(err))
		writeJSON(w, http.StatusInternalServerError, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "search failed"})
		return
	}
	if items == nil {
		items = []biz.EvalMemoryItem{}
	}
	writeJSON(w, http.StatusOK, searchResponse{Data: items})
}

// ---------- health ----------

func (s *evalServer) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
