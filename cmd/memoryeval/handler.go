package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
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

	// maxBodyBytes caps Add/Search request bodies (EVAL_MAX_BODY_BYTES, 413
	// above); 0 disables the cap, keeping the textual-run behavior identical.
	maxBodyBytes int64
	// mediaDir enables the image evidence copy (EVAL_MEDIA_DIR); empty means
	// images are acknowledged but not persisted — the caption-only default.
	mediaDir string
	// maxImageBytes / maxMediaBytes bound each decoded image and the sum of
	// decoded images per Add request (EVAL_MAX_IMAGE_BYTES /
	// EVAL_MAX_MEDIA_BYTES), defaulting to the contract limits.
	maxImageBytes int64
	maxMediaBytes int64
}

func newEvalServer(store biz.EvalMemoryStore, token string, lg loggateway.Logger) *evalServer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	s := &evalServer{
		store:         store,
		token:         token,
		lg:            lg.With(loggateway.Domain("memoryeval")),
		maxImageBytes: defaultMaxImageBytes,
		maxMediaBytes: defaultMaxImagesBytes,
	}
	s.maxBodyBytes = envInt64("EVAL_MAX_BODY_BYTES", 0)
	if v := strings.TrimSpace(os.Getenv("EVAL_MEDIA_DIR")); v != "" {
		s.mediaDir = v
	}
	if n := envInt64("EVAL_MAX_IMAGE_BYTES", 0); n > 0 {
		s.maxImageBytes = n
	}
	if n := envInt64("EVAL_MAX_MEDIA_BYTES", 0); n > 0 {
		s.maxMediaBytes = n
	}
	return s
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
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
// as "content" or "text"; id as "id" or "message_id". content may be a plain
// string or an ordered ContentPart array (text / image_url).
type rawMessage struct {
	Role      string        `json:"role"`
	Content   rawContent    `json:"content"`
	Text      string        `json:"text"`
	ID        string        `json:"id"`
	MessageID string        `json:"message_id"`
	Timestamp flexTimestamp `json:"timestamp"`
}

// rawContent is the platform content/query field: either a plain string or
// an ordered array of content parts (2026-10-23 verified contract). Unknown
// part types are collected for a warn log instead of failing the request.
type rawContent struct {
	texts        []string
	images       []rawImageRef
	unknownTypes []string
}

// rawImageRef carries the contract image shape into the media path. Index is
// the part position inside the original content array, preserving ordering
// traceability for the evidence copy.
type rawImageRef struct {
	Index int
	URL   string
}

func (c *rawContent) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	if s[0] == '"' {
		var text string
		if err := json.Unmarshal(b, &text); err != nil {
			return err
		}
		c.texts = append(c.texts, text)
		return nil
	}
	if s[0] != '[' {
		return errors.New("content must be a string or an array of parts")
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(b, &parts); err != nil {
		return err
	}
	for i, raw := range parts {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return err
		}
		switch probe.Type {
		case "text":
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			c.texts = append(c.texts, p.Text)
		case "image_url":
			var p struct {
				ImageURL json.RawMessage `json:"image_url"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			url := ""
			if len(p.ImageURL) > 0 {
				var obj struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal(p.ImageURL, &obj); err == nil {
					url = obj.URL
				} else {
					// Tolerate the bare-string image_url form.
					_ = json.Unmarshal(p.ImageURL, &url)
				}
			}
			c.images = append(c.images, rawImageRef{Index: i, URL: url})
		default:
			c.unknownTypes = append(c.unknownTypes, probe.Type)
		}
	}
	return nil
}

// joinedText folds Add-side text parts in original order, newline-separated,
// so downstream statement splitting gives each part its own atom — the same
// as a multi-line string body.
func (c rawContent) joinedText() string {
	return strings.Join(c.texts, "\n")
}

// joinedQuery folds Search-side text parts in original order with a space
// separator; a multi-part question stays one line (never statement-split).
func (c rawContent) joinedQuery() string {
	fields := make([]string, 0, len(c.texts))
	for _, t := range c.texts {
		if v := strings.TrimSpace(t); v != "" {
			fields = append(fields, v)
		}
	}
	return strings.Join(fields, " ")
}

// flexTimestamp tolerates the platform's timestamp variants: JSON string or
// JSON number (Cycle 2 sends Unix milliseconds as a number). Both normalize to
// the decimal-string form; null/absent becomes "".
type flexTimestamp string

func (t *flexTimestamp) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*t = ""
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	*t = flexTimestamp(s)
	return nil
}

func (m rawMessage) body() string {
	if joined := strings.TrimSpace(m.Content.joinedText()); joined != "" {
		return joined
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
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	Timestamp string `json:"timestamp"`
	Stored    int    `json:"stored,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *evalServer) handleAdd(w http.ResponseWriter, r *http.Request) {
	if s.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
	}
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, addResponse{Success: false, Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, Error: "invalid JSON body"})
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, Error: "user_id is required"})
		return
	}
	msgs := make([]biz.EvalMessage, 0, len(req.Messages))
	if req.RequestID == "" {
		req.RequestID = uuid.NewString()
	}
	mediaAck := 0
	mediaBudget := s.maxMediaBytes
	unknown := map[string]int{}
	for i, m := range req.Messages {
		for _, typ := range m.Content.unknownTypes {
			unknown[typ]++
		}
		body := strings.TrimSpace(m.body())
		parts := s.persistMediaParts(req.RequestID, req.UserID, i, m.Content.images, &mediaAck, &mediaBudget)
		if body == "" {
			continue
		}
		messageID := m.msgID()
		if messageID == "" {
			messageID = req.RequestID + ":" + strconv.Itoa(i)
		}
		msgs = append(msgs, biz.EvalMessage{
			Role:      strings.TrimSpace(m.Role),
			Content:   body,
			MessageID: messageID,
			Timestamp: strings.TrimSpace(string(m.Timestamp)),
			Parts:     parts,
		})
	}
	if len(unknown) > 0 {
		types := make([]string, 0, len(unknown))
		for typ, n := range unknown {
			types = append(types, typ+":"+strconv.Itoa(n))
		}
		s.lg.Warn("eval add skipped unsupported content part types",
			loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", req.UserID), loggateway.Str("types", strings.Join(types, ",")))
	}
	if len(msgs) == 0 {
		if mediaAck > 0 {
			// FAQ Q23: an image-only batch must not fail. The caption-only
			// intake acknowledges the payload but has no text evidence to
			// store, so the store is not called and stored stays 0.
			writeJSON(w, http.StatusOK, addResponse{
				Success:   true,
				RequestID: req.RequestID,
				UserID:    req.UserID,
				SessionID: strings.TrimSpace(req.SessionID),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, addResponse{Success: false, UserID: req.UserID, SessionID: req.SessionID, Error: "messages must contain at least one non-empty entry"})
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
		writeJSON(w, http.StatusInternalServerError, addResponse{Success: false, RequestID: req.RequestID, UserID: req.UserID, SessionID: req.SessionID, Error: "add failed"})
		return
	}
	writeJSON(w, http.StatusOK, addResponse{
		Success:   true,
		RequestID: req.RequestID,
		UserID:    req.UserID,
		SessionID: strings.TrimSpace(req.SessionID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Stored:    n,
	})
}

// persistMediaParts acknowledges inline image parts, persisting an evidence
// copy when the media sink is enabled. ack counts every contract-valid image
// (proper Data URI and allowed mime) so an image-only message batch can
// still succeed per FAQ Q23; the evidence copy is written only when
// EVAL_MEDIA_DIR is set and both size budgets allow. Nothing here ever
// fails the request — invalid payloads are logged and skipped.
func (s *evalServer) persistMediaParts(requestID, userID string, msgIndex int, images []rawImageRef, ack *int, budget *int64) []biz.EvalContentPart {
	var parts []biz.EvalContentPart
	for _, img := range images {
		mime, payload, ok := parseEvalDataURI(img.URL)
		if !ok {
			s.lg.Warn("eval add skipped unsupported image payload",
				loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", userID))
			continue
		}
		(*ack)++
		if s.mediaDir == "" {
			continue
		}
		raw, err := decodeEvalImagePayload(payload, s.maxImageBytes)
		if err != nil {
			s.lg.Warn("eval add skipped undecodable or oversized image",
				loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", userID), loggateway.Err(err))
			continue
		}
		// Check before debiting: an over-budget image is dropped without
		// consuming budget, so later in-budget images still persist.
		if *budget-int64(len(raw)) < 0 {
			s.lg.Warn("eval add dropped image over the request media budget",
				loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", userID))
			continue
		}
		*budget -= int64(len(raw))
		path, err := saveEvalMediaPart(s.mediaDir, userID, requestID, msgIndex, img.Index, mime, raw)
		if err != nil {
			s.lg.Warn("eval media evidence write failed",
				loggateway.StepID("memoryeval.add"), loggateway.Str("user_id", userID), loggateway.Err(err))
			continue
		}
		parts = append(parts, biz.EvalContentPart{
			Type:  "image_url",
			Index: img.Index,
			Mime:  mime,
			Bytes: len(raw),
			Ref:   path,
		})
	}
	return parts
}

// ---------- Search ----------

type searchRequest struct {
	Query   rawContent `json:"query"`
	UserID  string     `json:"user_id"`
	TopK    int32      `json:"top_k"`
	Options []string   `json:"options"`
}

type searchResponse struct {
	Data  []biz.EvalMemoryItem `json:"data"`
	Error string               `json:"error,omitempty"`
}

func (s *evalServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
	}
	var req searchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "request body too large"})
			return
		}
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "invalid JSON body"})
		return
	}
	req.UserID = strings.TrimSpace(req.UserID)
	// query may be a string or a ContentPart array; image parts carry no text
	// and are ignored by the caption-only retrieval route.
	query := strings.TrimSpace(req.Query.joinedQuery())
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "user_id is required"})
		return
	}
	if query == "" {
		writeJSON(w, http.StatusBadRequest, searchResponse{Data: []biz.EvalMemoryItem{}, Error: "query is required"})
		return
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 100 // platform default
	}
	items, err := s.store.SearchMemories(r.Context(), req.UserID, query, topK, req.Options...)
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
