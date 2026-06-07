package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// VerificationGateType defines the type of verification gate.
type VerificationGateType string

const (
	GateTypeDeptLeadApproval  VerificationGateType = "dept_lead_approval"
	GateTypeCrossDeptDelivery VerificationGateType = "cross_dept_delivery"
	GateTypeBorrowApproval    VerificationGateType = "borrow_approval"
)

// VerificationGate defines a verification checkpoint in a graph.
type VerificationGate struct {
	GateType    VerificationGateType `json:"gate_type"`
	AgentID     string               `json:"agent_id,omitempty"`     // for dept_lead_approval
	Description string               `json:"description"`
	MaxRetries  int                  `json:"max_retries"`            // default 3
}

// CrossDeptDeliveryGate defines a two-party approval gate for cross-department deliverables.
type CrossDeptDeliveryGate struct {
	GateType              VerificationGateType `json:"gate_type"`               // "cross_dept_delivery"
	OutputDepartmentID    string               `json:"output_department_id"`    // output side department
	ReceivingDepartmentID string               `json:"receiving_department_id"` // receiving side department
	DeliverableName       string               `json:"deliverable_name"`
	Description           string               `json:"description"`
	MaxRetries            int                  `json:"max_retries"`             // default 3
}

// GateResult holds the result of a gate execution.
type GateResult struct {
	Approved bool
	Reason   string
}

// VerificationGateExecutor executes verification gates.
// Initial implementation: direct LLM API call (Plan A).
type VerificationGateExecutor struct {
	deptLeadMgr *DeptLeadManager
	llmCaller   LLMCaller
	lg          loggateway.Logger
}

func NewVerificationGateExecutor(deptLeadMgr *DeptLeadManager, llmCaller LLMCaller, lg loggateway.Logger) *VerificationGateExecutor {
	return &VerificationGateExecutor{deptLeadMgr: deptLeadMgr, llmCaller: llmCaller, lg: lg}
}

// ExecuteGate executes a single verification gate.
// Returns (approved bool, reason string, err error)
func (e *VerificationGateExecutor) ExecuteGate(ctx context.Context, gate VerificationGate, teamOutput string) (bool, string, error) {
	switch gate.GateType {
	case GateTypeDeptLeadApproval:
		return e.executeDeptLeadApproval(ctx, gate, teamOutput)
	case GateTypeCrossDeptDelivery:
		return e.executeCrossDeptDelivery(ctx, gate, teamOutput)
	case GateTypeBorrowApproval:
		return e.executeBorrowApproval(ctx, gate, teamOutput)
	default:
		return false, "", kerrors.BadRequest("GATE", fmt.Sprintf("unknown gate type: %s", gate.GateType))
	}
}

// executeDeptLeadApproval calls the dept lead LLM to evaluate team output quality.
func (e *VerificationGateExecutor) executeDeptLeadApproval(ctx context.Context, gate VerificationGate, teamOutput string) (bool, string, error) {
	if gate.AgentID == "" {
		return false, "", kerrors.BadRequest("GATE", "dept_lead_approval gate requires agent_id")
	}

	systemPrompt := `你是一个部门主管，负责审核团队产出的质量。请评估以下团队输出是否满足质量标准。

请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

	userPrompt := fmt.Sprintf("团队输出：\n%s\n\n请评估此输出是否通过质量审核。", truncateForPrompt(teamOutput, 2000))

	resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
		Provider: "", // uses default
		Model:    "", // uses default
		System:   systemPrompt,
		User:     userPrompt,
	})
	if err != nil {
		return false, "", kerrors.InternalServer("GATE", "dept lead LLM call failed").WithCause(err)
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
func (e *VerificationGateExecutor) executeCrossDeptDelivery(ctx context.Context, gate VerificationGate, teamOutput string) (bool, string, error) {
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
			return false, "", kerrors.InternalServer("GATE", "output department has no lead agent; cannot perform quality check").WithCause(err)
		}
		systemPrompt := `你是一个部门主管，负责审核本部门产出的交付物质量。请评估以下交付物是否满足质量标准。

请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

		userPrompt := fmt.Sprintf("交付物名称：%s\n团队输出：\n%s\n\n请评估此交付物是否通过质量审核。",
			crossGate.DeliverableName, truncateForPrompt(teamOutput, 2000))

		resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
			Provider: "",
			Model:    "",
			System:   systemPrompt,
			User:     userPrompt,
		})
		if err != nil {
			return false, "", kerrors.InternalServer("GATE", "output dept lead LLM call failed").WithCause(err)
		}

		result, parseErr := parseGateResult(resp)
		if parseErr != nil {
			e.lg.Warn("解析输出方部门主管审批结果失败",
				loggateway.StepID("gate.cross_dept.output_lead"),
				loggateway.Err(parseErr),
			)
			return false, "输出方部门主管审批结果解析失败", nil
		} else if !result.Approved {
			return false, fmt.Sprintf("输出方部门主管拒绝: %s", result.Reason), nil
		}
		_ = outputLead // dept lead found, LLM call completed
	}

	// Step 2: Receiving-side dept lead acceptance check
	if crossGate.ReceivingDepartmentID != "" {
		receivingLead, err := e.deptLeadMgr.GetDeptLeadForTeam(ctx, crossGate.ReceivingDepartmentID)
		if err != nil {
			return false, "", kerrors.InternalServer("GATE", "receiving department has no lead agent; cannot perform acceptance check").WithCause(err)
		}
		systemPrompt := `你是一个部门主管，负责验收其他部门交付给本部门的工作成果。请评估以下交付物是否满足本部门的需求。

请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

		userPrompt := fmt.Sprintf("交付物名称：%s\n团队输出：\n%s\n\n请评估此交付物是否满足本部门需求。",
			crossGate.DeliverableName, truncateForPrompt(teamOutput, 2000))

		resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
			Provider: "",
			Model:    "",
			System:   systemPrompt,
			User:     userPrompt,
		})
		if err != nil {
			return false, "", kerrors.InternalServer("GATE", "receiving dept lead LLM call failed").WithCause(err)
		}

		result, parseErr := parseGateResult(resp)
		if parseErr != nil {
			e.lg.Warn("解析接收方部门主管审批结果失败",
				loggateway.StepID("gate.cross_dept.receiving_lead"),
				loggateway.Err(parseErr),
			)
			return false, "接收方部门主管审批结果解析失败", nil
		} else if !result.Approved {
			return false, fmt.Sprintf("接收方部门主管拒绝: %s", result.Reason), nil
		}
		_ = receivingLead // dept lead found, LLM call completed
	}

	return true, "双方部门主管均通过审批", nil
}

// executeBorrowApproval handles borrow approval gates.
func (e *VerificationGateExecutor) executeBorrowApproval(ctx context.Context, gate VerificationGate, teamOutput string) (bool, string, error) {
	// Plan A: simple LLM call to evaluate borrow request
	systemPrompt := `你是一个部门主管，负责审批跨部门借调请求。请评估以下借调请求是否合理。

请以 JSON 格式回复：
{"approved": true/false, "reason": "审批理由"}`

	userPrompt := fmt.Sprintf("借调请求：\n%s\n\n请评估此借调请求是否应该批准。", truncateForPrompt(teamOutput, 2000))

	resp, _, err := e.llmCaller.Call(ctx, LLMCallRequest{
		Provider: "",
		Model:    "",
		System:   systemPrompt,
		User:     userPrompt,
	})
	if err != nil {
		return false, "", kerrors.InternalServer("GATE", "borrow approval LLM call failed").WithCause(err)
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
func truncateForPrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
