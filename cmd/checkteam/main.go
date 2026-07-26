package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type probe struct {
	EnableStateDeliverable bool   `json:"enable_state_deliverable"`
	IntentAnchorAgentID    string `json:"intent_anchor_agent_id"`
	Members                []struct {
		AgentID string `json:"agent_id"`
	} `json:"members"`
}

func anchorOf(p probe) string {
	first := ""
	for _, m := range p.Members {
		if id := strings.TrimSpace(m.AgentID); id != "" {
			first = id
			break
		}
	}
	if want := strings.TrimSpace(p.IntentAnchorAgentID); want != "" {
		for _, m := range p.Members {
			if strings.TrimSpace(m.AgentID) == want {
				return want
			}
		}
	}
	return first
}

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open err:", err)
		os.Exit(1)
	}
	defer db.Close()

	// setws mode: go run ./cmd/checkteam setws <teamID> <workspaceID>
	// Scratch helper to move a shared (workspace_id="") team into a tenant
	// workspace so tenant-facing mutate endpoints (run-test) accept it.
	// setws mode: go run ./cmd/checkteam setws <teamID> <workspaceID|->
	// Scratch helper to move a shared (workspace_id="") team into a tenant
	// workspace so tenant-facing mutate endpoints (run-test) accept it.
	// "-" maps to empty string (shared).
	if len(os.Args) >= 4 && os.Args[1] == "setws" {
		ws := os.Args[3]
		if ws == "-" {
			ws = ""
		}
		res, err := db.Exec(`UPDATE teams SET workspace_id = $2 WHERE id = $1`, os.Args[2], ws)
		if err != nil {
			fmt.Println("update err:", err)
			os.Exit(1)
		}
		n, _ := res.RowsAffected()
		fmt.Printf("updated rows=%d team=%s workspace_id=%q\n", n, os.Args[2], os.Args[3])
		return
	}

	// runs mode: go run ./cmd/checkteam runs <teamID>
	// List team_runs for a team.
	if len(os.Args) >= 3 && os.Args[1] == "runs" {
		listRuns(db, os.Args[2])
		return
	}

	// closerun mode: go run ./cmd/checkteam closerun <runID> <status>
	// Scratch helper to close a stale (leaked) run so new tests can start.
	if len(os.Args) >= 4 && os.Args[1] == "closerun" {
		res, err := db.Exec(`UPDATE team_runs SET status = $2, finished_at = NOW(), updated_at = NOW() WHERE id = $1`, os.Args[2], os.Args[3])
		if err != nil {
			fmt.Println("closerun err:", err)
			os.Exit(1)
		}
		n, _ := res.RowsAffected()
		fmt.Printf("closerun rows=%d run=%s status=%q\n", n, os.Args[2], os.Args[3])
		return
	}

	// delvev mode: go run ./cmd/checkteam delvev <sessionID>
	// Dump full set_deliverable events (content + args) for one session.
	if len(os.Args) >= 3 && os.Args[1] == "delvev" {
		dumpDeliverableEvents(db, os.Args[2])
		return
	}

	// sess mode: go run ./cmd/checkteam sess <sessionID>
	// Dump trpc_session_states + set_deliverable events for one session.
	if len(os.Args) >= 3 && os.Args[1] == "sess" {
		probeSession(db, os.Args[2])
		return
	}

	teamIDs := []string{"7cb96d6d4c912f6e11eca1b6", "bff43a17f5556a3392b56d55"}

	for _, teamID := range teamIDs {
		fmt.Printf("\n================ TEAM %s ================\n", teamID)

		var status, dagNodeID, defJSON string
		err := db.QueryRow(`
			SELECT COALESCE(status,''), COALESCE(dag_node_id,''), COALESCE(definition_json,'')
			FROM teams WHERE id = $1
		`, teamID).Scan(&status, &dagNodeID, &defJSON)
		if err != nil {
			fmt.Println("team query err:", err)
			continue
		}
		fmt.Printf("status=%s dag_node_id=%s\n", status, dagNodeID)
		var wsID string
		if err := db.QueryRow(`SELECT COALESCE(workspace_id,'<null>') FROM teams WHERE id = $1`, teamID).Scan(&wsID); err == nil {
			fmt.Printf("workspace_id=%s\n", wsID)
		}
		var delAt string
		if err := db.QueryRow(`SELECT COALESCE(deleted_at,'<null>') FROM teams WHERE id = $1`, teamID).Scan(&delAt); err == nil {
			fmt.Printf("deleted_at=%q\n", delAt)
		}

		var p probe
		if err := json.Unmarshal([]byte(defJSON), &p); err != nil {
			fmt.Println("definition_json parse err:", err)
		} else {
			fmt.Printf("enable_state_deliverable=%v intent_anchor=%q anchor(resolved)=%q members=%d\n",
				p.EnableStateDeliverable, p.IntentAnchorAgentID, anchorOf(p), len(p.Members))
		}

		// List sessions bound to this team (v1 + v2)
		type sessRow struct {
			id, stype, created string
			src                string
		}
		var sessions []sessRow
		for _, q := range []struct {
			src, sql string
		}{
			{"sessions", `SELECT id, COALESCE(session_type,''), COALESCE(CAST(created_at AS TEXT),'') FROM sessions WHERE team_id = $1 ORDER BY created_at`},
			{"sessions_v2", `SELECT id, COALESCE(session_type,''), COALESCE(CAST(created_at AS TEXT),'') FROM sessions_v2 WHERE team_id = $1 ORDER BY created_at`},
		} {
			rows, err := db.Query(q.sql, teamID)
			if err != nil {
				fmt.Printf("%s query err: %v\n", q.src, err)
				continue
			}
			for rows.Next() {
				var r sessRow
				if err := rows.Scan(&r.id, &r.stype, &r.created); err == nil {
					r.src = q.src
					sessions = append(sessions, r)
				}
			}
			rows.Close()
		}
		fmt.Printf("-- bound sessions: %d\n", len(sessions))
		for _, s := range sessions {
			fmt.Printf("   [%s] id=%s type=%s created=%s\n", s.src, s.id, s.stype, s.created)
		}

		// For each session, scan ALL coordinate rows in trpc_session_states
		for _, s := range sessions {
			fmt.Printf("-- trpc_session_states rows for session_id=%s:\n", s.id)
			rows, err := db.Query(`
				SELECT app_name, user_id, COALESCE(state::text,'')
				FROM trpc_session_states WHERE session_id = $1
			`, s.id)
			if err != nil {
				fmt.Println("   state query err:", err)
				continue
			}
			found := false
			for rows.Next() {
				found = true
				var app, user, state string
				if err := rows.Scan(&app, &user, &state); err != nil {
					continue
				}
				hasDeliv := strings.Contains(state, `"deliverable"`)
				fmt.Printf("   app=%q user=%q state_len=%d has_deliverable_key=%v\n", app, user, len(state), hasDeliv)
				var m map[string]any
				if err := json.Unmarshal([]byte(state), &m); err == nil {
					keys := make([]string, 0, len(m))
					for k := range m {
						keys = append(keys, k)
					}
					fmt.Printf("   top_level_keys: %v\n", keys)
					if d, ok := m["deliverable"]; ok {
						dj, _ := json.Marshal(d)
						preview := string(dj)
						if len(preview) > 400 {
							preview = preview[:400] + "..."
						}
						fmt.Printf("   deliverable_TOPLEVEL_preview: %s\n", preview)
					} else {
						// find nested occurrence context
						idx := strings.Index(state, `"deliverable"`)
						start := idx - 120
						if start < 0 {
							start = 0
						}
						end := idx + 200
						if end > len(state) {
							end = len(state)
						}
						fmt.Printf("   deliverable_NESTED_context: ...%s...\n", state[start:end])
					}
				}
			}
			rows.Close()
			if !found {
				fmt.Println("   (no rows)")
			}

			// Dump set_deliverable events for this session
			fmt.Printf("-- set_deliverable events for session_id=%s:\n", s.id)
			evRows, err := db.Query(`
				SELECT app_name, user_id, event::text
				FROM trpc_session_events
				WHERE session_id = $1 AND event::text LIKE '%set_deliverable%'
				ORDER BY id
			`, s.id)
			if err != nil {
				fmt.Println("   event query err:", err)
				continue
			}
			evFound := false
			for evRows.Next() {
				evFound = true
				var app, user, evText string
				if err := evRows.Scan(&app, &user, &evText); err != nil {
					continue
				}
				var ev map[string]any
				if err := json.Unmarshal([]byte(evText), &ev); err != nil {
					fmt.Println("   event parse err:", err)
					continue
				}
				fmt.Printf("   app=%q user=%q author=%v done=%v id=%v\n",
					app, user, ev["author"], ev["done"], ev["id"])
				if sd, ok := ev["stateDelta"].(map[string]any); ok {
					keys := make([]string, 0, len(sd))
					for k := range sd {
						keys = append(keys, k)
					}
					fmt.Printf("   stateDelta_keys=%v\n", keys)
					if d, ok := sd["deliverable"]; ok {
						dj, _ := json.Marshal(d)
						pv := string(dj)
						if len(pv) > 300 {
							pv = pv[:300] + "..."
						}
						fmt.Printf("   stateDelta.deliverable=%s\n", pv)
					}
				} else {
					fmt.Println("   stateDelta=(none)")
				}
				if chs, ok := ev["choices"].([]any); ok {
					for _, c := range chs {
						cm, _ := c.(map[string]any)
						msg, _ := cm["message"].(map[string]any)
						if msg == nil {
							continue
						}
						toolID, _ := msg["tool_id"].(string)
						toolName, _ := msg["tool_name"].(string)
						content, _ := msg["content"].(string)
						if len(content) > 300 {
							content = content[:300] + "..."
						}
						fmt.Printf("   msg.role=%v toolName=%q toolID=%q content=%s\n",
							msg["role"], toolName, toolID, content)
						if tcs, ok := msg["tool_calls"].([]any); ok {
							for _, tc := range tcs {
								tcm, _ := tc.(map[string]any)
								fn, _ := tcm["function"].(map[string]any)
								args, _ := fn["arguments"].(string)
								if len(args) > 300 {
									args = args[:300] + "..."
								}
								fmt.Printf("   tool_call id=%v name=%v args=%s\n",
									tcm["id"], fn["name"], args)
							}
						}
					}
				}
			}
			evRows.Close()
			if !evFound {
				fmt.Println("   (no set_deliverable events)")
			}
		}
	}
}

// probeSession dumps trpc state + set_deliverable events for one session ID.
func probeSession(db *sql.DB, sessionID string) {
	fmt.Printf("================ SESSION %s ================\n", sessionID)

	rows, err := db.Query(`
		SELECT app_name, user_id, COALESCE(state::text,'')
		FROM trpc_session_states WHERE session_id = $1
	`, sessionID)
	if err != nil {
		fmt.Println("state query err:", err)
		return
	}
	found := false
	for rows.Next() {
		found = true
		var app, user, state string
		if err := rows.Scan(&app, &user, &state); err != nil {
			continue
		}
		fmt.Printf("app=%q user=%q state_len=%d\n", app, user, len(state))
		var m map[string]any
		if err := json.Unmarshal([]byte(state), &m); err != nil {
			continue
		}
		// state payload may nest under "state"
		inner, _ := m["state"].(map[string]any)
		if inner == nil {
			inner = m
		}
		keys := make([]string, 0, len(inner))
		for k := range inner {
			keys = append(keys, k)
		}
		fmt.Printf("state_keys: %v\n", keys)
		if d, ok := inner["deliverable"]; ok {
			// values are base64-encoded JSON in trpc session state
			if s, ok := d.(string); ok {
				fmt.Printf("deliverable_raw_b64_len=%d\n", len(s))
				if dec, err := decodeB64(s); err == nil {
					pv := dec
					if len(pv) > 600 {
						pv = pv[:600] + "..."
					}
					fmt.Printf("deliverable_decoded: %s\n", pv)
				}
			} else {
				dj, _ := json.Marshal(d)
				pv := string(dj)
				if len(pv) > 600 {
					pv = pv[:600] + "..."
				}
				fmt.Printf("deliverable_json: %s\n", pv)
			}
		} else {
			fmt.Println("deliverable: (ABSENT)")
		}
	}
	rows.Close()
	if !found {
		fmt.Println("(no state rows)")
	}

	evRows, err := db.Query(`
		SELECT event::text FROM trpc_session_events
		WHERE session_id = $1 AND event::text LIKE '%set_deliverable%'
		ORDER BY id
	`, sessionID)
	if err != nil {
		fmt.Println("event query err:", err)
		return
	}
	evFound := false
	toolRespWithDelta := 0
	toolRespTotal := 0
	for evRows.Next() {
		evFound = true
		var evText string
		if err := evRows.Scan(&evText); err != nil {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(evText), &ev); err != nil {
			continue
		}
		isToolResp := false
		if chs, ok := ev["choices"].([]any); ok {
			for _, c := range chs {
				cm, _ := c.(map[string]any)
				msg, _ := cm["message"].(map[string]any)
				if msg == nil {
					continue
				}
				if msg["role"] == "tool" && msg["tool_name"] == "set_deliverable" {
					isToolResp = true
				}
			}
		}
		if !isToolResp {
			continue
		}
		toolRespTotal++
		sd, _ := ev["stateDelta"].(map[string]any)
		if len(sd) == 0 {
			fmt.Println("tool_response: stateDelta=(NONE)  <-- pre-fix symptom")
			continue
		}
		keys := make([]string, 0, len(sd))
		for k := range sd {
			keys = append(keys, k)
		}
		fmt.Printf("tool_response: stateDelta_keys=%v\n", keys)
		if raw, ok := sd["deliverable"]; ok {
			toolRespWithDelta++
			if s, ok := raw.(string); ok {
				if dec, err := decodeB64(s); err == nil {
					pv := dec
					if len(pv) > 400 {
						pv = pv[:400] + "..."
					}
					fmt.Printf("  stateDelta.deliverable_decoded: %s\n", pv)
				}
			} else {
				dj, _ := json.Marshal(raw)
				pv := string(dj)
				if len(pv) > 400 {
					pv = pv[:400] + "..."
				}
				fmt.Printf("  stateDelta.deliverable: %s\n", pv)
			}
		}
	}
	evRows.Close()
	if !evFound {
		fmt.Println("(no set_deliverable events)")
	}
	fmt.Printf("SUMMARY: set_deliverable tool responses with deliverable stateDelta: %d/%d\n",
		toolRespWithDelta, toolRespTotal)
}

// listRuns lists team_runs rows for a team.
func listRuns(db *sql.DB, teamID string) {
	rows, err := db.Query(`
		SELECT id, COALESCE(status,''), COALESCE(mode,''),
			COALESCE(CAST(started_at AS TEXT),''),
			COALESCE(CAST(finished_at AS TEXT),''),
			COALESCE(session_id,'')
		FROM team_runs WHERE team_id = $1 ORDER BY started_at
	`, teamID)
	if err != nil {
		fmt.Println("runs query err:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, status, mode, started, finished, sess string
		if err := rows.Scan(&id, &status, &mode, &started, &finished, &sess); err != nil {
			continue
		}
		fmt.Printf("run=%s status=%s mode=%s started=%s finished=%s session=%s\n",
			id, status, mode, started, finished, sess)
	}
}

// dumpDeliverableEvents prints full set_deliverable events (content + args) for one session.
func dumpDeliverableEvents(db *sql.DB, sessID string) {
	rows, err := db.Query(`
		SELECT id, event::text
		FROM trpc_session_events
		WHERE session_id = $1 AND event::text LIKE '%set_deliverable%'
		ORDER BY id
	`, sessID)
	if err != nil {
		fmt.Println("query err:", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var evText string
		if err := rows.Scan(&id, &evText); err != nil {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(evText), &ev); err != nil {
			continue
		}
		fmt.Printf("=== event row=%d author=%v done=%v\n", id, ev["author"], ev["done"])
		if sd, ok := ev["stateDelta"].(map[string]any); ok && len(sd) > 0 {
			keys := []string{}
			for k := range sd {
				keys = append(keys, k)
			}
			fmt.Printf("  stateDelta_keys=%v\n", keys)
		} else {
			fmt.Println("  stateDelta=(none)")
		}
		if chs, ok := ev["choices"].([]any); ok {
			for _, c := range chs {
				cm, _ := c.(map[string]any)
				msg, _ := cm["message"].(map[string]any)
				if msg == nil {
					continue
				}
				content, _ := msg["content"].(string)
				fmt.Printf("  role=%v tool_name=%v content_len=%d\n", msg["role"], msg["tool_name"], len(content))
				if len(content) > 0 {
					pv := content
					if len(pv) > 300 {
						pv = pv[:300] + " ...TAIL... " + content[len(content)-200:]
					}
					fmt.Printf("  content: %s\n", pv)
				}
				if tcs, ok := msg["tool_calls"].([]any); ok {
					for _, tc := range tcs {
						tcm, _ := tc.(map[string]any)
						fn, _ := tcm["function"].(map[string]any)
						args, _ := fn["arguments"].(string)
						fmt.Printf("  tool_call name=%v args_len=%d\n", fn["name"], len(args))
						if len(args) > 0 {
							pv := args
							if len(pv) > 400 {
								pv = pv[:400] + "..."
							}
							fmt.Printf("  args: %s\n", pv)
						}
					}
				}
			}
		}
	}
}

func decodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
