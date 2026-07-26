package main

import (
	"database/sql"
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
