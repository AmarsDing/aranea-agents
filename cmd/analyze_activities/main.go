package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type activity struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	SessionID        string `json:"sessionId"`
	ParentActivityID string `json:"parentActivityId"`
	SpiritSessionID  string `json:"spiritSessionId"`
	TeamID           string `json:"teamId"`
	AgentKey         string `json:"agentKey"`
	AgentName        string `json:"agentName"`
	Timestamp        string `json:"timestamp"`
	Stage            string `json:"stage"`
	MetaJSON         string `json:"metaJson"`
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
	out, _ := os.Create("f:\\aranea-agents\\tmp_analysis_output.txt")
	defer out.Close()
	fmt.Fprintf(out, "Total: %d\n", len(r.Activities))

	// Count by kind
	counts := map[string]int{}
	for _, a := range r.Activities {
		counts[a.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(out, "  %s: %d\n", k, counts[k])
	}

	// Show team_stage and session activities (the tree we care about)
	fmt.Fprintln(out, "\n=== team_stage activities ===")
	for _, a := range r.Activities {
		if a.Kind == "team_stage" {
			fmt.Fprintf(out, "id=%s\n  status=%s stage=%s\n  session=%s spirit=%s teamId=%s\n  parent=%s\n  ts=%s\n",
				a.ID, a.Status, a.Stage, a.SessionID, a.SpiritSessionID, a.TeamID, a.ParentActivityID, a.Timestamp)
		}
	}

	// Show ALL activities with their spiritSessionId to debug sub-session inclusion
	fmt.Fprintln(out, "\n=== ALL activities (spiritSessionId check) ===")
	for _, a := range r.Activities {
		fmt.Fprintf(out, "kind=%-12s id=%s\n  session=%s spirit=%s parent=%s\n",
			a.Kind, a.ID, a.SessionID, a.SpiritSessionID, a.ParentActivityID)
	}

	fmt.Fprintln(out, "\n=== session activities ===")
	for _, a := range r.Activities {
		if a.Kind == "session" {
			fmt.Fprintf(out, "id=%s\n  status=%s stage=%s\n  session=%s spirit=%s teamId=%s agentKey=%s\n  parent=%s\n  ts=%s\n",
				a.ID, a.Status, a.Stage, a.SessionID, a.SpiritSessionID, a.TeamID, a.AgentKey, a.ParentActivityID, a.Timestamp)
		}
	}

	// Verify parent-child relationship: session.parentActivityId should match team_stage.id
	fmt.Fprintln(out, "\n=== parent-child link check ===")
	teamStageIDs := map[string]string{} // id -> teamId
	for _, a := range r.Activities {
		if a.Kind == "team_stage" {
			teamStageIDs[a.ID] = a.TeamID
		}
	}
	for _, a := range r.Activities {
		if a.Kind == "session" {
			_, ok := teamStageIDs[a.ParentActivityID]
			link := "OK"
			if !ok {
				link = "BROKEN (parent not in team_stage set)"
			}
			fmt.Fprintf(out, "session %s (agent=%s) -> parent %s [%s]\n", a.ID, a.AgentKey, a.ParentActivityID, link)
		}
	}
}
