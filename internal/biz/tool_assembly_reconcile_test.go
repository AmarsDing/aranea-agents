package biz

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

// --- fakes（仅实现 ReconcileToolAssembly 消费的最小面） ---

type reconcileFakeAgentReader struct {
	agents []Agent
}

func (f reconcileFakeAgentReader) SearchAgents(_ context.Context, _ AgentListQuery) (AgentListResult, error) {
	return AgentListResult{Items: f.agents, Total: len(f.agents)}, nil
}

func (f reconcileFakeAgentReader) GetAgentByID(_ context.Context, id string) (Agent, error) {
	for _, a := range f.agents {
		if a.ID == id {
			return a, nil
		}
	}
	return Agent{}, shared.ErrNotFound
}

func (f reconcileFakeAgentReader) GetAgentByAgentKey(_ context.Context, agentKey string) (Agent, error) {
	for _, a := range f.agents {
		if a.AgentKey == agentKey {
			return a, nil
		}
	}
	return Agent{}, shared.ErrNotFound
}

func (f reconcileFakeAgentReader) ListExtrasForAgents(_ context.Context, _ []string) (map[string]AgentListExtras, error) {
	return map[string]AgentListExtras{}, nil
}

func (f reconcileFakeAgentReader) ListAgentsByIDs(_ context.Context, ids []string) ([]Agent, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Agent
	for _, a := range f.agents {
		if want[a.ID] {
			out = append(out, a)
		}
	}
	return out, nil
}

type reconcileFakeToolRegistry struct {
	catalog   []Tool
	overrides map[string][]ToolAgentOverride
}

func (f reconcileFakeToolRegistry) SearchTools(_ context.Context, _ ToolListQuery) (ToolListResult, error) {
	return ToolListResult{Items: f.catalog, Total: len(f.catalog)}, nil
}

func (f reconcileFakeToolRegistry) GetTool(_ context.Context, idOrKey string) (Tool, error) {
	for _, t := range f.catalog {
		if t.Key == idOrKey || t.ID == idOrKey {
			return t, nil
		}
	}
	return Tool{}, shared.ErrNotFound
}

func (f reconcileFakeToolRegistry) ListToolCatalogEntries(_ context.Context, _ []string) ([]ToolCatalogEntry, error) {
	return nil, nil
}

func (f reconcileFakeToolRegistry) ListToolAgentOverrides(_ context.Context, _ string) ([]ToolAgentOverride, error) {
	return nil, nil
}

func (f reconcileFakeToolRegistry) ListToolAgentOverridesByAgent(_ context.Context, agentID string) ([]ToolAgentOverride, error) {
	return f.overrides[agentID], nil
}

type reconcileFakeSettingsRepo struct {
	rows map[string]AgentRuntimeSettings
}

func (f reconcileFakeSettingsRepo) GetAgentRuntimeSettings(_ context.Context, agentID string) (AgentRuntimeSettings, error) {
	if s, ok := f.rows[agentID]; ok {
		return s, nil
	}
	return AgentRuntimeSettings{}, shared.ErrNotFound
}

func (f reconcileFakeSettingsRepo) ListAgentRuntimeSettings(_ context.Context) (map[string]AgentRuntimeSettings, error) {
	return f.rows, nil
}

func (f reconcileFakeSettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error) {
	return v, nil
}

func newReconcileTestUsecase(agents []Agent, catalog []Tool, settings map[string]AgentRuntimeSettings) *AgentUsecase {
	return NewAgentUsecase(AgentUsecaseDeps{
		Reader:   reconcileFakeAgentReader{agents: agents},
		Tools:    reconcileFakeToolRegistry{catalog: catalog},
		Settings: reconcileFakeSettingsRepo{rows: settings},
		Lg:       loggateway.NewNoop(),
	})
}

// reconcileIssueCodes 汇总指定 agent 命中的 issue code → severity（agent_key 为
// 空的目录层 issue 传 "" 查询）。
func reconcileIssueCodes(report ToolAssemblyReport, agentKey string) map[string]string {
	out := map[string]string{}
	for _, is := range report.Issues {
		if is.AgentKey == agentKey {
			out[is.Code] = is.Severity
		}
	}
	return out
}

func reconcileFindRow(report ToolAssemblyReport, agentKey string) ToolAssemblyAgentRow {
	for _, r := range report.Agents {
		if r.AgentKey == agentKey {
			return r
		}
	}
	return ToolAssemblyAgentRow{}
}

// reconcileHealthyCatalog 提供 read_only/coding 常用的目录面（>3 个可用工具），
// shell_exec / mcp_broker 为 registryOptInOnlyKeys 成员（enabled=false 非死工具）。
func reconcileHealthyCatalog() []Tool {
	return []Tool{
		{Key: "datetime", Category: "utility", Source: "builtin", Enabled: true},
		{Key: "read_file", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "read_multiple_files", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "list_file", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "search_file", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "search_content", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "todo_write", Category: "session", Source: "builtin", Enabled: true},
		{Key: "save_file", Category: "filesystem", Source: "builtin", Enabled: true},
		{Key: "shell_exec", Category: "runtime", Source: "builtin", Enabled: false, RiskLevel: "critical"},
		{Key: "knowledge_search", Category: "integration", Source: "builtin", Enabled: true},
		{Key: "mcp_broker", Category: "integration", Source: "builtin", Enabled: false},
	}
}

// 健康 coding agent：>3 个有效工具，无任何 issue。
func TestReconcileToolAssembly_healthyCodingAgent(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: "worker", DisplayName: "Worker"}
	settings := map[string]AgentRuntimeSettings{
		"a1": {AgentID: "a1", ToolsEnabled: true, ToolsProfile: "coding", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.AgentsChecked != 1 {
		t.Fatalf("AgentsChecked=%d want 1", report.AgentsChecked)
	}
	if codes := reconcileIssueCodes(report, "worker"); len(codes) != 0 {
		t.Fatalf("healthy agent must have no issues, got %v", codes)
	}
	row := reconcileFindRow(report, "worker")
	if row.ProfileEff != "coding" || row.SettingsSrc != "db" || !row.ToolsEnabled {
		t.Fatalf("unexpected row: %+v", row)
	}
	for _, key := range []string{"shell_exec", "knowledge_search", "mcp_broker", "read_file"} {
		found := false
		for _, k := range row.EffectiveKeys {
			if k == key {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("effective keys must include %s, got %v", key, row.EffectiveKeys)
		}
	}
}

// 无 runtime_settings 行：从 config_json legacy 迁移，记 LOW NO_SETTINGS_ROW。
func TestReconcileToolAssembly_noSettingsRowLegacy(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: "legacy-worker", ConfigJSON: `{"tools":{"enabled":true,"profile":"read_only","deny":[]}}`}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), map[string]AgentRuntimeSettings{})

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	codes := reconcileIssueCodes(report, "legacy-worker")
	if codes[ToolAssemblyCodeNoSettingsRow] != ToolAssemblySeverityLow {
		t.Fatalf("want LOW NO_SETTINGS_ROW, got %v", codes)
	}
	row := reconcileFindRow(report, "legacy-worker")
	if row.SettingsSrc != "legacy" {
		t.Fatalf("SettingsSrc=%q want legacy", row.SettingsSrc)
	}
	// legacy read_only 迁移后有效工具面应与 db read_only 一致（7 个），不触发 ZERO/FEW。
	if _, ok := codes[ToolAssemblyCodeZeroTools]; ok {
		t.Fatalf("legacy read_only must not be ZERO_TOOLS, got %v", codes)
	}
	if _, ok := codes[ToolAssemblyCodeFewTools]; ok {
		t.Fatalf("legacy read_only must not be FEW_TOOLS, got %v", codes)
	}
}

// 专家岗持久化 profile=spirit：HIGH SPIRIT_PROFILE，运行时钳制为 coding。
func TestReconcileToolAssembly_spiritProfileOnSpecialist(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: "specialist", DisplayName: "Specialist"}
	settings := map[string]AgentRuntimeSettings{
		"a1": {AgentID: "a1", ToolsEnabled: true, ToolsProfile: "spirit", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	codes := reconcileIssueCodes(report, "specialist")
	if codes[ToolAssemblyCodeSpiritProfile] != ToolAssemblySeverityHigh {
		t.Fatalf("want HIGH SPIRIT_PROFILE, got %v", codes)
	}
	row := reconcileFindRow(report, "specialist")
	if row.ProfileRaw != "spirit" || row.ProfileEff != "coding" || row.Clamped != "spirit->coding" {
		t.Fatalf("unexpected clamp row: %+v", row)
	}
}

// TOOLS_OFF / ZERO_TOOLS / FEW_TOOLS 三档计数门禁。
func TestReconcileToolAssembly_toolsCountGates(t *testing.T) {
	agents := []Agent{
		{ID: "off", AgentKey: "agent-off"},
		{ID: "zero", AgentKey: "agent-zero"},
		{ID: "few", AgentKey: "agent-few"},
	}
	settings := map[string]AgentRuntimeSettings{
		"off":  {AgentID: "off", ToolsEnabled: false, ToolsProfile: "coding", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
		"zero": {AgentID: "zero", ToolsEnabled: true, ToolsProfile: "chat_only", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
		"few":  {AgentID: "few", ToolsEnabled: true, ToolsProfile: "read_only", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	sparseCatalog := []Tool{
		{Key: "datetime", Category: "utility", Source: "builtin", Enabled: true},
		{Key: "read_file", Category: "filesystem", Source: "builtin", Enabled: true},
	}
	u := newReconcileTestUsecase(agents, sparseCatalog, settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := reconcileIssueCodes(report, "agent-off")[ToolAssemblyCodeToolsOff]; got != ToolAssemblySeverityMid {
		t.Fatalf("agent-off want MID TOOLS_OFF, got %q", got)
	}
	if got := reconcileIssueCodes(report, "agent-zero")[ToolAssemblyCodeZeroTools]; got != ToolAssemblySeverityHigh {
		t.Fatalf("agent-zero want HIGH ZERO_TOOLS, got %q", got)
	}
	fewCodes := reconcileIssueCodes(report, "agent-few")
	if fewCodes[ToolAssemblyCodeFewTools] != ToolAssemblySeverityMid {
		t.Fatalf("agent-few want MID FEW_TOOLS, got %v", fewCodes)
	}
	// TOOLS_OFF 走 switch 首分支，不再叠加 ZERO_TOOLS。
	if codes := reconcileIssueCodes(report, "agent-off"); len(codes) != 1 {
		t.Fatalf("agent-off must only report TOOLS_OFF, got %v", codes)
	}
}

// 治理岗（dept lead）持久化 full：运行时钳制 read_only，记 clamped 注记。
func TestReconcileToolAssembly_govClampedToReadOnly(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: DeptLeadAgentKeyPrefix + "eng__", AgentVariant: "dept_lead"}
	settings := map[string]AgentRuntimeSettings{
		"a1": {AgentID: "a1", ToolsEnabled: true, ToolsProfile: "full", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row := reconcileFindRow(report, DeptLeadAgentKeyPrefix+"eng__")
	if row.ProfileEff != "read_only" || row.Clamped != "gov->read_only" {
		t.Fatalf("gov must clamp to read_only, got %+v", row)
	}
	codes := reconcileIssueCodes(report, DeptLeadAgentKeyPrefix+"eng__")
	if _, ok := codes[ToolAssemblyCodeGovNotReadonly]; ok {
		t.Fatalf("clamped gov must not report GOV_NOT_READONLY, got %v", codes)
	}
	// 治理岗豁免 ZERO/FEW 计数门禁。
	if _, ok := codes[ToolAssemblyCodeZeroTools]; ok {
		t.Fatalf("gov exempt from ZERO_TOOLS, got %v", codes)
	}
}

// Q10（session-eval-20260827，C+A 组合）：治理岗经 allow JSON 授 subagents 四件
// 作分身执行兜底。不变式：profile_eff 仍 read_only（钳制不动 allow）、不报
// GOV_NOT_READONLY、四件全部进有效工具面（种子 enabled=false 但属
// registryOptInOnlyKeys，allow 命名即生效，不被 applyRegistryAdminDenials 硬 deny）。
func TestReconcileToolAssembly_govSubagentAllowKeepsReadOnly(t *testing.T) {
	ag := Agent{ID: "gm1", AgentKey: CompanyLeadAgentKeyPrefix + "acme__", AgentVariant: AgentVariantCompanyLead}
	settings := map[string]AgentRuntimeSettings{
		"gm1": {AgentID: "gm1", ToolsEnabled: true, ToolsProfile: "read_only",
			ToolsAllowJSON: companyLeadSubagentAllowJSON, ToolsDenyJSON: "[]"},
	}
	catalog := append(reconcileHealthyCatalog(),
		Tool{Key: "subagents_spawn", Category: "composition", Source: "builtin", Enabled: false},
		Tool{Key: "subagents_list", Category: "composition", Source: "builtin", Enabled: false},
		Tool{Key: "subagents_get", Category: "composition", Source: "builtin", Enabled: false},
		Tool{Key: "subagents_cancel", Category: "composition", Source: "builtin", Enabled: false},
	)
	u := newReconcileTestUsecase([]Agent{ag}, catalog, settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	row := reconcileFindRow(report, CompanyLeadAgentKeyPrefix+"acme__")
	if row.ProfileEff != "read_only" {
		t.Fatalf("gov with subagent allow must keep read_only effective profile, got %+v", row)
	}
	codes := reconcileIssueCodes(report, CompanyLeadAgentKeyPrefix+"acme__")
	if _, ok := codes[ToolAssemblyCodeGovNotReadonly]; ok {
		t.Fatalf("subagent allow must not trip GOV_NOT_READONLY, got %v", codes)
	}
	got := map[string]bool{}
	for _, k := range row.EffectiveKeys {
		got[k] = true
	}
	for _, want := range []string{"subagents_spawn", "subagents_list", "subagents_get", "subagents_cancel"} {
		if !got[want] {
			t.Fatalf("effective keys missing %s: %v", want, row.EffectiveKeys)
		}
	}
	// R17 边界：allow 不命名 Spirit 保留件时四件不得漏入有效面。
	for _, reserved := range SpiritReservedToolKeys() {
		if got[reserved] {
			t.Fatalf("spirit reserved %s must stay disabled for gov agent", reserved)
		}
	}
}

// profile 非注册值：LOW UNDEFINED_PROFILE（归一化兜底仍按 coding 计算工具面）。
func TestReconcileToolAssembly_undefinedProfile(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: "worker"}
	settings := map[string]AgentRuntimeSettings{
		"a1": {AgentID: "a1", ToolsEnabled: true, ToolsProfile: "dept_lead", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	codes := reconcileIssueCodes(report, "worker")
	if codes[ToolAssemblyCodeUndefinedProf] != ToolAssemblySeverityLow {
		t.Fatalf("want LOW UNDEFINED_PROFILE, got %v", codes)
	}
}

// read_only 下 memory_* deny 冗余：LOW REDUNDANT_DENY。
func TestReconcileToolAssembly_redundantDeny(t *testing.T) {
	ag := Agent{ID: "a1", AgentKey: "reader"}
	settings := map[string]AgentRuntimeSettings{
		"a1": {AgentID: "a1", ToolsEnabled: true, ToolsProfile: "read_only", ToolsAllowJSON: "[]", ToolsDenyJSON: `["memory_search","memory_add"]`},
	}
	u := newReconcileTestUsecase([]Agent{ag}, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	codes := reconcileIssueCodes(report, "reader")
	if codes[ToolAssemblyCodeRedundantDeny] != ToolAssemblySeverityLow {
		t.Fatalf("want LOW REDUNDANT_DENY, got %v", codes)
	}
}

// 目录层死工具（enabled=false 且非 registryOptInOnlyKeys 成员）：LOW DEAD_TOOL +
// DeadTools 汇总，AgentKey 为空。
func TestReconcileToolAssembly_deadTool(t *testing.T) {
	catalog := append(reconcileHealthyCatalog(),
		Tool{Key: "dead_legacy_tool", Category: "legacy", Source: "builtin", RiskLevel: "low", Enabled: false},
	)
	u := newReconcileTestUsecase(nil, catalog, map[string]AgentRuntimeSettings{})

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	codes := reconcileIssueCodes(report, "")
	if codes[ToolAssemblyCodeDeadTool] != ToolAssemblySeverityLow {
		t.Fatalf("want LOW DEAD_TOOL, got %v", codes)
	}
	if len(report.DeadTools) != 1 || report.DeadTools[0] != "dead_legacy_tool" {
		t.Fatalf("DeadTools=%v want [dead_legacy_tool]", report.DeadTools)
	}
	// opt-in 成员（shell_exec/mcp_broker，enabled=false）不得误报死工具。
	for _, d := range report.DeadTools {
		if d == "shell_exec" || d == "mcp_broker" {
			t.Fatalf("opt-in key %s must not be reported dead", d)
		}
	}
}

// 豁免：__voice_butler__（chat_only 刻意）与 eval_* 探针不触发计数门禁。
func TestReconcileToolAssembly_exemptions(t *testing.T) {
	agents := []Agent{
		{ID: "vb", AgentKey: VoiceButlerAgentKey},
		{ID: "ep", AgentKey: "eval_probe_x"},
	}
	settings := map[string]AgentRuntimeSettings{
		"vb": {AgentID: "vb", ToolsEnabled: true, ToolsProfile: "chat_only", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
		"ep": {AgentID: "ep", ToolsEnabled: true, ToolsProfile: "chat_only", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
	}
	u := newReconcileTestUsecase(agents, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, key := range []string{VoiceButlerAgentKey, "eval_probe_x"} {
		codes := reconcileIssueCodes(report, key)
		if _, ok := codes[ToolAssemblyCodeZeroTools]; ok {
			t.Fatalf("%s exempt from ZERO_TOOLS, got %v", key, codes)
		}
		if _, ok := codes[ToolAssemblyCodeFewTools]; ok {
			t.Fatalf("%s exempt from FEW_TOOLS, got %v", key, codes)
		}
	}
}

// issue 排序：HIGH > MID > LOW，同 severity 按 code 再按 agent_key。
func TestReconcileToolAssembly_issueSorting(t *testing.T) {
	agents := []Agent{
		{ID: "zero", AgentKey: "agent-zero"},
		{ID: "legacy", AgentKey: "agent-legacy"},
	}
	settings := map[string]AgentRuntimeSettings{
		"zero": {AgentID: "zero", ToolsEnabled: true, ToolsProfile: "chat_only", ToolsAllowJSON: "[]", ToolsDenyJSON: "[]"},
		// legacy 无 settings 行 → LOW NO_SETTINGS_ROW。
	}
	u := newReconcileTestUsecase(agents, reconcileHealthyCatalog(), settings)

	report, err := u.ReconcileToolAssembly(context.Background())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(report.Issues) < 2 {
		t.Fatalf("want >=2 issues, got %v", report.Issues)
	}
	first, second := report.Issues[0], report.Issues[1]
	if first.Severity != ToolAssemblySeverityHigh || first.Code != ToolAssemblyCodeZeroTools {
		t.Fatalf("first issue must be HIGH ZERO_TOOLS, got %+v", first)
	}
	if second.Severity != ToolAssemblySeverityLow || second.Code != ToolAssemblyCodeNoSettingsRow {
		t.Fatalf("second issue must be LOW NO_SETTINGS_ROW, got %+v", second)
	}
}
