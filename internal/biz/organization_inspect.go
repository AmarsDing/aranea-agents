package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// OrgInspectEntry is one node in a governance-only org snapshot.
type OrgInspectEntry struct {
	Level     string `json:"level"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	ParentKey string `json:"parent_key,omitempty"`
	LeadHint  string `json:"lead_hint,omitempty"`
}

// OrgInspectView is the read-only org snapshot for GM / dept_lead.
type OrgInspectView struct {
	ScopeKey  string            `json:"scope_key"`
	ScopeName string            `json:"scope_name"`
	Entries   []OrgInspectEntry `json:"entries"`
}

// InspectGovernance returns the org subtree the caller is allowed to see.
// Company leads see the company tree; department leads see their department.
// focusKey, when set and inside the allowed subtree, narrows the snapshot.
// This is a query tool, not a dispatch tool — no plan_and_execute.
func (u *OrganizationUsecase) InspectGovernance(ctx context.Context, caller Agent, focusKey string) (OrgInspectView, error) {
	if u == nil || u.repo == nil {
		return OrgInspectView{}, apierror.Internal("ORG", "organization reader unavailable")
	}
	if !IsOrgGovernanceAgent(caller) {
		return OrgInspectView{}, apierror.Forbidden("ORG", "org inspect is limited to company_lead and dept_lead")
	}
	nodes, err := u.repo.ListOrgNodes(ctx)
	if err != nil {
		return OrgInspectView{}, err
	}
	return BuildGovernanceInspectView(nodes, caller, focusKey)
}

// BuildGovernanceInspectView is the pure tree-walk used by InspectGovernance.
func BuildGovernanceInspectView(nodes []OrganizationNode, caller Agent, focusKey string) (OrgInspectView, error) {
	if len(nodes) == 0 {
		return OrgInspectView{}, nil
	}
	byID := make(map[string]OrganizationNode, len(nodes))
	children := make(map[string][]OrganizationNode, len(nodes))
	for _, n := range nodes {
		if strings.TrimSpace(n.DeletedAt) != "" {
			continue
		}
		byID[n.ID] = n
		if n.ParentID != "" {
			children[n.ParentID] = append(children[n.ParentID], n)
		}
	}
	anchor, ok := locateCallerOrgNode(byID, caller)
	if !ok {
		return OrgInspectView{}, apierror.NotFound("ORG", "caller is not attached to an organization node")
	}
	root := scopeRoot(byID, anchor, IsCompanyLeadAgent(caller))
	if focusKey != "" {
		if focused, ok := findDescendantByKey(root, children, strings.TrimSpace(focusKey)); ok {
			root = focused
		}
	}
	entries := flattenOrgSubtree(root, byID, children)
	return OrgInspectView{
		ScopeKey:  root.Key,
		ScopeName: root.Name,
		Entries:   entries,
	}, nil
}

func locateCallerOrgNode(byID map[string]OrganizationNode, caller Agent) (OrganizationNode, bool) {
	if id := strings.TrimSpace(caller.PositionID); id != "" {
		if n, ok := byID[id]; ok {
			return n, true
		}
	}
	for _, n := range byID {
		if n.DeptLeadAgentID == caller.ID || n.CompanyLeadAgentID == caller.ID {
			return n, true
		}
	}
	return OrganizationNode{}, false
}

func scopeRoot(byID map[string]OrganizationNode, anchor OrganizationNode, companyWide bool) OrganizationNode {
	cur := anchor
	if companyWide {
		for cur.ParentID != "" {
			parent, ok := byID[cur.ParentID]
			if !ok {
				break
			}
			cur = parent
			if cur.Level == "company" {
				return cur
			}
		}
		return cur
	}
	if cur.Level == "department" {
		return cur
	}
	for cur.ParentID != "" {
		parent, ok := byID[cur.ParentID]
		if !ok {
			break
		}
		cur = parent
		if cur.Level == "department" {
			return cur
		}
	}
	return cur
}

func findDescendantByKey(root OrganizationNode, children map[string][]OrganizationNode, key string) (OrganizationNode, bool) {
	if root.Key == key {
		return root, true
	}
	var walk func(OrganizationNode) (OrganizationNode, bool)
	walk = func(n OrganizationNode) (OrganizationNode, bool) {
		for _, c := range children[n.ID] {
			if c.Key == key {
				return c, true
			}
			if found, ok := walk(c); ok {
				return found, true
			}
		}
		return OrganizationNode{}, false
	}
	return walk(root)
}

func flattenOrgSubtree(root OrganizationNode, byID map[string]OrganizationNode, children map[string][]OrganizationNode) []OrgInspectEntry {
	parentKeyOf := func(n OrganizationNode) string {
		if n.ParentID == "" {
			return ""
		}
		return byID[n.ParentID].Key
	}
	leadOf := func(n OrganizationNode) string {
		if n.Level == "company" && n.CompanyLeadAgentID != "" {
			return "company_lead"
		}
		if n.Level == "department" && n.DeptLeadAgentID != "" {
			return "dept_lead"
		}
		return ""
	}
	out := []OrgInspectEntry{{
		Level:     root.Level,
		Key:       root.Key,
		Name:      root.Name,
		ParentKey: parentKeyOf(root),
		LeadHint:  leadOf(root),
	}}
	var walk func(OrganizationNode)
	walk = func(n OrganizationNode) {
		for _, c := range children[n.ID] {
			out = append(out, OrgInspectEntry{
				Level:     c.Level,
				Key:       c.Key,
				Name:      c.Name,
				ParentKey: parentKeyOf(c),
				LeadHint:  leadOf(c),
			})
			walk(c)
		}
	}
	walk(root)
	return out
}
