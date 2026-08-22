package biz

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Playbook is a company-authorized delivery flow (Evolving).
type Playbook struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	AuthorizedBy string          `json:"authorized_by,omitempty"`
	AuthorizedAt string          `json:"authorized_at,omitempty"`
	Stages       []PlaybookStage `json:"stages"`
}

// PlaybookStage is one department slot in a playbook.
type PlaybookStage struct {
	ID               string   `json:"id"`
	DepartmentKey    string   `json:"department_key,omitempty"`
	DomainPath       string   `json:"domain_path,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	DeliverableNames []string `json:"deliverable_names,omitempty"`
	Specialty        string   `json:"specialty,omitempty"`
	CollectionIDs    []string `json:"collection_ids,omitempty"`
	ConfirmBefore    bool     `json:"confirm_before,omitempty"`
	GraphTemplateID  string   `json:"graph_template_id,omitempty"`
}

type companyMetadataPlaybooks struct {
	Playbooks []Playbook `json:"playbooks"`
}

// ParseCompanyPlaybooks reads playbooks[] from a company node's metadata_json.
func ParseCompanyPlaybooks(metadataJSON string) []Playbook {
	raw := strings.TrimSpace(metadataJSON)
	if raw == "" {
		return nil
	}
	var wrap companyMetadataPlaybooks
	if err := json.Unmarshal([]byte(raw), &wrap); err != nil {
		return nil
	}
	return wrap.Playbooks
}

// FindAuthorizedPlaybook returns the playbook with the given id if it has been authorized.
func FindAuthorizedPlaybook(playbooks []Playbook, id string) (Playbook, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Playbook{}, false
	}
	for _, pb := range playbooks {
		if pb.ID == id && strings.TrimSpace(pb.AuthorizedBy) != "" {
			return pb, true
		}
	}
	return Playbook{}, false
}

// ExpandPlaybook turns authorized stages into department-slot SubTasks.
// Does not invent specialties beyond what the playbook declared.
func ExpandPlaybook(pb Playbook) []SubTask {
	out := make([]SubTask, 0, len(pb.Stages))
	for i, st := range pb.Stages {
		id := strings.TrimSpace(st.ID)
		if id == "" {
			id = fmt.Sprintf("pb_%s_%d", pb.ID, i+1)
		}
		name := strings.TrimSpace(st.Specialty)
		if name == "" {
			name = strings.TrimSpace(st.DepartmentKey)
		}
		if name == "" {
			name = id
		}
		domain := strings.TrimSpace(st.DomainPath)
		if domain == "" {
			domain = strings.TrimSpace(st.Specialty)
		}
		var contracts []DeliverableContract
		for _, dn := range st.DeliverableNames {
			dn = strings.TrimSpace(dn)
			if dn == "" {
				continue
			}
			contracts = append(contracts, DeliverableContract{Name: dn})
		}
		out = append(out, SubTask{
			ID:           id,
			Name:         name,
			Description:  "playbook:" + pb.ID,
			DependsOn:    append([]string(nil), st.DependsOn...),
			DomainPath:   domain,
			Deliverables: contracts,
		})
	}
	return out
}

// ConstraintFingerprint is a stable hash of playbook id plus normalized constraints.
func ConstraintFingerprint(playbookID string, constraints map[string]string) string {
	keys := make([]string, 0, len(constraints))
	for k := range constraints {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(strings.TrimSpace(playbookID))
	b.WriteByte('\n')
	for _, k := range keys {
		b.WriteString(strings.TrimSpace(k))
		b.WriteByte('=')
		b.WriteString(strings.TrimSpace(constraints[k]))
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:16])
}

// RecipeKeysReusable reports whether historical AgentKeys may be reused.
// Empty fingerprints (legacy recipes or unknown current constraints) stay compatible.
func RecipeKeysReusable(entryFingerprint, wantFingerprint string) bool {
	want := strings.TrimSpace(wantFingerprint)
	have := strings.TrimSpace(entryFingerprint)
	if want == "" || have == "" {
		return true
	}
	return want == have
}
