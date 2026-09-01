package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// VerificationGateType defines the type of verification gate.
type VerificationGateType string

const (
	GateTypeDeptLeadApproval  VerificationGateType = "dept_lead_approval"
	GateTypeCrossDeptDelivery VerificationGateType = "cross_dept_delivery"
	GateTypeBorrowApproval    VerificationGateType = "borrow_approval"
	// GateTypeToolAssertion is a deterministic fact-check gate (F9, Phase 11):
	// invoke a whitelisted tool and assert a JSON path equals an expected value.
	// Unlike the LLM quality gates above, this cannot be deceived by prose.
	GateTypeToolAssertion VerificationGateType = "tool_assertion"
)

// VerificationGate defines a verification checkpoint in a graph.
type VerificationGate struct {
	GateType    VerificationGateType `json:"gate_type"`
	AgentID     string               `json:"agent_id,omitempty"` // for dept_lead_approval
	Description string               `json:"description"`
	MaxRetries  int                  `json:"max_retries"` // default 3
	// tool_assertion fields (F9, Phase 11).
	Tool          string `json:"tool,omitempty"`           // whitelisted tool name
	ArgumentsJSON string `json:"arguments_json,omitempty"` // tool arguments as JSON object
	AssertPath    string `json:"assert_path,omitempty"`    // dot-separated JSON path, e.g. "skill_key"
	AssertEquals  string `json:"assert_equals,omitempty"`  // expected scalar value (canonical string)
}

// ToolAssertionInvoker invokes a whitelisted tool for deterministic
// verification gates and returns the raw JSON result. Implementations live in
// the same package (skillAssertionInvoker) — tools are NOT executed through
// the LLM tool loop, so the assertion is fully deterministic.
type ToolAssertionInvoker interface {
	InvokeForAssertion(ctx context.Context, toolName string, argumentsJSON string) (json.RawMessage, error)
}

// VerificationGateExecutorOption configures optional executor dependencies.
type VerificationGateExecutorOption func(*VerificationGateExecutor)

// WithToolAssertionInvoker wires the deterministic tool invoker used by
// tool_assertion gates. Nil invoker = tool_assertion gates fail closed.
func WithToolAssertionInvoker(inv ToolAssertionInvoker) VerificationGateExecutorOption {
	return func(e *VerificationGateExecutor) { e.toolInvoker = inv }
}

// WithDeptMailbox wires the dept mailbox for cross-dept delivery rejection
// notifications. Nil = skip notification (backward compatible).
func WithDeptMailbox(mb *DeptMailboxUsecase) VerificationGateExecutorOption {
	return func(e *VerificationGateExecutor) { e.deptMailbox = mb }
}

// CrossDeptDeliveryGate defines a two-party approval gate for cross-department deliverables.
type CrossDeptDeliveryGate struct {
	GateType              VerificationGateType `json:"gate_type"`               // "cross_dept_delivery"
	OutputDepartmentID    string               `json:"output_department_id"`    // output side department
	ReceivingDepartmentID string               `json:"receiving_department_id"` // receiving side department
	DeliverableName       string               `json:"deliverable_name"`
	Description           string               `json:"description"`
	MaxRetries            int                  `json:"max_retries"` // default 3
}

// GateResult holds the result of a gate execution.
type GateResult struct {
	Approved bool
	Reason   string
}

const defaultTruncateChars = 2000

// VerificationGateExecutor executes verification gates.
// Initial implementation: direct LLM API call (Plan A).
type VerificationGateExecutor struct {
	deptLeadMgr *DeptLeadManager
	llmCaller   LLMCaller
	lg          loggateway.Logger
	toolInvoker ToolAssertionInvoker
	receipts    *verificationReceiptLedger // ADR-79-V V3 证据回执台账
	deptMailbox *DeptMailboxUsecase       // M71: 拒绝时自动通知对方主管
}

func NewVerificationGateExecutor(deptLeadMgr *DeptLeadManager, llmCaller LLMCaller, lg loggateway.Logger, opts ...VerificationGateExecutorOption) *VerificationGateExecutor {
	e := &VerificationGateExecutor{deptLeadMgr: deptLeadMgr, llmCaller: llmCaller, lg: lg, receipts: newVerificationReceiptLedger()}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExecuteGate executes a single verification gate.
// truncateChars is the max rune count for truncating team output in prompts.
// When <= 0, defaultTruncateChars (2000) is used.
// Returns (approved bool, reason string, err error)
func (e *VerificationGateExecutor) ExecuteGate(ctx context.Context, gate VerificationGate, teamOutput string, truncateChars int) (bool, string, error) {
	if truncateChars <= 0 {
		truncateChars = defaultTruncateChars
	}
	switch gate.GateType {
	case GateTypeDeptLeadApproval:
		return e.executeDeptLeadApproval(ctx, gate, teamOutput, truncateChars)
	case GateTypeCrossDeptDelivery:
		return e.executeCrossDeptDelivery(ctx, gate, teamOutput, truncateChars)
	case GateTypeBorrowApproval:
		return e.executeBorrowApproval(ctx, gate, teamOutput, truncateChars)
	case GateTypeToolAssertion:
		return e.executeToolAssertion(ctx, gate)
	default:
		return false, "", apierror.BadRequest("GATE", "unknown gate type: %s", gate.GateType)
	}
}

// toolAssertionWhitelist lists the tools a tool_assertion gate may invoke
// (F9, Phase 11). First version: cli_admin_skill_get only — install
// verification is the only deterministic fact-check use case so far.
var toolAssertionWhitelist = map[string]bool{
	"cli_admin_skill_get": true,
}

// executeToolAssertion invokes a whitelisted tool and asserts a JSON path in
// its result equals the expected value. Deterministic: no LLM involved.
// Invocation failure or assertion mismatch → approved=false (not an error);
// misconfiguration (unknown tool / nil invoker) → error.
func (e *VerificationGateExecutor) executeToolAssertion(ctx context.Context, gate VerificationGate) (bool, string, error) {
	tool := strings.TrimSpace(gate.Tool)
	if !toolAssertionWhitelist[tool] {
		return false, "", apierror.BadRequest("GATE", "tool_assertion gate: tool %q is not whitelisted", tool)
	}
	if e.toolInvoker == nil {
		return false, "", apierror.Internal("GATE", "tool_assertion gate: no tool invoker configured")
	}
	raw, err := e.toolInvoker.InvokeForAssertion(ctx, tool, gate.ArgumentsJSON)
	if err != nil {
		return false, fmt.Sprintf("工具 %s 调用失败: %v", tool, err), nil
	}
	var doc any
	if uerr := json.Unmarshal(raw, &doc); uerr != nil {
		return false, fmt.Sprintf("工具 %s 返回非 JSON 结果: %v", tool, uerr), nil
	}
	val, ok := jsonPathLookup(doc, gate.AssertPath)
	if !ok {
		return false, fmt.Sprintf("断言路径 %q 在工具返回中不存在", gate.AssertPath), nil
	}
	got := canonicalJSONScalar(val)
	if got != strings.TrimSpace(gate.AssertEquals) {
		return false, fmt.Sprintf("断言失败: %s = %q，期望 %q", gate.AssertPath, got, gate.AssertEquals), nil
	}
	return true, fmt.Sprintf("断言通过: %s = %q", gate.AssertPath, got), nil
}

// jsonPathLookup walks a decoded JSON document by a dot-separated path of
// object keys (e.g. "skill_key" or "data.skill.enabled"). Returns false when
// any segment is missing or a non-object is encountered mid-path.
func jsonPathLookup(doc any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// canonicalJSONScalar renders a decoded JSON scalar as its canonical string
// for equality comparison: strings as-is, bools as true/false, numbers in
// shortest form, nil as "null".
func canonicalJSONScalar(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ---------------------------------------------------------------------------
// F9 (Phase 11): skill assertion invoker — deterministic cli_admin_skill_get
// ---------------------------------------------------------------------------

// skillAssertionView is the JSON shape returned by the skill assertion
// invoker. Mirrors cli_admin.SkillItem (biz cannot import internal/tools),
// plus the enabled flag the gate asserts on.
type skillAssertionView struct {
	ID          string `json:"id"`
	SkillKey    string `json:"skill_key"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`
}

// skillAssertionInvoker implements ToolAssertionInvoker for the whitelisted
// cli_admin_skill_get tool, backed directly by CLIAdminSkillLister.
type skillAssertionInvoker struct {
	skills CLIAdminSkillLister
}

// NewSkillAssertionInvoker creates the invoker wired to the skill usecase.
func NewSkillAssertionInvoker(skills CLIAdminSkillLister) ToolAssertionInvoker {
	return &skillAssertionInvoker{skills: skills}
}

// InvokeForAssertion resolves cli_admin_skill_get arguments and returns the
// skill as JSON. Arguments: {"id": "<skill id>"} or {"skill_key": "<slug>"}
// (id takes precedence). Unknown tools are rejected — the whitelist is
// enforced again here so the invoker stays safe when reused elsewhere.
func (i *skillAssertionInvoker) InvokeForAssertion(ctx context.Context, toolName string, argumentsJSON string) (json.RawMessage, error) {
	if i == nil || i.skills == nil {
		return nil, apierror.Internal("GATE", "skill assertion invoker: no skill lister configured")
	}
	if strings.TrimSpace(toolName) != "cli_admin_skill_get" {
		return nil, apierror.BadRequest("GATE", "skill assertion invoker: tool %q is not supported", toolName)
	}
	var args struct {
		ID       string `json:"id"`
		SkillKey string `json:"skill_key"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return nil, apierror.BadRequest("GATE", "skill assertion invoker: invalid arguments_json").WithCause(err)
	}
	var (
		s   Skill
		err error
	)
	switch {
	case strings.TrimSpace(args.ID) != "":
		s, err = i.skills.Get(ctx, strings.TrimSpace(args.ID))
	case strings.TrimSpace(args.SkillKey) != "":
		s, err = i.skills.GetBySlug(ctx, strings.TrimSpace(args.SkillKey))
	default:
		return nil, apierror.BadRequest("GATE", "skill assertion invoker: id or skill_key is required")
	}
	if err != nil {
		return nil, err
	}
	version := ""
	if s.CurrentVersion != nil {
		version = s.CurrentVersion.Version
	}
	return json.Marshal(skillAssertionView{
		ID:          s.ID,
		SkillKey:    s.Slug,
		DisplayName: s.Name,
		Status:      s.Status,
		Version:     version,
		Enabled:     s.Enabled,
	})
}

// ---------------------------------------------------------------------------
// F9 (Phase 11): gate auto-generation for skill-install teams
// ---------------------------------------------------------------------------

// installSkillPhraseRe captures the skill name from the intent phrase the
// decomposition prompt prescribes (F8 layer 2): "安装 xlsx skill" /
// "install the xlsx skill" / "安装 xlsx 技能".
var installSkillPhraseRe = regexp.MustCompile(`(?i)(?:安装|install)\s+(?:the\s+)?([a-z0-9][a-z0-9._-]*)\s*(?:skill|技能)`)

// installURLRe finds the source URL in the task description for the fallback
// key extraction (last path segment).
var installURLRe = regexp.MustCompile(`https?://[^\s，。、）)"']+`)

// skillInstallAssertionGate builds the deterministic tool_assertion gate for a
// single-member system-admin team whose task is a skill install (F9, Phase
// 11). The gate asserts the installed skill is enabled — runtime visibility
// requires enabled && published, and fresh installs default enabled=true, so
// enabled=false or a missing skill both mean the install did not complete.
//
// Returns nil when the team is not a solo system-admin team, the task is not
// a skill-install intent, or no skill key can be extracted from the intent
// (fail-open at generation time; a wrong key fails closed at gate time with a
// clear reason, which is the honest outcome for an unverifiable claim).
func skillInstallAssertionGate(agentKeys []string, taskDescription string) *VerificationGate {
	if len(agentKeys) != 1 || strings.TrimSpace(agentKeys[0]) != SystemAdminAgentKey {
		return nil
	}
	if !strings.Contains(taskDescription, "cli_admin_skill_install_from_url") {
		return nil
	}
	key := extractSkillKeyFromIntent(taskDescription)
	if key == "" {
		return nil
	}
	args, err := json.Marshal(map[string]string{"skill_key": key})
	if err != nil {
		return nil
	}
	return &VerificationGate{
		GateType:      GateTypeToolAssertion,
		Description:   fmt.Sprintf("验证 skill %q 已安装且启用", key),
		Tool:          "cli_admin_skill_get",
		ArgumentsJSON: string(args),
		AssertPath:    "enabled",
		AssertEquals:  "true",
	}
}

// extractSkillKeyFromIntent derives the expected skill slug from the task
// intent text. Preference order: the explicit name in the 安装/install phrase
// (matches the SKILL.md frontmatter name for well-named skills), then the
// source URL's last path segment. The result is normalized with the same
// slug rules the importer uses (NormalizeSkillSlug).
func extractSkillKeyFromIntent(taskDescription string) string {
	if m := installSkillPhraseRe.FindStringSubmatch(taskDescription); m != nil {
		if key := NormalizeSkillSlug(m[1]); key != "" {
			return key
		}
	}
	if u := installURLRe.FindString(taskDescription); u != "" {
		seg := strings.TrimRight(u, "/")
		if idx := strings.LastIndex(seg, "/"); idx >= 0 {
			seg = seg[idx+1:]
		}
		seg = strings.TrimSuffix(seg, ".git")
		return NormalizeSkillSlug(seg)
	}
	return ""
}

// gateApprovalSuffix is appended to the dept lead's system prompt for LLM gate calls.
const gateApprovalSuffix = `

## 当前任务

请评估以下团队输出是否满足质量标准。请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

// gateDeliveryQualitySuffix is appended for output-side quality check.
const gateDeliveryQualitySuffix = `

## 当前任务

请评估以下交付物是否满足质量标准。请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

// gateDeliveryAcceptanceSuffix is appended for receiving-side acceptance check.
const gateDeliveryAcceptanceSuffix = `

## 当前任务

请评估以下交付物是否满足本部门需求。请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

// executeDeptLeadApproval calls the dept lead LLM to evaluate team output quality.
func (e *VerificationGateExecutor) executeDeptLeadApproval(ctx context.Context, gate VerificationGate, teamOutput string, truncateChars int) (bool, string, error) {
	if gate.AgentID == "" {
		return false, "", apierror.BadRequest("GATE", "dept_lead_approval gate requires agent_id")
	}

	// Fetch dept lead agent to use its Provider/Model/System prompt
	lead, leadErr := e.deptLeadMgr.GetDeptLeadAgent(ctx, gate.AgentID)
	systemPrompt := `你是一个部门主管，负责审核团队产出的质量。`
	provider := ""
	model := ""
	if leadErr == nil && lead != nil {
		if sp := extractSystemPrompt(lead); sp != "" {
			systemPrompt = sp
		}
		provider = lead.Provider
		model = lead.Model
	} else {
		e.lg.Warn("failed to fetch dept lead agent, using default config",
			loggateway.StepID("gate.dept_lead_approval"),
			loggateway.Str("agent_id", gate.AgentID),
			loggateway.Err(leadErr),
		)
	}
	systemPrompt += gateApprovalSuffix

	userPrompt := fmt.Sprintf("团队输出：\n%s\n\n请评估此输出是否通过质量审核。", truncateForPrompt(teamOutput, truncateChars))

	resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   systemPrompt,
		User:     userPrompt,
	})
	if err != nil {
		return false, "", func() *apierror.Error {
			e := apierror.Internal("GATE", "dept lead LLM call failed")
			e.Cause = err
			return e
		}()
	}

	result, parseErr := parseGateResult(resp)
	if parseErr != nil {
		e.lg.Warn("解析部门主管审批结果失败，默认拒绝",
			loggateway.StepID("gate.dept_lead_approval"),
			loggateway.Err(parseErr),
		)
		return false, "审批结果解析失败，默认拒绝", nil
	}

	return result.Approved, result.Reason, nil
}

// executeCrossDeptDelivery performs two-party approval:
// 1. Output-side dept lead approval (quality check)
// 2. Receiving-side dept lead approval (acceptance check)
// Both must pass for the gate to pass.
func (e *VerificationGateExecutor) executeCrossDeptDelivery(ctx context.Context, gate VerificationGate, teamOutput string, truncateChars int) (bool, string, error) {
	// Parse the CrossDeptDeliveryGate from the gate's Description field
	// (since VerificationGate is the uniform type, extra fields are in Description as JSON)
	var crossGate CrossDeptDeliveryGate
	if err := json.Unmarshal([]byte(gate.Description), &crossGate); err != nil {
		// Fallback: try to use the gate directly
		crossGate = CrossDeptDeliveryGate{
			GateType:    GateTypeCrossDeptDelivery,
			Description: gate.Description,
			MaxRetries:  gate.MaxRetries,
		}
	}

	// Step 1: Output-side dept lead quality check
	if crossGate.OutputDepartmentID != "" {
		outputLead, err := e.deptLeadMgr.GetDeptLeadForTeam(ctx, crossGate.OutputDepartmentID)
		if err != nil {
			return false, "", func() *apierror.Error {
				e := apierror.Internal("GATE", "output department has no lead agent; cannot perform quality check")
				e.Cause = err
				return e
			}()
		}
		systemPrompt := `你是一个部门主管，负责审核本部门产出的交付物质量。`
		provider := ""
		model := ""
		if sp := extractSystemPrompt(outputLead); sp != "" {
			systemPrompt = sp
		}
		provider = outputLead.Provider
		model = outputLead.Model
		systemPrompt += gateDeliveryQualitySuffix

		userPrompt := fmt.Sprintf("交付物名称：%s\n团队输出：\n%s\n\n请评估此交付物是否通过质量审核。",
			crossGate.DeliverableName, truncateForPrompt(teamOutput, truncateChars))

		resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
			Provider: provider,
			Model:    model,
			System:   systemPrompt,
			User:     userPrompt,
		})
		if err != nil {
			return false, "", func() *apierror.Error {
				e := apierror.Internal("GATE", "output dept lead LLM call failed")
				e.Cause = err
				return e
			}()
		}

		result, parseErr := parseGateResult(resp)
		if parseErr != nil {
			e.lg.Warn("解析输出方部门主管审批结果失败",
				loggateway.StepID("gate.cross_dept.output_lead"),
				loggateway.Err(parseErr),
			)
			return false, "输出方部门主管审批结果解析失败", nil
		} else if !result.Approved {
			e.notifyDeliveryRejected(ctx, outputLead, crossGate.OutputDepartmentID, crossGate.ReceivingDepartmentID,
				crossGate.DeliverableName, result.Reason)
			return false, fmt.Sprintf("输出方部门主管拒绝: %s", result.Reason), nil
		}
	}

	// Step 2: Receiving-side dept lead acceptance check
	if crossGate.ReceivingDepartmentID != "" {
		receivingLead, err := e.deptLeadMgr.GetDeptLeadForTeam(ctx, crossGate.ReceivingDepartmentID)
		if err != nil {
			return false, "", func() *apierror.Error {
				e := apierror.Internal("GATE", "receiving department has no lead agent; cannot perform acceptance check")
				e.Cause = err
				return e
			}()
		}
		systemPrompt := `你是一个部门主管，负责验收其他部门交付给本部门的工作成果。`
		provider := ""
		model := ""
		if sp := extractSystemPrompt(receivingLead); sp != "" {
			systemPrompt = sp
		}
		provider = receivingLead.Provider
		model = receivingLead.Model
		systemPrompt += gateDeliveryAcceptanceSuffix

		userPrompt := fmt.Sprintf("交付物名称：%s\n团队输出：\n%s\n\n请评估此交付物是否满足本部门需求。",
			crossGate.DeliverableName, truncateForPrompt(teamOutput, truncateChars))

		resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
			Provider: provider,
			Model:    model,
			System:   systemPrompt,
			User:     userPrompt,
		})
		if err != nil {
			return false, "", func() *apierror.Error {
				e := apierror.Internal("GATE", "receiving dept lead LLM call failed")
				e.Cause = err
				return e
			}()
		}

		result, parseErr := parseGateResult(resp)
		if parseErr != nil {
			e.lg.Warn("解析接收方部门主管审批结果失败",
				loggateway.StepID("gate.cross_dept.receiving_lead"),
				loggateway.Err(parseErr),
			)
			return false, "接收方部门主管审批结果解析失败", nil
		} else if !result.Approved {
			e.notifyDeliveryRejected(ctx, receivingLead, crossGate.ReceivingDepartmentID, crossGate.OutputDepartmentID,
				crossGate.DeliverableName, result.Reason)
			return false, fmt.Sprintf("接收方部门主管拒绝: %s", result.Reason), nil
		}
	}

	return true, "双方部门主管均通过审批", nil
}

// notifyDeliveryRejected sends a deptmail notification to the other
// department's lead when a cross-dept delivery is rejected. Best-effort:
// notification failure never affects the gate result (already returned).
func (e *VerificationGateExecutor) notifyDeliveryRejected(ctx context.Context, rejectingLead *Agent, rejectingDeptID, otherDeptID, deliverableName, reason string) {
	if e.deptMailbox == nil || rejectingLead == nil {
		return
	}
	subject := fmt.Sprintf("交付物审批未通过: %s", deliverableName)
	body := fmt.Sprintf("交付物「%s」未通过审批。\n\n审批方：%s\n拒绝原因：%s",
		deliverableName, rejectingDeptID, reason)
	if _, err := e.deptMailbox.SendMessage(ctx, rejectingLead.ID, otherDeptID, subject, body, "[]"); err != nil {
		e.lg.Warn("跨部门交付拒绝通知发送失败",
			loggateway.StepID("gate.cross_dept.notify"),
			loggateway.Str("deliverable", deliverableName),
			loggateway.Err(err),
		)
	}
}

// executeBorrowApproval handles borrow approval gates.
func (e *VerificationGateExecutor) executeBorrowApproval(ctx context.Context, gate VerificationGate, teamOutput string, truncateChars int) (bool, string, error) {
	// Plan A: simple LLM call to evaluate borrow request
	systemPrompt := `你是一个部门主管，负责审批跨部门借调请求。请评估以下借调请求是否合理。

请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

	userPrompt := fmt.Sprintf("借调请求：\n%s\n\n请评估此借调请求是否应该批准。", truncateForPrompt(teamOutput, truncateChars))

	resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
		Provider: "",
		Model:    "",
		System:   systemPrompt,
		User:     userPrompt,
	})
	if err != nil {
		return false, "", func() *apierror.Error {
			e := apierror.Internal("GATE", "borrow approval LLM call failed")
			e.Cause = err
			return e
		}()
	}

	result, parseErr := parseGateResult(resp)
	if parseErr != nil {
		e.lg.Warn("解析借调审批结果失败，默认拒绝",
			loggateway.StepID("gate.borrow_approval"),
			loggateway.Err(parseErr),
		)
		return false, "审批结果解析失败，默认拒绝", nil
	}

	return result.Approved, result.Reason, nil
}

// parseGateResult parses the LLM response into a GateResult.
func parseGateResult(resp string) (GateResult, error) {
	// Try to extract JSON from the response (may contain markdown fences)
	cleaned := resp
	if idx := strings.Index(resp, "{"); idx >= 0 {
		cleaned = resp[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	var result GateResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return GateResult{}, err
	}
	return result, nil
}

// ParseVerificationGates parses a JSON string into a slice of VerificationGate.
func ParseVerificationGates(jsonStr string) ([]VerificationGate, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var gates []VerificationGate
	if err := json.Unmarshal([]byte(jsonStr), &gates); err != nil {
		return nil, err
	}
	return gates, nil
}

// VerificationGatesToJSON serializes a slice of VerificationGate to JSON.
func VerificationGatesToJSON(gates []VerificationGate) string {
	if len(gates) == 0 {
		return "[]"
	}
	b, err := json.Marshal(gates)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// truncateForPrompt truncates content for inclusion in LLM prompts.
func truncateForPrompt(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}
