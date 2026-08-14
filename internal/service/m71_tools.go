package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/deptmail"
	"aranea-agents/internal/tools/memberfs"
	"aranea-agents/internal/tools/sessionaccess"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ---------------------------------------------------------------------------
// M71: agent resource sharing tool assembly (identity-bound at assembly time)
// ---------------------------------------------------------------------------

// memberFSDeptMailTools assembles memberfs (3 tools) + deptmail (4 tools) for
// department lead agents. Other agents get nothing — the usecases re-verify
// the dept_lead identity on every call (defense in depth).
func (o *ChatOrchestrator) memberFSDeptMailTools(ag biz.Agent) []trpctool.Tool {
	if o == nil || !biz.IsDeptLeadAgent(ag) {
		return nil
	}
	var out []trpctool.Tool
	if o.rt().Sharing.ResourceAccess != nil {
		out = append(out, memberfs.RegisterAll(o.rt().Sharing.ResourceAccess, ag.ID, o.lg())...)
	}
	if o.rt().Sharing.DeptMailbox != nil {
		out = append(out, deptmail.RegisterAll(o.rt().Sharing.DeptMailbox, ag.ID, o.lg())...)
	}
	return out
}

// sessionAccessTools assembles sessionaccess (3 tools) for the spirit agent.
func (o *ChatOrchestrator) sessionAccessTools(ag biz.Agent) []trpctool.Tool {
	if o == nil || o.rt().Sharing.SessionSearch == nil {
		return nil
	}
	if strings.TrimSpace(ag.AgentKey) != biz.SpiritAgentKey {
		return nil
	}
	return sessionaccess.RegisterAll(o.rt().Sharing.SessionSearch, ag.ID, o.lg())
}
