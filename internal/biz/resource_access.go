package biz

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// M71: 受控资源访问层 — memberfs（部门主管只读访问本部门员工工作目录）
// 统一编排：权限校验 → 范围解析 → 审计落库（fail-closed）。
// 设计：docs/development/71-agent-resource-sharing.design.md
// ---------------------------------------------------------------------------

// 访问者角色。
const (
	RoleDeptLead = "dept_lead"
	RoleSpirit   = "spirit"
)

// 访问关系类型。
const (
	RelationOrgHome   = "org_home"
	RelationTeamOwner = "team_owner"
	RelationNone      = "none"
)

// 审计结果。
const (
	ResultAllowed = "allowed"
	ResultDenied  = "denied"
)

// 审计动作。
const (
	ActionListFiles      = "list_files"
	ActionReadFile       = "read_file"
	ActionSearchFiles    = "search_files"
	ActionSendMail       = "send_mail"
	ActionReadMail       = "read_mail"
	ActionReplyMail      = "reply_mail"
	ActionSearchMessages = "search_messages"
	ActionListSessions   = "list_sessions"
	ActionReadSession    = "read_session"
)

const domainResourceAccess = "RESOURCE_ACCESS"

// memberfs 限制常量。
const (
	DefaultMemberDirDepth           = 2
	MaxMemberDirDepth               = 4
	DefaultMemberFileMaxBytes int64 = 200 * 1024
	MaxMemberFileMaxBytes     int64 = 200 * 1024
	MaxMemberSearchResults          = 200
)

// AuditEntry is one row in resource_access_audits.
type AuditEntry struct {
	ActorAgentID  string
	ActorRole     string
	Action        string
	TargetAgentID string
	TargetDeptID  string
	Relation      string
	ResourceURI   string
	Result        string
	DenyReason    string
}

// FileEntry is one node in a member workspace directory tree.
type FileEntry struct {
	Path  string `json:"path"` // relative path from agent workspace root
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ---------------------------------------------------------------------------
// 端口（Stability:evolving）
// ---------------------------------------------------------------------------

// MemberFileReader performs the actual (read-only) filesystem operations.
// Implemented in the service layer (os file access + path safety).
// Stability:evolving
type MemberFileReader interface {
	List(root, subdir string, depth int) ([]FileEntry, error)
	ReadText(root, rel string, maxBytes int64) (content string, truncated bool, err error)
	Search(root, pattern string, limit int) ([]string, error)
}

// MemberDirResolver resolves an agent key to its workspace directory.
// Implemented in the service layer (same layout as resolveAgentFilesystemDir).
// Stability:evolving
type MemberDirResolver interface {
	ResolveDir(ctx context.Context, agentKey string) (string, error)
}

// AccessAuditor persists audit entries. Implementations must be fail-closed:
// a returned error denies the access.
// Stability:evolving
type AccessAuditor interface {
	Record(ctx context.Context, e AuditEntry) error
}

// ---------------------------------------------------------------------------
// ResourceAccessUsecase
// ---------------------------------------------------------------------------

// ResourceAccessUsecaseDeps groups the dependencies for ResourceAccessUsecase.
type ResourceAccessUsecaseDeps struct {
	Agents      AgentReader
	Org         OrganizationReader
	TeamLister  DeptTeamLister
	FileReader  MemberFileReader
	DirResolver MemberDirResolver
	Auditor     AccessAuditor
	Lg          loggateway.Logger
}

// ResourceAccessUsecase authorizes and audits memberfs access (FR-01~FR-04).
type ResourceAccessUsecase struct {
	agents      AgentReader
	org         OrganizationReader
	teamLister  DeptTeamLister
	fileReader  MemberFileReader
	dirResolver MemberDirResolver
	auditor     AccessAuditor
	lg          loggateway.Logger
}

// NewResourceAccessUsecase creates the usecase.
func NewResourceAccessUsecase(deps ResourceAccessUsecaseDeps) *ResourceAccessUsecase {
	lg := deps.Lg
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ResourceAccessUsecase{
		agents:      deps.Agents,
		org:         deps.Org,
		teamLister:  deps.TeamLister,
		fileReader:  deps.FileReader,
		dirResolver: deps.DirResolver,
		auditor:     deps.Auditor,
		lg:          lg.With(loggateway.Domain("resource_access")),
	}
}

// ListMemberFiles lists the target agent's workspace directory tree (FR-01).
func (u *ResourceAccessUsecase) ListMemberFiles(ctx context.Context, callerAgentID, targetAgentKey, subdir string, depth int) ([]FileEntry, error) {
	if depth <= 0 {
		depth = DefaultMemberDirDepth
	}
	if depth > MaxMemberDirDepth {
		depth = MaxMemberDirDepth
	}
	_, dir, err := u.guardMemberDir(ctx, callerAgentID, targetAgentKey, ActionListFiles, "dir:"+strings.TrimSpace(subdir))
	if err != nil {
		return nil, err
	}
	entries, err := u.fileReader.List(dir, subdir, depth)
	if err != nil {
		return nil, apierror.BadRequest(domainResourceAccess, "list failed: %s", err)
	}
	return entries, nil
}

// ReadMemberFile reads a text file from the target agent's workspace (FR-02).
func (u *ResourceAccessUsecase) ReadMemberFile(ctx context.Context, callerAgentID, targetAgentKey, rel string, maxBytes int64) (string, bool, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMemberFileMaxBytes
	}
	if maxBytes > MaxMemberFileMaxBytes {
		maxBytes = MaxMemberFileMaxBytes
	}
	_, dir, err := u.guardMemberDir(ctx, callerAgentID, targetAgentKey, ActionReadFile, "file:"+strings.TrimSpace(rel))
	if err != nil {
		return "", false, err
	}
	content, truncated, err := u.fileReader.ReadText(dir, rel, maxBytes)
	if err != nil {
		return "", false, apierror.BadRequest(domainResourceAccess, "read failed: %s", err)
	}
	return content, truncated, nil
}

// SearchMemberFiles searches file names by glob pattern in the target agent's workspace (FR-03).
func (u *ResourceAccessUsecase) SearchMemberFiles(ctx context.Context, callerAgentID, targetAgentKey, pattern string, limit int) ([]string, error) {
	if limit <= 0 || limit > MaxMemberSearchResults {
		limit = MaxMemberSearchResults
	}
	_, dir, err := u.guardMemberDir(ctx, callerAgentID, targetAgentKey, ActionSearchFiles, "glob:"+strings.TrimSpace(pattern))
	if err != nil {
		return nil, err
	}
	paths, err := u.fileReader.Search(dir, pattern, limit)
	if err != nil {
		return nil, apierror.BadRequest(domainResourceAccess, "search failed: %s", err)
	}
	return paths, nil
}

// guardMemberDir runs authorization + dir resolution + audit (fail-closed).
// Returns the relation and the resolved workspace dir on success.
func (u *ResourceAccessUsecase) guardMemberDir(ctx context.Context, callerAgentID, targetAgentKey, action, uri string) (string, string, error) {
	relation, targetAgentID, denyReason, err := u.authorizeMemberDir(ctx, callerAgentID, targetAgentKey)
	entry := AuditEntry{
		ActorAgentID:  callerAgentID,
		ActorRole:     RoleDeptLead,
		Action:        action,
		TargetAgentID: targetAgentID,
		Relation:      relation,
		ResourceURI:   uri,
	}
	if err != nil || denyReason != "" {
		entry.Result = ResultDenied
		entry.DenyReason = denyReason
		if err != nil {
			entry.DenyReason = err.Error()
		}
		if aErr := u.audit(ctx, entry); aErr != nil {
			return "", "", apierror.Internal(domainResourceAccess, "audit failed: %s", aErr)
		}
		if err != nil {
			return "", "", err
		}
		return "", "", apierror.Forbidden(domainResourceAccess, "%s", denyReason)
	}

	dir, err := u.dirResolver.ResolveDir(ctx, targetAgentKey)
	if err != nil {
		entry.Result = ResultDenied
		entry.DenyReason = "workspace dir unresolved: " + err.Error()
		if aErr := u.audit(ctx, entry); aErr != nil {
			return "", "", apierror.Internal(domainResourceAccess, "audit failed: %s", aErr)
		}
		return "", "", apierror.NotFound(domainResourceAccess, "target agent workspace not found: %s", err)
	}

	entry.Result = ResultAllowed
	if aErr := u.audit(ctx, entry); aErr != nil {
		return "", "", apierror.Internal(domainResourceAccess, "audit failed: %s", aErr)
	}
	return relation, dir, nil
}

// authorizeMemberDir decides whether caller (must be a dept lead) may read
// target agent's workspace. Returns (relation, targetAgentID, denyReason, err).
// err is a hard failure (lookup errors); denyReason is a policy denial.
func (u *ResourceAccessUsecase) authorizeMemberDir(ctx context.Context, callerAgentID, targetAgentKey string) (string, string, string, error) {
	caller, err := u.agents.GetAgentByID(ctx, callerAgentID)
	if err != nil {
		return RelationNone, "", "", apierror.Forbidden(domainResourceAccess, "caller agent not found")
	}
	if !IsDeptLeadAgent(caller) {
		return RelationNone, "", "", apierror.Forbidden(domainResourceAccess, "only department lead agents may access member workspaces")
	}
	callerDeptID, err := u.agentDepartment(ctx, caller)
	if err != nil {
		return RelationNone, "", "", err
	}
	if callerDeptID == "" {
		return RelationNone, "", "", apierror.Forbidden(domainResourceAccess, "caller is not attached to a department")
	}

	targetKey := strings.TrimSpace(targetAgentKey)
	if targetKey == "" {
		return RelationNone, "", "target agent_key is required", nil
	}
	target, err := u.agents.GetAgentByAgentKey(ctx, targetKey)
	if err != nil {
		return RelationNone, "", "target agent not found: " + targetKey, nil
	}

	// org_home: target's owning department == caller's department.
	targetDeptID, err := u.agentDepartment(ctx, target)
	if err != nil {
		return RelationNone, target.ID, "", err
	}
	if targetDeptID != "" && targetDeptID == callerDeptID {
		return RelationOrgHome, target.ID, "", nil
	}

	// team_owner: target is a cross-dept member of a non-archived team owned by caller's department.
	if u.teamLister != nil {
		teams, err := u.teamLister.ListTeamsByDepartmentID(ctx, callerDeptID)
		if err != nil {
			return RelationNone, target.ID, "", err
		}
		for _, t := range teams {
			if t.Status == TeamStatusArchived || t.DeletedAt != "" {
				continue
			}
			if crossDeptMemberContains(t.CrossDeptMemberIDs, target.ID) {
				return RelationTeamOwner, target.ID, "", nil
			}
		}
	}

	return RelationNone, target.ID, "target agent is neither in your department nor borrowed by your department's teams", nil
}

// agentDepartment resolves the department ID for an agent via its position in
// the org tree (position node itself, or its parent — mirrors
// DeptLeadManager.agentDepartment).
func (u *ResourceAccessUsecase) agentDepartment(ctx context.Context, a Agent) (string, error) {
	deptID, err := resolveAgentDepartment(ctx, u.org, a)
	if err != nil {
		return "", apierror.Internal(domainResourceAccess, "department lookup failed: %s", err)
	}
	return deptID, nil
}

// resolveAgentDepartment resolves the department ID for an agent via its
// position node (the node itself when level=department, otherwise its parent
// department). Shared by ResourceAccessUsecase and DeptMailboxUsecase (M71) so
// both apply identical department-attachment semantics.
func resolveAgentDepartment(ctx context.Context, org OrganizationReader, a Agent) (string, error) {
	if a.PositionID == "" {
		return "", nil
	}
	pos, err := org.GetOrgNode(ctx, a.PositionID)
	if err != nil {
		return "", err
	}
	if pos.Level == "department" {
		return pos.ID, nil
	}
	if pos.ParentID != "" {
		parent, err := org.GetOrgNode(ctx, pos.ParentID)
		if err != nil {
			return "", err
		}
		if parent.Level == "department" {
			return parent.ID, nil
		}
	}
	return "", nil
}

// audit records an entry; nil auditor is treated as failure (fail-closed, NFR-06).
func (u *ResourceAccessUsecase) audit(ctx context.Context, e AuditEntry) error {
	if u.auditor == nil {
		return apierror.Internal(domainResourceAccess, "auditor not configured")
	}
	return u.auditor.Record(ctx, e)
}

// IsDeptLeadAgent reports whether the agent is a department lead.
func IsDeptLeadAgent(a Agent) bool {
	if a.AgentVariant == "dept_lead" {
		return true
	}
	key := strings.TrimSpace(a.AgentKey)
	return strings.HasPrefix(key, DeptLeadAgentKeyPrefix) && strings.HasSuffix(key, "__")
}

// IsCompanyLeadAgent reports whether the agent is a company general manager.
func IsCompanyLeadAgent(a Agent) bool {
	if a.AgentVariant == AgentVariantCompanyLead {
		return true
	}
	key := strings.TrimSpace(a.AgentKey)
	return strings.HasPrefix(key, CompanyLeadAgentKeyPrefix) && strings.HasSuffix(key, "__")
}

// IsOrgGovernanceAgent reports whether the agent is a dept or company lead.
func IsOrgGovernanceAgent(a Agent) bool {
	return IsDeptLeadAgent(a) || IsCompanyLeadAgent(a)
}

// crossDeptMemberContains reports whether the JSON array crossDeptMemberIDs
// contains the given agent ID.
func crossDeptMemberContains(crossDeptMemberIDsJSON, agentID string) bool {
	if strings.TrimSpace(crossDeptMemberIDsJSON) == "" || agentID == "" {
		return false
	}
	var ids []string
	if err := json.Unmarshal([]byte(crossDeptMemberIDsJSON), &ids); err != nil {
		return false
	}
	for _, id := range ids {
		if id == agentID {
			return true
		}
	}
	return false
}
