package data

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupMonitorTraceNameRepo builds a Data with full Ent schema (agents/teams)
// plus the raw-SQL monitor_traces table mirroring production DDL.
func setupMonitorTraceNameRepo(t *testing.T) *monitorRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_traces (
		id TEXT PRIMARY KEY,
		trace_key TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'running',
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		deleted_at TEXT NOT NULL DEFAULT '',
		session_id TEXT NOT NULL DEFAULT '',
		run_id TEXT NOT NULL DEFAULT '',
		invocation_id TEXT NOT NULL DEFAULT '',
		agent_id TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		team_id TEXT NOT NULL DEFAULT '',
		parent_trace_id TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		span_count INTEGER NOT NULL DEFAULT 0,
		error_count INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		total_cost_usd REAL NOT NULL DEFAULT 0.0
	)`); err != nil {
		t.Fatalf("create monitor_traces: %v", err)
	}
	d := &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		rawDB:      db,
		readDB:     db,
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
	}
	return &monitorRepo{data: d}
}

func seedAgentAndTeam(t *testing.T, r *monitorRepo) {
	t.Helper()
	ctx := context.Background()
	if _, err := r.data.entClient.Agent.Create().
		SetID("ag-1").SetAgentKey("code-helper").SetDisplayName("编码小助手").
		SetProvider("openai").SetModel("gpt-4o").
		Save(ctx); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	if _, err := r.data.entClient.Team.Create().
		SetID("tm-1").SetTeamKey("market-research").SetDisplayName("市场调研团队").
		Save(ctx); err != nil {
		t.Fatalf("seed team: %v", err)
	}
}

func insertTraceRow(t *testing.T, r *monitorRepo, tw biz.MonitorTraceWrite) {
	t.Helper()
	if err := r.InsertMonitorTrace(context.Background(), tw); err != nil {
		t.Fatalf("insert trace %s: %v", tw.TraceID, err)
	}
}

func TestListMonitorTraces_ResolvesAgentTeamDisplayNames(t *testing.T) {
	r := setupMonitorTraceNameRepo(t)
	seedAgentAndTeam(t, r)

	// Team run: name resolves to team display name.
	insertTraceRow(t, r, biz.MonitorTraceWrite{TraceID: "tr-team", Name: "team", AgentID: "ag-1", TeamID: "tm-1"})
	// Agent run: name resolves to agent display name.
	insertTraceRow(t, r, biz.MonitorTraceWrite{TraceID: "tr-agent", Name: "chat", AgentID: "ag-1"})
	// Unknown refs: falls back to stored domain.
	insertTraceRow(t, r, biz.MonitorTraceWrite{TraceID: "tr-orphan", Name: "graph", AgentID: "ag-gone", TeamID: "tm-gone"})

	res, err := r.ListMonitorTraces(context.Background(), biz.MonitorTracesQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(res.Items))
	}
	byID := map[string]biz.MonitorPlatformRow{}
	for _, it := range res.Items {
		byID[it.Key] = it
	}

	team := byID["tr-team"]
	if team.AgentName != "编码小助手" {
		t.Errorf("team row AgentName = %q, want 编码小助手", team.AgentName)
	}
	if team.TeamName != "市场调研团队" {
		t.Errorf("team row TeamName = %q, want 市场调研团队", team.TeamName)
	}
	if team.Name != "市场调研团队" {
		t.Errorf("team row Name = %q, want resolved team display name", team.Name)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(team.ConfigJSON), &cfg); err != nil {
		t.Fatalf("config_json unmarshal: %v", err)
	}
	if cfg["domain"] != "team" {
		t.Errorf("config_json.domain = %v, want original stored domain %q", cfg["domain"], "team")
	}

	agent := byID["tr-agent"]
	if agent.Name != "编码小助手" {
		t.Errorf("agent row Name = %q, want resolved agent display name", agent.Name)
	}
	if agent.TeamName != "" {
		t.Errorf("agent row TeamName = %q, want empty", agent.TeamName)
	}

	orphan := byID["tr-orphan"]
	if orphan.Name != "graph" {
		t.Errorf("orphan row Name = %q, want stored domain fallback %q", orphan.Name, "graph")
	}
	if orphan.AgentName != "" || orphan.TeamName != "" {
		t.Errorf("orphan row names = (%q, %q), want empty", orphan.AgentName, orphan.TeamName)
	}
}

func TestGetMonitorTrace_ResolvesDisplayNames(t *testing.T) {
	r := setupMonitorTraceNameRepo(t)
	seedAgentAndTeam(t, r)
	insertTraceRow(t, r, biz.MonitorTraceWrite{TraceID: "tr-get", Name: "team", AgentID: "ag-1", TeamID: "tm-1"})

	row, err := r.GetMonitorTrace(context.Background(), "tr-get")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Name != "市场调研团队" {
		t.Errorf("Name = %q, want resolved team display name", row.Name)
	}
	if row.AgentName != "编码小助手" || row.TeamName != "市场调研团队" {
		t.Errorf("AgentName/TeamName = (%q, %q), want (编码小助手, 市场调研团队)", row.AgentName, row.TeamName)
	}
}
