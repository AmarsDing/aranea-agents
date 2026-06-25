package team

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

// parityMemberOutcome is deterministic per-member execution data for run-level parity.
type parityMemberOutcome struct {
	AgentID       string
	SortOrder     int
	Role          string
	TokenIn       int
	TokenOut      int
	ToolCallCount int
	Output        string
	Status        string
}

func parityMemberOutcomes(def Definition) []parityMemberOutcome {
	members := EnabledMembers(def)
	out := make([]parityMemberOutcome, 0, len(members))
	for i, m := range members {
		sortOrder := m.SortOrder
		if sortOrder <= 0 {
			sortOrder = i + 1
		}
		role := strings.TrimSpace(m.Role)
		if role == "" {
			role = "worker"
		}
		toolCalls := sortOrder // deterministic spread
		out = append(out, parityMemberOutcome{
			AgentID:       m.AgentID,
			SortOrder:     sortOrder,
			Role:          role,
			TokenIn:       100 + sortOrder*10,
			TokenOut:      50 + sortOrder*5,
			ToolCallCount: toolCalls,
			Output:        fmt.Sprintf("reply from %s in %s mode", m.AgentID, def.Mode),
			Status:        biz.TeamMemberStepStatusOK,
		})
	}
	return out
}

func parityRunBase(mode string) biz.TeamRun {
	return biz.TeamRun{
		ID:        "run-parity-" + mode,
		TeamID:    "team-parity",
		SessionID: "sess-parity",
		Mode:      mode,
		Status:    biz.TeamRunStatusSuccess,
	}
}

// buildGraphEventSteps simulates Graph path: one step per executed graph member node.
func buildGraphEventSteps(run biz.TeamRun, teamID string, def Definition, outcomes []parityMemberOutcome) []biz.TeamRunStep {
	steps := make([]biz.TeamRunStep, 0, len(outcomes))
	for i, o := range outcomes {
		nodeID := fmt.Sprintf("member-%d", o.SortOrder)
		steps = append(steps, biz.TeamRunStep{
			ID:            fmt.Sprintf("step-graph-%s", nodeID),
			RunID:         run.ID,
			TeamID:        teamID,
			AgentID:       o.AgentID,
			AgentKey:      compileAgentKey(o.AgentID),
			AgentName:     o.AgentID,
			Role:          o.Role,
			SortOrder:     i,
			Status:        o.Status,
			OutputPreview: o.Output,
			TokenIn:       o.TokenIn,
			TokenOut:      o.TokenOut,
			ToolCallCount: o.ToolCallCount,
		})
		_ = def.Mode
	}
	return steps
}

type parityRunReport struct {
	Mode             string
	MemberCount      int
	ToolCallTotal    int
	StepAgentKeys    []string
	SummaryAgentKeys []string
	GraphTokenIn     int
	GraphTokenOut    int
	GraphSummaryHash string
}

func memberSummaryKeys(data biz.TeamRunSummaryData) []string {
	keys := make([]string, 0, len(data.Members))
	for _, m := range data.Members {
		keys = append(keys, m.AgentKey)
	}
	sort.Strings(keys)
	return keys
}

func summaryMemberFingerprint(data biz.TeamRunSummaryData) string {
	parts := make([]string, 0, len(data.Members))
	for _, m := range data.Members {
		parts = append(parts, fmt.Sprintf("%s:%d:%d:%s", m.AgentKey, m.ToolCallCount, m.TokenIn+m.TokenOut, m.Status))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func hashSummaryMembers(data biz.TeamRunSummaryData) string {
	h := sha256.Sum256([]byte(summaryMemberFingerprint(data)))
	return hex.EncodeToString(h[:8])
}

func compareParityRunSummaries(t *testing.T, mode string, def Definition, outcomes []parityMemberOutcome) parityRunReport {
	t.Helper()
	run := parityRunBase(mode)
	graphSteps := buildGraphEventSteps(run, run.TeamID, def, outcomes)

	graphRun := run
	enrichTeamRunMetricsFromSteps(&graphRun, graphSteps)

	graphData := biz.BuildTeamRunSummaryData(graphRun, graphSteps)

	report := parityRunReport{
		Mode:             mode,
		MemberCount:      graphData.MemberCount,
		ToolCallTotal:    graphData.ToolCallCount,
		StepAgentKeys:    memberSummaryKeys(graphData),
		SummaryAgentKeys: memberSummaryKeys(graphData),
		GraphTokenIn:     graphData.TokenIn,
		GraphTokenOut:    graphData.TokenOut,
		GraphSummaryHash: hashSummaryMembers(graphData),
	}

	if report.MemberCount != len(outcomes) {
		t.Fatalf("%s member_count: graph=%d want=%d", mode, report.MemberCount, len(outcomes))
	}

	graphMap := SummaryMapFromData(graphData)
	if mc, _ := graphMap["member_count"].(int); mc != report.MemberCount {
		t.Fatalf("%s WS member_count mismatch got=%d want=%d", mode, mc, report.MemberCount)
	}

	return report
}

// TestParityRunSummary_AllModes verifies team_summary member-level data from Graph event-driven
// step fixtures (TG-RT-PARITY run-level harness).
func TestParityRunSummary_AllModes(t *testing.T) {
	modes := []string{"sequential", "parallel", "coordinator", "critic_loop", "adaptive", "swarm"}
	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			def := parityFixture(mode)
			outcomes := parityMemberOutcomes(def)
			if len(outcomes) == 0 {
				t.Fatal("no member outcomes")
			}
			report := compareParityRunSummaries(t, mode, def, outcomes)
			if report.MemberCount != len(outcomes) {
				t.Fatalf("member_count=%d want %d", report.MemberCount, len(outcomes))
			}
			t.Logf("parity ok mode=%s members=%d tools=%d fingerprint=%s tokens graph=%d/%d",
				mode, report.MemberCount, report.ToolCallTotal, report.GraphSummaryHash,
				report.GraphTokenIn, report.GraphTokenOut)
		})
	}
}

// graphOnlyEnvelopeTypes are WS types emitted on Graph path (documented).
var graphOnlyEnvelopeTypes = []event.EnvelopeType{
	event.EnvelopeTypeGraphNodeStart,
	event.EnvelopeTypeGraphNodeEnd,
	event.EnvelopeTypeGraphNodeError,
	event.EnvelopeTypeGraphExecutionDone,
	event.EnvelopeTypeOrchestrationAgentStatus,
}

func TestParityRunEnvelopeDiff_documented(t *testing.T) {
	graphSet := envelopeTypeSet(graphOnlyEnvelopeTypes)
	shared := []event.EnvelopeType{
		event.EnvelopeTypeTeamSummary,
		event.EnvelopeTypeTeamRunFinished,
	}
	for _, typ := range shared {
		if _, ok := graphSet[typ]; ok {
			t.Fatalf("%q should not be graph-only", typ)
		}
	}
}

func envelopeTypeSet(types []event.EnvelopeType) map[event.EnvelopeType]struct{} {
	out := make(map[event.EnvelopeType]struct{}, len(types))
	for _, typ := range types {
		out[typ] = struct{}{}
	}
	return out
}

func intersectEnvelopeSets(a, b map[event.EnvelopeType]struct{}) []event.EnvelopeType {
	var out []event.EnvelopeType
	for k := range a {
		if _, ok := b[k]; ok {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
