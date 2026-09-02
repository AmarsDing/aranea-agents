package data

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"aranea-agents/internal/biz/pack"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// twinMonitorAgentKey 是 twinmonitor-pack 定义的唯一 Agent key。
const twinMonitorAgentKey = "twin_butler__general"

// twinMonitorWantTools 钉死 twin_butler 的只读工具白名单——防 pack yaml 被误改
// 引入写操作工具（twin_alarm_ack/twin_config_push/twin_config_rollback 等）。
var twinMonitorWantTools = []string{
	"twin_alarm_query",
	"twin_alarm_get",
	"twin_alarm_rule_get",
	"twin_device_search",
	"twin_device_get",
	"twin_device_metrics",
	"twin_collector_status",
	"twin_line_status",
	"twin_line_events",
	"twin_inspection_query",
	"twin_remediation_status",
	"twin_arrival_overview",
	"twin_arrival_status",
	"twin_notice_records",
	"twin_report_tasks",
	"twin_kb_search",
	"knowledge_search",
	"datetime",
}

// TestSeedPackTwinMonitorVersionUniqueness guards against seed version colliding
// with DDL/data migration versions that share the schema_migrations table.
func TestSeedPackTwinMonitorVersionUniqueness(t *testing.T) {
	if SeedPackTwinMonitorV1 <= MigrationMonitorTraceInterruptedBackfill {
		t.Errorf("SeedPackTwinMonitorV1=%d must be greater than the latest DDL migration version %d (schema_migrations versions are globally unique)",
			SeedPackTwinMonitorV1, MigrationMonitorTraceInterruptedBackfill)
	}
	if SeedPackTwinMonitorV1 <= SeedPackItOpsV1 {
		t.Errorf("SeedPackTwinMonitorV1=%d must be greater than SeedPackItOpsV1=%d",
			SeedPackTwinMonitorV1, SeedPackItOpsV1)
	}
}

// TestTwinMonitorPackReadable 无需 PG 的 pack 结构校验：从 scenario 目录读取
// twinmonitor-pack，验证 manifest/agent 定义/提示词文件/只读工具白名单齐全。
// 防 pack 文件漂移（改名/删文件/误挂写工具）在 seed 之前即暴露。
func TestTwinMonitorPackReadable(t *testing.T) {
	fsys := os.DirFS("../")
	p, err := pack.ReadPackFromFS(fsys, "scenario/packs/twinmonitor-pack")
	if err != nil {
		t.Fatalf("ReadPackFromFS twinmonitor-pack: %v", err)
	}

	if p.Manifest.Name != "twinmonitor-pack" {
		t.Errorf("manifest.name = %q, want twinmonitor-pack", p.Manifest.Name)
	}
	if len(p.Agents) != 1 {
		t.Fatalf("pack agents count = %d, want 1 (twin_butler__general)", len(p.Agents))
	}
	spec := p.Agents[0]
	if spec.Key != twinMonitorAgentKey {
		t.Errorf("agent key = %q, want %q", spec.Key, twinMonitorAgentKey)
	}

	// 工具白名单：与 twinMonitorWantTools 精确一致（顺序无关），且无写操作工具
	if len(spec.ToolsAllow) != len(twinMonitorWantTools) {
		t.Errorf("tools_allow count = %d, want %d (got %v)", len(spec.ToolsAllow), len(twinMonitorWantTools), spec.ToolsAllow)
	}
	allowSet := make(map[string]bool, len(spec.ToolsAllow))
	for _, tool := range spec.ToolsAllow {
		allowSet[tool] = true
	}
	for _, want := range twinMonitorWantTools {
		if !allowSet[want] {
			t.Errorf("pack tools_allow missing %q", want)
		}
	}
	for _, banned := range []string{"twin_alarm_ack", "twin_config_push", "twin_config_rollback", "gns3_fault_inject", "gns3_fault_clear"} {
		if allowSet[banned] {
			t.Errorf("pack tools_allow must NOT contain write tool %q (read-only boundary violated)", banned)
		}
	}

	// 提示词文件：yaml 声明 3 个且内容均已读入
	if len(spec.Files) != 3 {
		t.Errorf("agent files count = %d, want 3 (IDENTITY/MISSION/RULE)", len(spec.Files))
	}
	files := p.AgentFiles[twinMonitorAgentKey]
	for _, name := range []string{"IDENTITY.md", "MISSION.md", "RULE.md"} {
		if strings.TrimSpace(files[name]) == "" {
			t.Errorf("prompt file %q missing or empty in pack AgentFiles", name)
		}
	}
}

// TestSeedPackTwinMonitorIdempotent verifies that running the twinmonitor pack
// seed multiple times does not create duplicate agents or prompt files.
func TestSeedPackTwinMonitorIdempotent(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	client, _ := testhelper.SetupTestPG(t)

	d := newDataFromClient(client, lg)
	scenarioDir := "../scenario"

	for i := 0; i < 3; i++ {
		if err := SeedPackTwinMonitor(ctx, client, d.Dialect(), scenarioDir, lg); err != nil {
			t.Fatalf("startup %d SeedPackTwinMonitor: %v", i+1, err)
		}
	}

	// 1. twin_butler__general 存在且不重复
	agents, err := client.Agent.Query().All(ctx)
	if err != nil {
		t.Fatalf("query agents: %v", err)
	}
	count := 0
	var agentID string
	for _, a := range agents {
		if a.AgentKey == twinMonitorAgentKey {
			count++
			agentID = a.ID
		}
	}
	if count == 0 {
		t.Fatalf("twinmonitor agent %q not found after seeding", twinMonitorAgentKey)
	}
	if count > 1 {
		t.Errorf("twinmonitor agent %q appears %d times after repeated seeding", twinMonitorAgentKey, count)
	}

	// 2. 提示词文件（IDENTITY/MISSION/RULE）已导入且不重复
	files, err := client.AgentPromptFile.Query().All(ctx)
	if err != nil {
		t.Fatalf("query agent prompt files: %v", err)
	}
	fileNames := make(map[string]int)
	for _, f := range files {
		if f.AgentID == agentID {
			fileNames[f.FileName]++
		}
	}
	for _, want := range []string{"IDENTITY.md", "MISSION.md", "RULE.md"} {
		if fileNames[want] == 0 {
			t.Errorf("twinmonitor agent %q missing prompt file %q after seeding", twinMonitorAgentKey, want)
		}
		if fileNames[want] > 1 {
			t.Errorf("twinmonitor agent %q prompt file %q appears %d times after repeated seeding", twinMonitorAgentKey, want, fileNames[want])
		}
	}

	// 3. 只读工具白名单与 pack 定义一致（防误挂写操作工具——工具面安全红线）
	settings, err := client.AgentRuntimeSetting.Query().All(ctx)
	if err != nil {
		t.Fatalf("query agent runtime settings: %v", err)
	}
	var allowJSON string
	found := false
	for _, s := range settings {
		if s.ID == agentID { // agent_runtime_settings 主键 id 的 StorageKey 为 agent_id
			allowJSON = s.ToolsAllowJSON
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("twinmonitor agent %q has no runtime settings row after seeding", twinMonitorAgentKey)
	}
	var gotTools []string
	if err := json.Unmarshal([]byte(allowJSON), &gotTools); err != nil {
		t.Fatalf("unmarshal tools_allow_json %q: %v", allowJSON, err)
	}
	gotSet := make(map[string]bool, len(gotTools))
	for _, tool := range gotTools {
		gotSet[tool] = true
	}
	if len(gotTools) != len(twinMonitorWantTools) {
		t.Errorf("twinmonitor agent tools count = %d, want %d (got %v)", len(gotTools), len(twinMonitorWantTools), gotTools)
	}
	for _, want := range twinMonitorWantTools {
		if !gotSet[want] {
			t.Errorf("twinmonitor agent missing read-only tool %q in tools_allow", want)
		}
	}
	// 反向红线：写操作工具绝不允许出现在白名单
	for _, banned := range []string{"twin_alarm_ack", "twin_config_push", "twin_config_rollback", "gns3_fault_inject", "gns3_fault_clear"} {
		if gotSet[banned] {
			t.Errorf("twinmonitor agent must NOT have write tool %q in tools_allow (read-only boundary violated)", banned)
		}
	}
}
