package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// CompanyLeadAgentKeyPrefix is the prefix for company lead agent keys.
// Full key pattern: __company_lead_{company_key}__
const CompanyLeadAgentKeyPrefix = "__company_lead_"

// AgentVariantCompanyLead is the catalog variant for a company general manager.
const AgentVariantCompanyLead = "company_lead"

const metaKeyCompanyLeadAgentID = "company_lead_agent_id"

// companyLeadSubagentAllowJSON 是 GM 的分身执行兜底工具面（Q10 C+A 组合）。
// 显式列四件而非 group:subagent：审计可读、组定义漂移不影响存量授权。
// 存量库由 docker/q10-gm-subagent-allow.sql 同步。
const companyLeadSubagentAllowJSON = `["subagents_spawn","subagents_list","subagents_get","subagents_cancel"]`

// Company office + 总经理 position keys (company → department → position).
const (
	CompanyOfficeDeptSuffix   = "_office"
	CompanyLeadPositionSuffix = "_gm"
	CompanyOfficeDeptName     = "总经理办公室"
	CompanyLeadPositionName   = "总经理"
)

// CompanyOfficeDeptKey returns the system department key under a company.
func CompanyOfficeDeptKey(companyKey string) string {
	return strings.TrimSpace(companyKey) + CompanyOfficeDeptSuffix
}

// IsCompanyOfficeDept reports whether key is the GM office department of the
// given parent node. Exact match replaces the former broad _office suffix
// filter (2026-08-24 review: the suffix also silently matched business
// departments like regional_office, which then never got a dept_lead).
func IsCompanyOfficeDept(parent OrganizationNode, key string) bool {
	return parent.Level == "company" && strings.TrimSpace(key) == CompanyOfficeDeptKey(parent.Key)
}

// CompanyLeadPositionKey returns the 总经理 position key under that office.
func CompanyLeadPositionKey(companyKey string) string {
	return strings.TrimSpace(companyKey) + CompanyLeadPositionSuffix
}

// HydrateCompanyLeadFromMetadata copies company_lead_agent_id from MetadataJSON.
func HydrateCompanyLeadFromMetadata(n *OrganizationNode) {
	if n == nil || strings.TrimSpace(n.CompanyLeadAgentID) != "" {
		return
	}
	n.CompanyLeadAgentID = companyLeadIDFromMetadata(n.MetadataJSON)
}

// ApplyCompanyLeadToMetadata writes CompanyLeadAgentID into MetadataJSON.
func ApplyCompanyLeadToMetadata(n *OrganizationNode) {
	if n == nil {
		return
	}
	n.MetadataJSON = mergeMetadataString(n.MetadataJSON, metaKeyCompanyLeadAgentID, n.CompanyLeadAgentID)
}

func companyLeadIDFromMetadata(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	v, _ := m[metaKeyCompanyLeadAgentID].(string)
	return strings.TrimSpace(v)
}

func mergeMetadataString(raw, key, value string) string {
	var m map[string]any
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			m = map[string]any{}
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	if strings.TrimSpace(value) == "" {
		delete(m, key)
	} else {
		m[key] = value
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return string(b)
}

// CreateCompanyLead creates a company lead Agent for the given company node.
// Called automatically when a company node is created. Idempotent.
func (m *DeptLeadManager) CreateCompanyLead(ctx context.Context, companyNode OrganizationNode) (*Agent, error) {
	if companyNode.Level != "company" {
		return nil, apierror.BadRequest("COMPANY_LEAD", "can only create lead for company level nodes")
	}

	agentKey := fmt.Sprintf("%s%s__", CompanyLeadAgentKeyPrefix, companyNode.Key)

	pos, posErr := m.ensureCompanyLeadPosition(ctx, companyNode)
	if posErr != nil {
		return nil, posErr
	}

	existing, err := m.agentRepo.GetAgentByAgentKey(ctx, agentKey)
	if err == nil && existing.ID != "" {
		HydrateCompanyLeadFromMetadata(&companyNode)
		if companyNode.CompanyLeadAgentID != existing.ID {
			companyNode.CompanyLeadAgentID = existing.ID
			ApplyCompanyLeadToMetadata(&companyNode)
			if _, updateErr := m.orgRepo.UpdateOrgNode(ctx, companyNode); updateErr != nil {
				m.lg.Warn("failed to link existing company lead to org node",
					loggateway.StepID("company_lead.create"),
					loggateway.Str("company_id", companyNode.ID),
					loggateway.Err(updateErr))
			}
		}
		if existing.PositionID != pos.ID && m.agentUC != nil {
			_, _ = m.agentUC.Update(ctx, existing.ID, Agent{PositionID: pos.ID, PositionKey: pos.Key})
		}
		if m.agentUC != nil {
			if a, getErr := m.agentUC.Get(ctx, existing.ID); getErr == nil && a.ID != "" {
				return &a, nil
			}
		}
		return &existing, nil
	}

	agent := Agent{
		AgentKey:     agentKey,
		DisplayName:  fmt.Sprintf("总经理-%s", companyNode.Name),
		Kind:         "system_builtin",
		Source:       "system",
		PositionID:   pos.ID,
		PositionKey:  pos.Key,
		AgentVariant: AgentVariantCompanyLead,
		Status:       "active",
		Readonly:     true,
	}
	if agent.ID == "" {
		agent.ID = newAgentCatalogID()
	}

	prompt := m.buildCompanyLeadPrompt(companyNode)
	agent.Files = []AgentPromptFile{
		{ID: newAgentCatalogID(), AgentID: agent.ID, Name: "system.md", Body: prompt, SortOrder: 1},
	}

	settings := withSettingDefaults(AgentRuntimeSettings{AgentID: agent.ID})
	settings.MemoryEnabled = true
	settings.ToolsEnabled = true
	settings.ToolsProfile = "read_only"
	// Q10（session-eval-20260827，用户裁定 C+A 组合）：read_only profile 经
	// allow JSON 授予 subagents 四件作分身执行兜底——治理钳制只改 profile
	// 不动 allow（ClampSpecialistToolFace），profile_eff 仍 read_only 不触发
	// GOV_NOT_READONLY；subagents_* 非 R17 Spirit 保留件。分身=GM 自身克隆
	// （spawn 无目标 agent 参数），prompt 同步禁止虚构「已派发部门」。
	settings.ToolsAllowJSON = companyLeadSubagentAllowJSON
	// 记忆栈对齐 dept_lead（DB 列默认全 true）：Ent Create 会显式写每个字段，
	// bool 零值 false 会覆盖列默认，导致 L1-L4/intent/clarify 全关（2026-08-24
	// 实锤：3 个 GM 记忆栈全关而 32 个 dept_lead 全开）。
	// subagents 显式 true：对齐 DefaultAgentRuntimeSettings 与 seed 字面量
	// （2026-08-24 三方正齐：默认 true / seed config_json true / DB 实值 t）。
	// R10 只禁启发式把 GM 分配为业务 Team Lead，不禁 GM 自身编排能力。
	// heartbeat 保持 false——总经理无自治职责（设计 R10）。
	settings.SubagentsEnabled = true
	settings.L0InjectL1 = true
	settings.L0InjectL3 = true
	settings.L0InjectL4 = true
	settings.L0SnapshotEnabled = true
	settings.L1Enabled = true
	settings.L2EpisodeEnabled = true
	settings.L2IndexEnabled = true
	settings.L2RecallEnabled = true
	settings.L3Enabled = true
	settings.L3InjectProvenance = true
	settings.L4Enabled = true
	settings.L4GraphInjectNeighbors = true
	settings.L4IdentityInject = true
	settings.IntentPassEnabled = true
	settings.ClarificationEnabled = true
	settings.IntentSkipEnabled = false

	configJSON, cfgErr := configJSONFromSettings(settings, agent.Files)
	if cfgErr != nil {
		return nil, apierror.Internal("COMPANY_LEAD", "failed to build company lead config: %s", cfgErr)
	}
	agent.ConfigJSON = EmbedAgentKindInConfigJSON(configJSON, AgentKindLLM, nil, m.lg)

	created, err := m.agentUC.CreateWithFilesAndSettings(ctx, agent, agent.Files, &settings)
	if err != nil {
		return nil, err
	}

	companyNode.CompanyLeadAgentID = created.ID
	ApplyCompanyLeadToMetadata(&companyNode)
	if _, err = m.orgRepo.UpdateOrgNode(ctx, companyNode); err != nil {
		return nil, err
	}
	return &created, nil
}

// companyLeadSlotRepo is the narrow org surface used to hang 总经理 on a real position
// without OrganizationUsecase.Create (which would spawn a dept_lead on the office).
type companyLeadSlotRepo interface {
	GetOrgNodeByKey(ctx context.Context, key string) (OrganizationNode, error)
	CreateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
}

// ensureCompanyLeadPosition creates 总经理办公室 / 总经理 under the company
// if missing (via repo, so the office department does not spawn an extra dept_lead).
func (m *DeptLeadManager) ensureCompanyLeadPosition(ctx context.Context, company OrganizationNode) (OrganizationNode, error) {
	return ensureCompanyLeadPositionOn(ctx, m.orgRepo, company)
}

func ensureCompanyLeadPositionOn(ctx context.Context, repo companyLeadSlotRepo, company OrganizationNode) (OrganizationNode, error) {
	officeKey := CompanyOfficeDeptKey(company.Key)
	posKey := CompanyLeadPositionKey(company.Key)

	office, err := repo.GetOrgNodeByKey(ctx, officeKey)
	if err != nil || office.ID == "" {
		office, err = repo.CreateOrgNode(ctx, OrganizationNode{
			Key:         officeKey,
			Name:        CompanyOfficeDeptName,
			Level:       "department",
			ParentID:    company.ID,
			WorkspaceID: company.WorkspaceID,
			IsSystem:    true,
			Status:      "active",
			Enabled:     true,
		})
		if err != nil {
			return OrganizationNode{}, err
		}
	}

	pos, err := repo.GetOrgNodeByKey(ctx, posKey)
	if err != nil || pos.ID == "" {
		pos, err = repo.CreateOrgNode(ctx, OrganizationNode{
			Key:         posKey,
			Name:        CompanyLeadPositionName,
			Level:       "position",
			ParentID:    office.ID,
			WorkspaceID: company.WorkspaceID,
			IsSystem:    true,
			Status:      "active",
			Enabled:     true,
		})
		if err != nil {
			return OrganizationNode{}, err
		}
	}
	return pos, nil
}

// DeleteCompanyLead deletes the company lead Agent linked on the company node.
func (m *DeptLeadManager) DeleteCompanyLead(ctx context.Context, companyID string) error {
	node, err := m.orgRepo.GetOrgNode(ctx, companyID)
	if err != nil {
		return err
	}
	HydrateCompanyLeadFromMetadata(&node)
	if node.CompanyLeadAgentID == "" {
		return nil
	}
	return m.agentUC.ForceDelete(ctx, node.CompanyLeadAgentID)
}

type companyLeadPromptData struct {
	CompanyName        string
	CompanyDescription string
}

var companyLeadPromptTmpl *template.Template

func (m *DeptLeadManager) loadCompanyLeadPromptTmpl() {
	if companyLeadPromptTmpl != nil {
		return
	}
	tmplPath := filepath.Join(ScenarioDir(), "system", "prompts", "company_lead.md")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		companyLeadPromptTmpl = template.Must(template.New("company_lead_fallback").Parse(`# 公司总经理

你是「{{.CompanyName}}」的总经理。

## 职责

1. **流程剧本**：对本公司标准流程一次授权，不要对用户原话做全局拆解
2. **对外接口**：跨公司只传公司级 Brief（范围/接口/期限/机密级）
3. **仲裁**：部门争议由你裁定；公司争议呈精灵/用户，禁止与对方总经理循环互怼

## 能力边界

- 你有 subagents_spawn 分身工具：重型或可并行的起草/调研任务，可派自己的分身后台执行，subagents_get 取回结果
- 分身是你自己的克隆，**不是部门主管**——禁止虚构「已派发到部门 / 已交办员工」
- 需要部门真正执行时：产出公司级 Brief，并明确告知用户路由建议（切换到对应部门主管会话，或请精灵组队接手）

你不是业务 Team Lead，不点员工、不继承精灵编排工具。

## 公司信息

- 公司名称：{{.CompanyName}}
- 公司描述：{{.CompanyDescription}}
`))
		return
	}
	companyLeadPromptTmpl = template.Must(template.New("company_lead").Parse(string(tmplBytes)))
}

func (m *DeptLeadManager) buildCompanyLeadPrompt(company OrganizationNode) string {
	m.loadCompanyLeadPromptTmpl()
	var b strings.Builder
	if err := companyLeadPromptTmpl.Execute(&b, companyLeadPromptData{
		CompanyName:        company.Name,
		CompanyDescription: company.Description,
	}); err != nil {
		m.lg.Warn("company lead prompt render failed",
			loggateway.StepID("company_lead.prompt"),
			loggateway.Err(err))
		return "# 公司总经理\n"
	}
	return b.String()
}
