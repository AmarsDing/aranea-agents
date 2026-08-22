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

// TryPlaybookForTask returns an authorized playbook only when the task names
// its id or name. A sole authorized playbook is NOT auto-picked on every task
// (that would force every light request onto the heavy chain).
func TryPlaybookForTask(metadataJSON, taskText string) (Playbook, []SubTask, bool) {
	pbs := ParseCompanyPlaybooks(metadataJSON)
	if id := playbookIDMentioned(taskText, pbs); id != "" {
		if pb, ok := FindAuthorizedPlaybook(pbs, id); ok {
			return pb, ExpandPlaybook(pb), true
		}
	}
	return Playbook{}, nil, false
}

// TrySoleAuthorizedPlaybook expands the company's only authorized playbook.
// Callers must already be on the heavy gear; do not use on light/medium.
func TrySoleAuthorizedPlaybook(metadataJSON string) (Playbook, []SubTask, bool) {
	var authorized []Playbook
	for _, pb := range ParseCompanyPlaybooks(metadataJSON) {
		if strings.TrimSpace(pb.AuthorizedBy) != "" {
			authorized = append(authorized, pb)
		}
	}
	if len(authorized) != 1 {
		return Playbook{}, nil, false
	}
	return authorized[0], ExpandPlaybook(authorized[0]), true
}

func playbookIDMentioned(taskText string, pbs []Playbook) string {
	s := strings.TrimSpace(taskText)
	if s == "" {
		return ""
	}
	for _, pb := range pbs {
		if pb.ID != "" && strings.Contains(s, pb.ID) {
			return pb.ID
		}
		if pb.Name != "" && strings.Contains(s, pb.Name) {
			return pb.ID
		}
	}
	return ""
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
			ID:              id,
			Name:            name,
			Description:     "playbook:" + pb.ID,
			DependsOn:       append([]string(nil), st.DependsOn...),
			DomainPath:      domain,
			Deliverables:    contracts,
			GraphTemplateID: strings.TrimSpace(st.GraphTemplateID),
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

// PlaybookFillRequiredReason is returned when the user asked for the org chain
// but the company has no authorized playbook. TaskPlanner must not invent jobs.
const PlaybookFillRequiredReason = "playbook_fill_required"

// PlaybookFillUserHint is the Spirit-facing next action (Chinese).
const PlaybookFillUserHint = "该公司还没有已授权的流程剧本。请先让总经理授权一本剧本（或在任务里点名剧本 id），不要按行业常识拆岗。"

// MergePlaybookIntoMetadata upserts a playbook into company metadata_json.
func MergePlaybookIntoMetadata(raw string, pb Playbook) string {
	if strings.TrimSpace(pb.ID) == "" {
		return raw
	}
	var m map[string]any
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			m = map[string]any{}
		}
	}
	if m == nil {
		m = map[string]any{}
	}
	existing := ParseCompanyPlaybooks(raw)
	replaced := false
	for i, p := range existing {
		if p.ID == pb.ID {
			existing[i] = pb
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, pb)
	}
	m["playbooks"] = existing
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return string(b)
}

// AuthorizePlaybookOnCompany stamps authorized_by and writes the playbook into metadata.
func AuthorizePlaybookOnCompany(n *OrganizationNode, pb Playbook) {
	if n == nil {
		return
	}
	if strings.TrimSpace(pb.AuthorizedBy) == "" {
		pb.AuthorizedBy = fmt.Sprintf("%s%s__", CompanyLeadAgentKeyPrefix, n.Key)
	}
	if strings.TrimSpace(pb.AuthorizedAt) == "" {
		pb.AuthorizedAt = "now"
	}
	n.MetadataJSON = MergePlaybookIntoMetadata(n.MetadataJSON, pb)
}

// FilterRecipeAgentKeys drops governance leads from a recipe key list.
func FilterRecipeAgentKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if IsOrgGovernanceAgent(Agent{AgentKey: k}) {
			continue
		}
		out = append(out, k)
	}
	return out
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
