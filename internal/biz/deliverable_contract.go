package biz

import (
	"encoding/json"
	"fmt"
)

// DeliverableContract defines the input/output contract between teams.
type DeliverableContract struct {
	Name        string `json:"name"`   // e.g., "design_spec"
	Type        string `json:"type"`   // e.g., "document", "code", "data"
	Format      string `json:"format"` // e.g., "markdown", "json", "zip"
	Description string `json:"description"`
}

// DeliverableContractValidator validates contract matching between upstream and downstream teams.
type DeliverableContractValidator struct{}

func NewDeliverableContractValidator() *DeliverableContractValidator {
	return &DeliverableContractValidator{}
}

// ValidateContractMatch checks if upstream deliverables match downstream input contract.
// Returns warnings (not errors) since contracts are advisory, not blocking.
func (v *DeliverableContractValidator) ValidateContractMatch(upstream []DeliverableContract, downstream []DeliverableContract) []string {
	var warnings []string
	for _, input := range downstream {
		found := false
		for _, output := range upstream {
			if output.Name == input.Name {
				found = true
				if output.Type != input.Type {
					warnings = append(warnings, fmt.Sprintf("contract type mismatch for %q: upstream=%q, downstream=%q", input.Name, output.Type, input.Type))
				}
				if output.Format != input.Format {
					warnings = append(warnings, fmt.Sprintf("contract format mismatch for %q: upstream=%q, downstream=%q", input.Name, output.Format, input.Format))
				}
				break
			}
		}
		if !found {
			warnings = append(warnings, fmt.Sprintf("downstream contract %q has no matching upstream deliverable", input.Name))
		}
	}
	return warnings
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
