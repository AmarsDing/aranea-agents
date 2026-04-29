package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/service"
)

func (h *HTTPHandler) handleChatMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID := r.URL.Query().Get("session_id")
		items, err := h.chatSvc.ListMessages(sessionID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.Message]{Items: items})
	case http.MethodPost:
		var in service.SendMessageInput
		if !decodeBody(w, r, &in) {
			return
		}
		out, err := h.chatSvc.Send(r.Context(), in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "message", out.AgentMessage.ID, r.Header.Get("X-Request-Id"), "chat.send")
		writeJSON(w, http.StatusCreated, out)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleChatMessagesStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming is not supported"))
		return
	}
	var in service.SendMessageInput
	if !decodeBody(w, r, &in) {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	var writeMu sync.Mutex
	writeEvent := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	err := h.chatSvc.SendStream(r.Context(), in, service.SendStreamCallbacks{
		OnUserMessage: func(message domain.Message) error {
			return writeEvent("user_message", message)
		},
		OnDelta: func(delta string) error {
			return writeEvent("delta", map[string]string{"content": delta})
		},
		OnAgentMessage: func(message domain.Message) error {
			_ = h.auditSvc.Log("create", "message", message.ID, r.Header.Get("X-Request-Id"), "chat.send.stream")
			return writeEvent("done", map[string]domain.Message{"agent_message": message})
		},
		OnToolEvent: func(event adkr.ToolEvent) error {
			return writeEvent("tool_event", event)
		},
		OnTeamMemberStart: func(message domain.Message) error {
			return writeEvent("member_message_start", message)
		},
		OnTeamMemberDelta: func(messageID string, delta string) error {
			return writeEvent("member_delta", map[string]string{"message_id": messageID, "content": delta})
		},
		OnTeamMemberMessage: func(message domain.Message) error {
			_ = h.auditSvc.Log("create", "message", message.ID, r.Header.Get("X-Request-Id"), "chat.team.member.stream")
			return writeEvent("member_message_done", map[string]domain.Message{"agent_message": message})
		},
	})
	if err != nil {
		_ = writeEvent("error", map[string]string{"message": err.Error()})
	}
}

func (h *HTTPHandler) handleChatOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.chatSvc.ListOptions(r.URL.Query().Get("type"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ChatOption]{Items: items})
}
