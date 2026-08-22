package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// M71 stubs
// ---------------------------------------------------------------------------

type m71AgentReader struct {
	byID  map[string]Agent
	byKey map[string]Agent
}

func (s *m71AgentReader) SearchAgents(context.Context, AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}
func (s *m71AgentReader) GetAgentByID(_ context.Context, id string) (Agent, error) {
	if a, ok := s.byID[id]; ok {
		return a, nil
	}
	return Agent{}, shared.ErrNotFound
}
func (s *m71AgentReader) GetAgentByAgentKey(_ context.Context, key string) (Agent, error) {
	if a, ok := s.byKey[key]; ok {
		return a, nil
	}
	return Agent{}, shared.ErrNotFound
}
func (s *m71AgentReader) ListExtrasForAgents(context.Context, []string) (map[string]AgentListExtras, error) {
	return map[string]AgentListExtras{}, nil
}
func (s *m71AgentReader) ListAgentsByIDs(_ context.Context, ids []string) ([]Agent, error) {
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		if a, ok := s.byID[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

type m71OrgReader struct {
	nodes map[string]OrganizationNode
	err   error
}

func (s *m71OrgReader) GetOrgNode(_ context.Context, id string) (OrganizationNode, error) {
	if s.err != nil {
		return OrganizationNode{}, s.err
	}
	if n, ok := s.nodes[id]; ok {
		return n, nil
	}
	return OrganizationNode{}, shared.ErrNotFound
}
func (s *m71OrgReader) GetOrgNodeByKey(context.Context, string) (OrganizationNode, error) {
	return OrganizationNode{}, shared.ErrNotFound
}
func (s *m71OrgReader) ListOrgNodes(context.Context) ([]OrganizationNode, error) { return nil, nil }
func (s *m71OrgReader) ListOrgNodesByLevel(context.Context, string) ([]OrganizationNode, error) {
	return nil, nil
}
func (s *m71OrgReader) ListOrgNodesByParentID(context.Context, string) ([]OrganizationNode, error) {
	return nil, nil
}
func (s *m71OrgReader) ListOrgNodesByIDs(context.Context, []string) ([]OrganizationNode, error) {
	return nil, nil
}

type m71TeamLister struct {
	teams []Team
	err   error
}

func (s *m71TeamLister) ListTeamsByDepartmentID(context.Context, string) ([]Team, error) {
	return s.teams, s.err
}

type m71FileReader struct{}

func (m71FileReader) List(string, string, int) ([]FileEntry, error) {
	return []FileEntry{{Path: "a.txt", IsDir: false, Size: 3}}, nil
}
func (m71FileReader) ReadText(string, string, int64) (string, bool, error) {
	return "hello", false, nil
}
func (m71FileReader) Search(string, string, int) ([]string, error) {
	return []string{"a.txt"}, nil
}

type m71DirResolver struct {
	dir string
	err error
}

func (s m71DirResolver) ResolveDir(context.Context, string) (string, error) { return s.dir, s.err }

type m71Auditor struct {
	entries []AuditEntry
	err     error
}

func (s *m71Auditor) Record(_ context.Context, e AuditEntry) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, e)
	return nil
}

func (s *m71Auditor) last() AuditEntry { return s.entries[len(s.entries)-1] }

func (s *m71Auditor) countByResult(result string) int {
	n := 0
	for _, e := range s.entries {
		if e.Result == result {
			n++
		}
	}
	return n
}

// --- fixtures ---

const (
	m71DeptA = "dept-a"
	m71DeptB = "dept-b"
	m71PosA1 = "pos-a1"
	m71PosB1 = "pos-b1"
)

func m71OrgFixture() *m71OrgReader {
	return &m71OrgReader{nodes: map[string]OrganizationNode{
		m71DeptA: {ID: m71DeptA, Key: "dept-a", Name: "部门A", Level: "department", DeptLeadAgentID: "lead-a"},
		m71DeptB: {ID: m71DeptB, Key: "dept-b", Name: "部门B", Level: "department", DeptLeadAgentID: "lead-b"},
		m71PosA1: {ID: m71PosA1, Key: "pos-a1", Name: "岗位A1", Level: "position", ParentID: m71DeptA},
		m71PosB1: {ID: m71PosB1, Key: "pos-b1", Name: "岗位B1", Level: "position", ParentID: m71DeptB},
	}}
}

func m71LeadAgent(id, deptPosID string) Agent {
	return Agent{ID: id, AgentKey: DeptLeadAgentKeyPrefix + id + "__", AgentVariant: "dept_lead", PositionID: deptPosID}
}

func m71MemberAgent(id, key, posID string) Agent {
	return Agent{ID: id, AgentKey: key, PositionID: posID}
}

func newM71ResourceAccess(agents *m71AgentReader, org *m71OrgReader, teams *m71TeamLister, auditor *m71Auditor) *ResourceAccessUsecase {
	return NewResourceAccessUsecase(ResourceAccessUsecaseDeps{
		Agents:      agents,
		Org:         org,
		TeamLister:  teams,
		FileReader:  m71FileReader{},
		DirResolver: m71DirResolver{dir: "/tmp/workspace/ws1/member"},
		Auditor:     auditor,
		Lg:          loggateway.NewNoop(),
	})
}

// ---------------------------------------------------------------------------
// ResourceAccessUsecase：权限矩阵（FR-01~FR-04 + NFR-06 fail-closed）
// ---------------------------------------------------------------------------

func TestResourceAccess_PermissionMatrix(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)                // dept lead of A (position = dept node)
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1) // member of A
	memberB := m71MemberAgent("mem-b", "worker_b", m71PosB1) // member of B
	plainAgent := m71MemberAgent("plain", "plain_x", m71PosA1)

	tests := []struct {
		name         string
		caller       Agent
		targetKey    string
		teams        []Team
		wantErr      bool
		wantRelation string
	}{
		{
			name:         "dept_lead reads own-department member (org_home)",
			caller:       leadA,
			targetKey:    memberA.AgentKey,
			wantRelation: RelationOrgHome,
		},
		{
			name:      "dept_lead reads borrowed member (team_owner)",
			caller:    leadA,
			targetKey: memberB.AgentKey,
			teams: []Team{{
				ID:                 "team-1",
				Status:             "active",
				CrossDeptMemberIDs: `["mem-b"]`,
			}},
			wantRelation: RelationTeamOwner,
		},
		{
			name:      "archived team borrow does not grant access",
			caller:    leadA,
			targetKey: memberB.AgentKey,
			teams: []Team{{
				ID:                 "team-1",
				Status:             TeamStatusArchived,
				CrossDeptMemberIDs: `["mem-b"]`,
			}},
			wantErr: true,
		},
		{
			name:      "deleted team borrow does not grant access",
			caller:    leadA,
			targetKey: memberB.AgentKey,
			teams: []Team{{
				ID:                 "team-1",
				Status:             "active",
				DeletedAt:          "2026-01-01T00:00:00Z",
				CrossDeptMemberIDs: `["mem-b"]`,
			}},
			wantErr: true,
		},
		{
			name:      "dept_lead denied for unrelated member",
			caller:    leadA,
			targetKey: memberB.AgentKey,
			wantErr:   true,
		},
		{
			name:      "non-dept-lead caller denied",
			caller:    plainAgent,
			targetKey: memberA.AgentKey,
			wantErr:   true,
		},
		{
			name:      "unknown target key denied",
			caller:    leadA,
			targetKey: "ghost",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agents := &m71AgentReader{
				byID:  map[string]Agent{leadA.ID: leadA, memberA.ID: memberA, memberB.ID: memberB, plainAgent.ID: plainAgent},
				byKey: map[string]Agent{memberA.AgentKey: memberA, memberB.AgentKey: memberB},
			}
			auditor := &m71Auditor{}
			uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{teams: tt.teams}, auditor)

			entries, err := uc.ListMemberFiles(context.Background(), tt.caller.ID, tt.targetKey, "", 0)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected denial, got success: %+v", entries)
				}
				if auditor.countByResult(ResultDenied) == 0 {
					t.Fatalf("expected a denied audit entry, got %+v", auditor.entries)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected allow, got error: %v", err)
			}
			if len(entries) == 0 {
				t.Fatal("expected file entries")
			}
			last := auditor.last()
			if last.Result != ResultAllowed {
				t.Fatalf("expected allowed audit, got %+v", last)
			}
			if last.Relation != tt.wantRelation {
				t.Fatalf("relation = %s, want %s", last.Relation, tt.wantRelation)
			}
		})
	}
}

func TestResourceAccess_CallerNotFound(t *testing.T) {
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(&m71AgentReader{byID: map[string]Agent{}, byKey: map[string]Agent{}}, m71OrgFixture(), &m71TeamLister{}, auditor)
	if _, err := uc.ListMemberFiles(context.Background(), "ghost", "worker_a", "", 0); err == nil {
		t.Fatal("expected error for unknown caller")
	}
}

func TestResourceAccess_CallerWithoutDepartment(t *testing.T) {
	leadNoDept := Agent{ID: "lead-x", AgentKey: DeptLeadAgentKeyPrefix + "x__", AgentVariant: "dept_lead", PositionID: ""}
	agents := &m71AgentReader{byID: map[string]Agent{leadNoDept.ID: leadNoDept}, byKey: map[string]Agent{}}
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, auditor)
	if _, err := uc.ListMemberFiles(context.Background(), leadNoDept.ID, "worker_a", "", 0); err == nil {
		t.Fatal("expected error for dept lead without department")
	}
}

func TestResourceAccess_LeadByKeyPrefixOnly(t *testing.T) {
	// AgentVariant empty; identified via AgentKey prefix + suffix.
	lead := Agent{ID: "lead-a", AgentKey: DeptLeadAgentKeyPrefix + "dept-a__", PositionID: m71DeptA}
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1)
	agents := &m71AgentReader{
		byID:  map[string]Agent{lead.ID: lead, memberA.ID: memberA},
		byKey: map[string]Agent{memberA.AgentKey: memberA},
	}
	auditor := &m71Auditor{}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, auditor)
	if _, err := uc.ListMemberFiles(context.Background(), lead.ID, memberA.AgentKey, "", 0); err != nil {
		t.Fatalf("expected allow via key-prefix identification, got %v", err)
	}
}

func TestResourceAccess_FailClosedOnAuditError(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1)
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, memberA.ID: memberA},
		byKey: map[string]Agent{memberA.AgentKey: memberA},
	}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, &m71Auditor{err: errors.New("db down")})
	_, err := uc.ListMemberFiles(context.Background(), leadA.ID, memberA.AgentKey, "", 0)
	if err == nil {
		t.Fatal("expected access denied when audit write fails (fail-closed)")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeInternal {
		t.Fatalf("expected internal audit error, got %v", err)
	}
}

func TestResourceAccess_NilAuditorDenies(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1)
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, memberA.ID: memberA},
		byKey: map[string]Agent{memberA.AgentKey: memberA},
	}
	uc := NewResourceAccessUsecase(ResourceAccessUsecaseDeps{
		Agents: agents, Org: m71OrgFixture(), TeamLister: &m71TeamLister{},
		FileReader: m71FileReader{}, DirResolver: m71DirResolver{dir: "/x"},
		Auditor: nil, Lg: loggateway.NewNoop(),
	})
	if _, err := uc.ListMemberFiles(context.Background(), leadA.ID, memberA.AgentKey, "", 0); err == nil {
		t.Fatal("expected access denied when auditor is nil (fail-closed)")
	}
}

func TestResourceAccess_DirResolveFailureAudited(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1)
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, memberA.ID: memberA},
		byKey: map[string]Agent{memberA.AgentKey: memberA},
	}
	auditor := &m71Auditor{}
	uc := NewResourceAccessUsecase(ResourceAccessUsecaseDeps{
		Agents: agents, Org: m71OrgFixture(), TeamLister: &m71TeamLister{},
		FileReader: m71FileReader{}, DirResolver: m71DirResolver{err: errors.New("no dir")},
		Auditor: auditor, Lg: loggateway.NewNoop(),
	})
	if _, _, err := uc.ReadMemberFile(context.Background(), leadA.ID, memberA.AgentKey, "a.txt", 0); err == nil {
		t.Fatal("expected error when workspace dir unresolvable")
	}
	if auditor.last().Result != ResultDenied {
		t.Fatalf("expected denied audit for unresolved dir, got %+v", auditor.last())
	}
}

func TestResourceAccess_ReadAndSearchClamp(t *testing.T) {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	memberA := m71MemberAgent("mem-a", "worker_a", m71PosA1)
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, memberA.ID: memberA},
		byKey: map[string]Agent{memberA.AgentKey: memberA},
	}
	uc := newM71ResourceAccess(agents, m71OrgFixture(), &m71TeamLister{}, &m71Auditor{})
	content, truncated, err := uc.ReadMemberFile(context.Background(), leadA.ID, memberA.AgentKey, "a.txt", 10*MaxMemberFileMaxBytes)
	if err != nil || content != "hello" || truncated {
		t.Fatalf("unexpected read result: content=%q truncated=%v err=%v", content, truncated, err)
	}
	matches, err := uc.SearchMemberFiles(context.Background(), leadA.ID, memberA.AgentKey, "*.txt", 99999)
	if err != nil || len(matches) != 1 {
		t.Fatalf("unexpected search result: %+v err=%v", matches, err)
	}
}

// ---------------------------------------------------------------------------
// DeptMailboxUsecase：收发读回 + 唤醒防抖（FR-05~FR-07 + US-05）
// ---------------------------------------------------------------------------

type m71MailboxRepo struct {
	created []DeptLeadMessage
	byID    map[string]*DeptLeadMessage
}

func newM71MailboxRepo() *m71MailboxRepo {
	return &m71MailboxRepo{byID: map[string]*DeptLeadMessage{}}
}

func (s *m71MailboxRepo) CreateMessage(_ context.Context, msg DeptLeadMessage) (DeptLeadMessage, error) {
	cp := msg
	s.byID[msg.ID] = &cp
	s.created = append(s.created, msg)
	return msg, nil
}
func (s *m71MailboxRepo) ListInbox(_ context.Context, toAgentID, status string, limit int) ([]DeptLeadMessage, error) {
	var out []DeptLeadMessage
	for i := len(s.created) - 1; i >= 0 && len(out) < limit; i-- {
		m := s.created[i]
		if m.ToAgentID != toAgentID {
			continue
		}
		if status != "" && m.Status != status {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
func (s *m71MailboxRepo) GetMessage(_ context.Context, id string) (DeptLeadMessage, error) {
	if m, ok := s.byID[id]; ok {
		return *m, nil
	}
	return DeptLeadMessage{}, shared.ErrNotFound
}
func (s *m71MailboxRepo) MarkRead(_ context.Context, id string, readAt time.Time) error {
	if m, ok := s.byID[id]; ok {
		m.Status = DeptMailStatusRead
		m.ReadAt = &readAt
		for i := range s.created {
			if s.created[i].ID == id {
				s.created[i].Status = DeptMailStatusRead
			}
		}
		return nil
	}
	return shared.ErrNotFound
}
func (s *m71MailboxRepo) MarkReplied(_ context.Context, id string) error {
	if m, ok := s.byID[id]; ok {
		m.Status = DeptMailStatusReplied
		return nil
	}
	return shared.ErrNotFound
}

type m71Waker struct {
	calls []string
	err   error
}

func (s *m71Waker) WakeDeptLead(_ context.Context, agentID, _ string) error {
	if s.err == nil {
		s.calls = append(s.calls, agentID)
	}
	return s.err
}

func newM71Mailbox(agents *m71AgentReader, org *m71OrgReader, repo *m71MailboxRepo, waker *m71Waker, auditor *m71Auditor) *DeptMailboxUsecase {
	return NewDeptMailboxUsecase(DeptMailboxUsecaseDeps{
		Repo: repo, Agents: agents, Org: org, Auditor: auditor, Waker: waker, Lg: loggateway.NewNoop(),
	})
}

func m71MailboxAgents() *m71AgentReader {
	leadA := m71LeadAgent("lead-a", m71DeptA)
	leadB := m71LeadAgent("lead-b", m71DeptB)
	plain := m71MemberAgent("plain", "plain_x", m71PosA1)
	return &m71AgentReader{
		byID:  map[string]Agent{leadA.ID: leadA, leadB.ID: leadB, plain.ID: plain},
		byKey: map[string]Agent{},
	}
}

func TestDeptMailbox_SendMessage(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	waker := &m71Waker{}
	auditor := &m71Auditor{}
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, waker, auditor)

	msg, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "协作请求", "请提供 Q3 数据", "")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if msg.Status != DeptMailStatusUnread || msg.ToAgentID != "lead-b" || msg.FromDeptID != m71DeptA {
		t.Fatalf("unexpected message: %+v", msg)
	}
	if len(waker.calls) != 1 || waker.calls[0] != "lead-b" {
		t.Fatalf("expected one wake for lead-b, got %+v", waker.calls)
	}
	if auditor.countByResult(ResultAllowed) == 0 {
		t.Fatal("expected allowed audit entry")
	}
}

func TestDeptMailbox_SubjectTruncationRuneSafe(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, &m71Waker{}, &m71Auditor{})

	// 100 个中文字符（300 字节）：按字符计不超限，必须完整保留。
	zh100 := strings.Repeat("协", 100)
	msg, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, zh100, "正文", "")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if msg.Subject != zh100 {
		t.Fatalf("100-rune subject must be kept intact, got %d runes", len([]rune(msg.Subject)))
	}

	// 250 字符（含中文）：截断到 200 字符，且结果是合法 UTF-8。
	long := strings.Repeat("协", 199) + strings.Repeat("x", 100)
	msg2, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, long, "正文", "")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if got := len([]rune(msg2.Subject)); got != 200 {
		t.Fatalf("expected 200 runes, got %d", got)
	}
	if !utf8.ValidString(msg2.Subject) {
		t.Fatal("truncated subject is not valid UTF-8")
	}
}

func TestTruncateForAudit(t *testing.T) {
	if got := truncateForAudit("  short  "); got != "short" {
		t.Fatalf("got %q", got)
	}
	long := strings.Repeat("检", 150)
	got := truncateForAudit(long)
	if len([]rune(got)) != 120 || !utf8.ValidString(got) {
		t.Fatalf("expected 120 valid runes, got %d", len([]rune(got)))
	}
}

func TestDeptMailbox_LeadOnSubPosition(t *testing.T) {
	// Regression: a dept lead attached to a sub-position under the department
	// node (not the department node itself) must resolve to the same department
	// as memberfs (shared resolveAgentDepartment) — previously requireDeptLead
	// only accepted the department node directly and denied such leads.
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	waker := &m71Waker{}
	leadSub := m71LeadAgent("lead-sub", m71PosA1) // position = pos-a1, parent = dept-a
	leadB := m71LeadAgent("lead-b", m71DeptB)
	agents := &m71AgentReader{
		byID:  map[string]Agent{leadSub.ID: leadSub, leadB.ID: leadB},
		byKey: map[string]Agent{},
	}
	uc := newM71Mailbox(agents, org, repo, waker, &m71Auditor{})

	msg, err := uc.SendMessage(context.Background(), "lead-sub", m71DeptB, "跨部门同步", "正文", "")
	if err != nil {
		t.Fatalf("lead on sub-position must be allowed, got %v", err)
	}
	if msg.FromDeptID != m71DeptA {
		t.Fatalf("expected from_dept resolved to %s, got %q", m71DeptA, msg.FromDeptID)
	}
}

func TestDeptMailbox_SendMessageDenials(t *testing.T) {
	org := m71OrgFixture()
	tests := []struct {
		name      string
		caller    string
		toDeptID  string
		subject   string
		refs      string
		wantAudit bool
	}{
		{name: "send to own department", caller: "lead-a", toDeptID: m71DeptA, subject: "s", wantAudit: true},
		{name: "target dept not found", caller: "lead-a", toDeptID: "dept-ghost", subject: "s", wantAudit: true},
		{name: "non-dept-lead sender", caller: "plain", toDeptID: m71DeptB, subject: "s"},
		{name: "empty subject", caller: "lead-a", toDeptID: m71DeptB, subject: "  "},
		{name: "invalid refs JSON", caller: "lead-a", toDeptID: m71DeptB, subject: "s", refs: "{not-array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newM71MailboxRepo()
			waker := &m71Waker{}
			auditor := &m71Auditor{}
			uc := newM71Mailbox(m71MailboxAgents(), org, repo, waker, auditor)
			if _, err := uc.SendMessage(context.Background(), tt.caller, tt.toDeptID, tt.subject, "body", tt.refs); err == nil {
				t.Fatal("expected denial")
			}
			if len(repo.created) != 0 {
				t.Fatal("message must not be persisted on denial")
			}
			if len(waker.calls) != 0 {
				t.Fatal("no wake expected on denial")
			}
			if tt.wantAudit && auditor.countByResult(ResultDenied) == 0 {
				t.Fatal("expected denied audit entry")
			}
		})
	}
}

func TestDeptMailbox_SendToDeptWithoutLead(t *testing.T) {
	org := m71OrgFixture()
	org.nodes["dept-c"] = OrganizationNode{ID: "dept-c", Key: "dept-c", Level: "department", DeptLeadAgentID: ""}
	uc := newM71Mailbox(m71MailboxAgents(), org, newM71MailboxRepo(), &m71Waker{}, &m71Auditor{})
	if _, err := uc.SendMessage(context.Background(), "lead-a", "dept-c", "s", "b", ""); err == nil {
		t.Fatal("expected denial for dept without lead")
	}
}

func TestDeptMailbox_WakeDebounce(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	waker := &m71Waker{}
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, waker, &m71Auditor{})

	// 同发送方 5min 窗口内第二次发送 → 不重复唤醒。
	if _, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s1", "b1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s2", "b2", ""); err != nil {
		t.Fatal(err)
	}
	if len(waker.calls) != 1 {
		t.Fatalf("expected debounced single wake, got %d", len(waker.calls))
	}

	// 跨发送方独立：lead-b 回复 lead-a → 新唤醒。
	if _, err := uc.SendMessage(context.Background(), "lead-b", m71DeptA, "s3", "b3", ""); err != nil {
		t.Fatal(err)
	}
	if len(waker.calls) != 2 || waker.calls[1] != "lead-a" {
		t.Fatalf("expected independent wake for lead-a, got %+v", waker.calls)
	}
}

func TestDeptMailbox_WakeFailureDoesNotBlock(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, &m71Waker{err: errors.New("turn busy")}, &m71Auditor{})
	msg, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s", "b", "")
	if err != nil {
		t.Fatalf("wake failure must not block send (NFR-05): %v", err)
	}
	if msg.ID == "" {
		t.Fatal("message must be persisted despite wake failure")
	}
}

func TestDeptMailbox_ReadMessage(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, &m71Waker{}, &m71Auditor{})

	sent, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	// 接收方读取：unread → read。
	got, err := uc.ReadMessage(context.Background(), "lead-b", sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != DeptMailStatusRead || got.ReadAt == nil {
		t.Fatalf("expected marked read, got %+v", got)
	}
	// 发送方读取自己的消息：不改状态。
	got2, err := uc.ReadMessage(context.Background(), "lead-a", sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Status != DeptMailStatusRead {
		t.Fatalf("sender re-read must keep status, got %s", got2.Status)
	}
}

func TestDeptMailbox_ReadMessageNonPartyDenied(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	auditor := &m71Auditor{}
	// 第三个部门主管作为无关方。
	leadC := m71LeadAgent("lead-c", "dept-c-pos")
	agents := m71MailboxAgents()
	agents.byID[leadC.ID] = leadC
	org.nodes["dept-c-pos"] = OrganizationNode{ID: "dept-c-pos", Level: "department"}
	uc := newM71Mailbox(agents, org, repo, &m71Waker{}, auditor)

	sent, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.ReadMessage(context.Background(), "lead-c", sent.ID); err == nil {
		t.Fatal("expected denial for non-party read")
	}
	if auditor.countByResult(ResultDenied) == 0 {
		t.Fatal("expected denied audit entry")
	}
}

func TestDeptMailbox_ReplyMessage(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	waker := &m71Waker{}
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, waker, &m71Auditor{})

	sent, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "数据请求", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := uc.ReplyMessage(context.Background(), "lead-b", sent.ID, "收到，今天发出")
	if err != nil {
		t.Fatal(err)
	}
	if reply.ReplyToID != sent.ID || reply.ToAgentID != "lead-a" || !strings.HasPrefix(reply.Subject, "Re: ") {
		t.Fatalf("unexpected reply: %+v", reply)
	}
	orig, err := repo.GetMessage(context.Background(), sent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if orig.Status != DeptMailStatusReplied {
		t.Fatalf("original must be marked replied, got %s", orig.Status)
	}
	// 回复触发对 lead-a 的唤醒（防抖键 lead-b→lead-a）。
	if len(waker.calls) != 2 || waker.calls[1] != "lead-a" {
		t.Fatalf("expected wake for lead-a, got %+v", waker.calls)
	}
}

func TestDeptMailbox_ReplyNonRecipientDenied(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, &m71Waker{}, &m71Auditor{})
	sent, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, "s", "b", "")
	if err != nil {
		t.Fatal(err)
	}
	// 发送方不能回复自己的消息（只有接收方可回复）。
	if _, err := uc.ReplyMessage(context.Background(), "lead-a", sent.ID, "x"); err == nil {
		t.Fatal("expected denial: only recipient may reply")
	}
}

func TestDeptMailbox_ListInbox(t *testing.T) {
	org := m71OrgFixture()
	repo := newM71MailboxRepo()
	uc := newM71Mailbox(m71MailboxAgents(), org, repo, &m71Waker{}, &m71Auditor{})
	for _, subj := range []string{"s1", "s2", "s3"} {
		if _, err := uc.SendMessage(context.Background(), "lead-a", m71DeptB, subj, "b", ""); err != nil {
			t.Fatal(err)
		}
	}
	items, err := uc.ListInbox(context.Background(), "lead-b", "", 0)
	if err != nil || len(items) != 3 {
		t.Fatalf("expected 3 inbox items, got %d err=%v", len(items), err)
	}
	items, err = uc.ListInbox(context.Background(), "lead-b", DeptMailStatusUnread, 2)
	if err != nil || len(items) != 2 {
		t.Fatalf("expected 2 unread items with limit, got %d err=%v", len(items), err)
	}
	if _, err := uc.ListInbox(context.Background(), "lead-b", "bogus", 0); err == nil {
		t.Fatal("expected error for invalid status filter")
	}
	if _, err := uc.ListInbox(context.Background(), "plain", "", 0); err == nil {
		t.Fatal("expected denial for non-dept-lead")
	}
}

// ---------------------------------------------------------------------------
// SessionSearchUsecase：spirit 校验 + 限流 + fail-closed（FR-08~FR-11）
// ---------------------------------------------------------------------------

type m71SessionReader struct {
	res       bizsession.SessionListResult
	err       error
	sess      bizsession.Session
	sessErr   error
	lastQuery bizsession.SessionSearchQuery
}

func (s *m71SessionReader) SearchSessions(_ context.Context, q bizsession.SessionSearchQuery) (bizsession.SessionListResult, error) {
	s.lastQuery = q
	return s.res, s.err
}
func (s *m71SessionReader) GetSessionByID(context.Context, string) (bizsession.Session, error) {
	if s.sessErr != nil {
		return bizsession.Session{}, s.sessErr
	}
	return s.sess, nil
}
func (s *m71SessionReader) GetSessionRevision(context.Context, string) (int64, error) { return 0, nil }
func (s *m71SessionReader) ListSessionsForBatch(context.Context, bizsession.SessionSearchQuery) ([]bizsession.Session, error) {
	return nil, nil
}
func (s *m71SessionReader) ListSessionsByIDs(context.Context, []string) ([]bizsession.Session, error) {
	return nil, nil
}
func (s *m71SessionReader) ListActiveAgentUserKeys(context.Context, int) ([]bizsession.AgentUserKey, error) {
	return nil, nil
}

type m71MessageReader struct {
	msgs []bizsession.ChatMessage
}

func (s *m71MessageReader) CountMessagesBySession(context.Context, string) (int, error) {
	return len(s.msgs), nil
}
func (s *m71MessageReader) ListMessagesBySession(context.Context, string, int, int) ([]bizsession.ChatMessage, error) {
	return s.msgs, nil
}
func (s *m71MessageReader) ListMessagesAfterTurn(context.Context, string, int) ([]bizsession.ChatMessage, error) {
	return nil, nil
}
func (s *m71MessageReader) ListMessagesRecent(context.Context, string, int) ([]bizsession.ChatMessage, error) {
	return nil, nil
}
func (s *m71MessageReader) ListMessagesByIDs(context.Context, string, []string) ([]bizsession.ChatMessage, error) {
	return nil, nil
}

type m71Searcher struct {
	hits            []GlobalMessageHit
	err             error
	lastWorkspaceID string
}

func (s *m71Searcher) SearchGlobalMessages(_ context.Context, _, _, workspaceID string, _ int) ([]GlobalMessageHit, error) {
	s.lastWorkspaceID = workspaceID
	return s.hits, s.err
}

func newM71SessionSearch(callerKey string, auditor *m71Auditor, msgs []bizsession.ChatMessage) *SessionSearchUsecase {
	spirit := Agent{ID: "spirit-1", AgentKey: callerKey}
	return NewSessionSearchUsecase(SessionSearchUsecaseDeps{
		Agents:   &m71AgentReader{byID: map[string]Agent{spirit.ID: spirit}, byKey: map[string]Agent{}},
		Sessions: &m71SessionReader{},
		Messages: &m71MessageReader{msgs: msgs},
		Searcher: &m71Searcher{hits: []GlobalMessageHit{{ID: "h1", SessionID: "s1", Kind: "reply", Snippet: "x"}}},
		Auditor:  auditor,
		Lg:       loggateway.NewNoop(),
	})
}

func TestSessionSearch_SpiritOnly(t *testing.T) {
	auditor := &m71Auditor{}
	uc := newM71SessionSearch(SpiritAgentKey, auditor, nil)
	hits, err := uc.SearchMessages(context.Background(), "spirit-1", "keyword", "", 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("expected allow for spirit, got hits=%v err=%v", hits, err)
	}
	if auditor.last().Result != ResultAllowed || auditor.last().ActorRole != RoleSpirit {
		t.Fatalf("unexpected audit: %+v", auditor.last())
	}

	uc2 := newM71SessionSearch("worker_a", &m71Auditor{}, nil)
	if _, err := uc2.SearchMessages(context.Background(), "spirit-1", "keyword", "", 0); err == nil {
		t.Fatal("expected denial for non-spirit caller")
	}
}

func TestSessionSearch_RateLimit(t *testing.T) {
	auditor := &m71Auditor{}
	uc := newM71SessionSearch(SpiritAgentKey, auditor, nil)
	for i := 0; i < sessionSearchRateLimit; i++ {
		if _, err := uc.SearchMessages(context.Background(), "spirit-1", "k", "", 0); err != nil {
			t.Fatalf("request %d should be allowed: %v", i+1, err)
		}
	}
	// 第 21 次超限。
	if _, err := uc.SearchMessages(context.Background(), "spirit-1", "k", "", 0); err == nil {
		t.Fatal("expected rate limit denial on 21st request")
	}
	if auditor.countByResult(ResultDenied) != 1 {
		t.Fatalf("expected exactly 1 denied audit, got %d", auditor.countByResult(ResultDenied))
	}
	// 窗口重置后恢复。
	uc.buckets["spirit-1"].windowStart = time.Now().Add(-2 * sessionSearchRateWindow)
	if _, err := uc.SearchMessages(context.Background(), "spirit-1", "k", "", 0); err != nil {
		t.Fatalf("expected allow after window reset: %v", err)
	}
}

func TestSessionSearch_FailClosedOnAuditError(t *testing.T) {
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{err: errors.New("db down")}, nil)
	if _, err := uc.SearchMessages(context.Background(), "spirit-1", "k", "", 0); err == nil {
		t.Fatal("expected denial when audit write fails (fail-closed)")
	}
}

func TestSessionSearch_EmptyKeyword(t *testing.T) {
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, nil)
	if _, err := uc.SearchMessages(context.Background(), "spirit-1", "  ", "", 0); err == nil {
		t.Fatal("expected error for empty keyword")
	}
}

func TestSessionSearch_ListAgentSessions(t *testing.T) {
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, nil)
	uc.sessions = &m71SessionReader{res: bizsession.SessionListResult{
		Items: []bizsession.Session{{ID: "s1", Title: "t", AgentID: "a1", MessageCount: 3, Status: "idle", UpdatedAt: "2026-07-01T00:00:00Z"}},
	}}
	metas, err := uc.ListAgentSessions(context.Background(), "spirit-1", "", 0)
	if err != nil || len(metas) != 1 || metas[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v err=%v", metas, err)
	}
}

func TestSessionSearch_ReadSessionHistory(t *testing.T) {
	msgs := []bizsession.ChatMessage{
		{ID: "m1", Role: "user", ContentMarkdown: "first", CreatedAt: "2026-07-01T00:00:01Z"},
		{ID: "m2", Role: "assistant", ContentMarkdown: "second", CreatedAt: "2026-07-01T00:00:02Z"},
		{ID: "m3", Role: "user", ContentMarkdown: "third", CreatedAt: "2026-07-01T00:00:03Z"},
	}
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, msgs)

	// limit 取最近 N 条。
	out, truncated, err := uc.ReadSessionHistory(context.Background(), "spirit-1", "s1", "", 2, 0)
	if err != nil || len(out) != 2 || out[0].ID != "m2" || truncated {
		t.Fatalf("unexpected page: %+v truncated=%v err=%v", out, truncated, err)
	}
	// before_message_id 向前翻页。
	out, _, err = uc.ReadSessionHistory(context.Background(), "spirit-1", "s1", "m3", 10, 0)
	if err != nil || len(out) != 2 || out[1].ID != "m2" {
		t.Fatalf("unexpected before-page: %+v err=%v", out, err)
	}
	// maxChars 截断：预算严格填满，"first"(5) + "se"(2) = 7。
	out, truncated, err = uc.ReadSessionHistory(context.Background(), "spirit-1", "s1", "", 10, 7)
	if err != nil || !truncated || len(out) != 2 || out[0].Content != "first" || out[1].Content != "se" {
		t.Fatalf("unexpected truncation: %+v truncated=%v err=%v", out, truncated, err)
	}
	// 空 session_id。
	if _, _, err := uc.ReadSessionHistory(context.Background(), "spirit-1", " ", "", 0, 0); err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

// P2-C workspace 隔离：检索/读取必须限定在 caller workspace 内。
func TestSessionSearch_ListAgentSessions_WorkspaceScoped(t *testing.T) {
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, nil)
	reader := &m71SessionReader{}
	uc.sessions = reader

	ctx := workspace.WithContext(context.Background(), "ws-a")
	if _, err := uc.ListAgentSessions(ctx, "spirit-1", "", 0); err != nil {
		t.Fatal(err)
	}
	if reader.lastQuery.WorkspaceID != "ws-a" {
		t.Fatalf("expected workspace filter ws-a, got %q", reader.lastQuery.WorkspaceID)
	}
	// 无 workspace 上下文 → default。
	if _, err := uc.ListAgentSessions(context.Background(), "spirit-1", "", 0); err != nil {
		t.Fatal(err)
	}
	if reader.lastQuery.WorkspaceID != workspace.DefaultWorkspaceID {
		t.Fatalf("expected default workspace filter, got %q", reader.lastQuery.WorkspaceID)
	}
}

func TestSessionSearch_SearchMessages_WorkspaceScoped(t *testing.T) {
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, nil)
	searcher := &m71Searcher{}
	uc.searcher = searcher

	ctx := workspace.WithContext(context.Background(), "ws-b")
	if _, err := uc.SearchMessages(ctx, "spirit-1", "k", "", 0); err != nil {
		t.Fatal(err)
	}
	if searcher.lastWorkspaceID != "ws-b" {
		t.Fatalf("expected searcher workspace ws-b, got %q", searcher.lastWorkspaceID)
	}
	if _, err := uc.SearchMessages(context.Background(), "spirit-1", "k", "", 0); err != nil {
		t.Fatal(err)
	}
	if searcher.lastWorkspaceID != workspace.DefaultWorkspaceID {
		t.Fatalf("expected searcher workspace default, got %q", searcher.lastWorkspaceID)
	}
}

func TestSessionSearch_ReadSessionHistory_WorkspaceGuard(t *testing.T) {
	msgs := []bizsession.ChatMessage{{ID: "m1", Role: "user", ContentMarkdown: "x"}}
	uc := newM71SessionSearch(SpiritAgentKey, &m71Auditor{}, msgs)
	reader := &m71SessionReader{sess: bizsession.Session{ID: "s1", WorkspaceID: "ws-a"}}
	uc.sessions = reader

	// 同 workspace → 允许。
	ctx := workspace.WithContext(context.Background(), "ws-a")
	if _, _, err := uc.ReadSessionHistory(ctx, "spirit-1", "s1", "", 0, 0); err != nil {
		t.Fatalf("expected allow in same workspace: %v", err)
	}
	// 跨 workspace → NotFound（不泄露存在性）。
	ctxOther := workspace.WithContext(context.Background(), "ws-b")
	if _, _, err := uc.ReadSessionHistory(ctxOther, "spirit-1", "s1", "", 0, 0); err == nil {
		t.Fatal("expected denial for cross-workspace read")
	}
	// 会话不存在 → NotFound。
	reader.sessErr = shared.ErrNotFound
	if _, _, err := uc.ReadSessionHistory(ctx, "spirit-1", "s1", "", 0, 0); err == nil {
		t.Fatal("expected not found for missing session")
	}
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func TestIsDeptLeadAgent(t *testing.T) {
	cases := []struct {
		name  string
		agent Agent
		want  bool
	}{
		{"variant match", Agent{AgentVariant: "dept_lead"}, true},
		{"key prefix+suffix", Agent{AgentKey: "__dept_lead_sales__"}, true},
		{"prefix only no suffix", Agent{AgentKey: "__dept_lead_sales"}, false},
		{"regular agent", Agent{AgentKey: "worker_a"}, false},
		{"company lead is not dept", Agent{AgentVariant: AgentVariantCompanyLead}, false},
		{"empty", Agent{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsDeptLeadAgent(c.agent); got != c.want {
				t.Fatalf("IsDeptLeadAgent = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNormalizeRefsJSON(t *testing.T) {
	if got, err := normalizeRefsJSON(""); err != nil || got != "[]" {
		t.Fatalf("empty refs: got=%q err=%v", got, err)
	}
	if _, err := normalizeRefsJSON(`{"a":1}`); err == nil {
		t.Fatal("expected error for non-array refs")
	}
	if got, err := normalizeRefsJSON(`[{"team_id":"t1"}]`); err != nil || got != `[{"team_id":"t1"}]` {
		t.Fatalf("valid refs: got=%q err=%v", got, err)
	}
}
