package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type activity struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	AgentKey string `json:"agentKey"`
	TeamID   string `json:"teamId"`
	MetaJSON string `json:"metaJson"`
}

type resp struct {
	Activities []activity `json:"items"`
}

func main() {
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	var r resp
	if err := json.Unmarshal(b, &r); err != nil {
		fmt.Fprintln(os.Stderr, "unmarshal:", err)
		os.Exit(1)
	}
	out, _ := os.Create("f:\\aranea-agents\\tmp_session_meta.txt")
	defer out.Close()
	for _, a := range r.Activities {
		if a.Kind != "session" {
			continue
		}
		fmt.Fprintf(out, "=== session activity ===\n")
		fmt.Fprintf(out, "id=%s agentKey=%s teamId=%s\n", a.ID, a.AgentKey, a.TeamID)
		// Parse metaJson
		if a.MetaJSON == "" {
			fmt.Fprintf(out, "  meta: EMPTY\n")
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(a.MetaJSON), &m); err != nil {
			fmt.Fprintf(out, "  meta parse error: %v\n", err)
			continue
		}
		if csid, ok := m["child_session_id"]; ok {
			fmt.Fprintf(out, "  child_session_id: %v\n", csid)
		} else {
			fmt.Fprintf(out, "  child_session_id: MISSING\n")
		}
		// Print all meta keys
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		fmt.Fprintf(out, "  meta keys: %v\n", keys)
	}
}
