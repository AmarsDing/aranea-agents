package biz

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeliverableContract defines the input/output contract between teams.
type DeliverableContract struct {
	Name        string `json:"name"`   // e.g., "design_spec"
	Type        string `json:"type"`   // e.g., "document", "code", "data"
	Format      string `json:"format"` // e.g., "markdown", "json", "zip"
	Description string `json:"description"`
	// SchemaJSON is an optional JSON Schema (C2) for content-level validation
	// of the deliverable. Only enforced when Format == "json" and the upstream
	// content is itself valid JSON; otherwise skipped (advisory).
	SchemaJSON string `json:"schema_json,omitempty"`
}

// Contract mismatch kinds returned by ValidateContractMatchDetailed.
const (
	ContractMismatchMissing = "missing"         // downstream entry has no matching upstream deliverable
	ContractMismatchType    = "type_mismatch"   // same name, different type
	ContractMismatchFormat  = "format_mismatch" // same name, different format
	// ContractMismatchSchema is produced by content-level validation (C2) in
	// ReadUpstreamDeliverable: the upstream deliverable's JSON content does
	// not satisfy the downstream entry's schema_json.
	ContractMismatchSchema = "schema_mismatch"
)

// ContractMismatch is a single structured contract mismatch between an
// upstream team's declared deliverables and a downstream team's input contract.
type ContractMismatch struct {
	Name     string `json:"name"`     // contract entry name
	Kind     string `json:"kind"`     // ContractMismatchMissing / Type / Format
	Expected string `json:"expected"` // downstream expectation (empty for missing)
	Actual   string `json:"actual"`   // upstream declaration (empty for missing)
}

// Warning renders the mismatch in the legacy advisory-warning string form.
func (m ContractMismatch) Warning() string {
	switch m.Kind {
	case ContractMismatchType:
		return fmt.Sprintf("contract type mismatch for %q: upstream=%q, downstream=%q", m.Name, m.Actual, m.Expected)
	case ContractMismatchFormat:
		return fmt.Sprintf("contract format mismatch for %q: upstream=%q, downstream=%q", m.Name, m.Actual, m.Expected)
	case ContractMismatchSchema:
		return fmt.Sprintf("contract schema mismatch for %q: %s", m.Name, m.Expected)
	default:
		return fmt.Sprintf("downstream contract %q has no matching upstream deliverable", m.Name)
	}
}

// ContractMismatchError is the runtime (tool-call level) contract validation
// failure returned by ReadUpstreamDeliverable: the reader team's InputContract
// does not match the upstream team's declared Deliverables. The structured
// Mismatches list makes the error LLM-actionable so the calling agent can
// auto-correct (wrong team_id /契约协商) and retry.
type ContractMismatchError struct {
	ReaderTeamID   string
	UpstreamTeamID string
	Mismatches     []ContractMismatch
}

// Error renders an LLM-actionable message naming both teams and every entry.
func (e *ContractMismatchError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "上游交付物契约不匹配：读取方团队 %q 的输入契约与上游团队 %q 声明的交付物不符 —", e.ReaderTeamID, e.UpstreamTeamID)
	for i, m := range e.Mismatches {
		if i > 0 {
			sb.WriteString("; ")
		}
		switch m.Kind {
		case ContractMismatchType:
			fmt.Fprintf(&sb, "%q 期望 type=%q 但上游声明 %q", m.Name, m.Expected, m.Actual)
		case ContractMismatchFormat:
			fmt.Fprintf(&sb, "%q 期望 format=%q 但上游声明 %q", m.Name, m.Expected, m.Actual)
		case ContractMismatchSchema:
			fmt.Fprintf(&sb, "%q 的内容不满足 schema 约束（%s）", m.Name, m.Expected)
		default:
			fmt.Fprintf(&sb, "%q 在上游交付物中不存在", m.Name)
		}
	}
	sb.WriteString("。请确认 team_id 是否正确；若正确，说明上下游交付物契约（name/type/format/schema）不一致，应调整任务分解或与上游团队协商后重试")
	return sb.String()
}

// DeliverableContractValidator validates contract matching between upstream and downstream teams.
type DeliverableContractValidator struct{}

func NewDeliverableContractValidator() *DeliverableContractValidator {
	return &DeliverableContractValidator{}
}

// ValidateContractMatch checks if upstream deliverables match downstream input contract.
// Returns warnings (not errors) since contracts are advisory, not blocking.
func (v *DeliverableContractValidator) ValidateContractMatch(upstream []DeliverableContract, downstream []DeliverableContract) []string {
	detailed := v.ValidateContractMatchDetailed(upstream, downstream)
	if len(detailed) == 0 {
		return nil
	}
	warnings := make([]string, 0, len(detailed))
	for _, m := range detailed {
		warnings = append(warnings, m.Warning())
	}
	return warnings
}

// ValidateContractMatchDetailed is the structured counterpart of
// ValidateContractMatch: same matching semantics, but returns one
// ContractMismatch per problem (missing / type / format) for programmatic
// consumers (e.g. ContractMismatchError at the tool-call level).
func (v *DeliverableContractValidator) ValidateContractMatchDetailed(upstream []DeliverableContract, downstream []DeliverableContract) []ContractMismatch {
	var out []ContractMismatch
	for _, input := range downstream {
		found := false
		for _, output := range upstream {
			if output.Name == input.Name {
				found = true
				if output.Type != input.Type {
					out = append(out, ContractMismatch{Name: input.Name, Kind: ContractMismatchType, Expected: input.Type, Actual: output.Type})
				}
				if output.Format != input.Format {
					out = append(out, ContractMismatch{Name: input.Name, Kind: ContractMismatchFormat, Expected: input.Format, Actual: output.Format})
				}
				break
			}
		}
		if !found {
			out = append(out, ContractMismatch{Name: input.Name, Kind: ContractMismatchMissing})
		}
	}
	return out
}

// ParseDeliverableContracts parses a JSON string into a slice of DeliverableContract.
func ParseDeliverableContracts(jsonStr string) ([]DeliverableContract, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var contracts []DeliverableContract
	if err := json.Unmarshal([]byte(jsonStr), &contracts); err != nil {
		return nil, err
	}
	return contracts, nil
}

// DeliverableContractsToJSON serializes a slice of DeliverableContract to JSON.
func DeliverableContractsToJSON(contracts []DeliverableContract) string {
	if len(contracts) == 0 {
		return "[]"
	}
	b, err := json.Marshal(contracts)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// ValidatePlanStepContracts advisory-checks contract matching across plan
// steps at dagRun startup (before dispatch). PlanStep contracts are persisted
// at PublishV2Board time, so all contracts are known before any team is
// lazily assembled — unlike SpiritTeamUsecase.ValidateDeliverableContracts
// which reads the teams table after assembly.
//
// For each step with dependencies and a non-empty InputContract, the
// deliverables of all its upstream steps are aggregated and matched via
// DeliverableContractValidator. Returns warnings only (never blocks).
func ValidatePlanStepContracts(steps []PlanStep) []string {
	validator := NewDeliverableContractValidator()
	deliverablesByID := make(map[string][]DeliverableContract, len(steps))
	for i := range steps {
		if len(steps[i].Deliverables) > 0 {
			deliverablesByID[steps[i].ID] = steps[i].Deliverables
		}
	}
	var allWarnings []string
	for i := range steps {
		step := &steps[i]
		if len(step.DependsOn) == 0 || len(step.InputContract) == 0 {
			continue
		}
		var upstream []DeliverableContract
		for _, depID := range step.DependsOn {
			upstream = append(upstream, deliverablesByID[depID]...)
		}
		allWarnings = append(allWarnings, validator.ValidateContractMatch(upstream, step.InputContract)...)
	}
	return allWarnings
}
