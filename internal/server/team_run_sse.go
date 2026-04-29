package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aranea-agents/internal/biz"

	sse "github.com/tx7do/kratos-transport/transport/sse"
)

// registerTeamRunSSE exposes GET /team-run-events (legacy-compatible SSE, not tx7do stream IDs).
func registerTeamRunSSE(srv *sse.Server, broker *biz.TeamRunEventBroker) {
	if srv == nil || broker == nil {
		return
	}
	srv.HandleFunc("/team-run-events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming is not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		teamID := r.URL.Query().Get("team_id")
		events, unsubscribe := broker.Subscribe(teamID)
		defer unsubscribe()

		_, _ = fmt.Fprint(w, ": connected\n\n")
		flusher.Flush()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-heartbeat.C:
				_, _ = fmt.Fprint(w, ": heartbeat\n\n")
				flusher.Flush()
			case event, ok := <-events:
				if !ok {
					return
				}
				raw, err := json.Marshal(event)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
				flusher.Flush()
			}
		}
	})
}
