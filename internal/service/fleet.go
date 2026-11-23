package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/fleet/v1"
	"aranea-agents/internal/biz/fleet"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ============================================================================
// FleetService 实现 fleet.v1：Harness 多任务运行台（复刻 stablyai/orca）。
//
// 权限（双保险）：middleware 按登记表硬判 coding.manage（make api 生成）；
// service 每个 RPC 再复核一次，防登记表被旁路。终端字节流与 fleetctl 命令
// 不走本服务，见 server/fleet_ws.go。
// ============================================================================

// FleetService 是 fleet.v1 服务实现。
type FleetService struct {
	v1.UnimplementedFleetServiceServer
	uc *fleet.Usecase
}

// NewFleetService 装配运行台服务。
func NewFleetService(uc *fleet.Usecase) *FleetService {
	return &FleetService{uc: uc}
}

// Usecase 暴露给 server 层的终端 WebSocket 与 fleetctl 入口。
func (s *FleetService) Usecase() *fleet.Usecase {
	if s == nil {
		return nil
	}
	return s.uc
}

func (s *FleetService) guard(ctx context.Context) error {
	if s.uc == nil {
		return apierror.Unavailable("FLEET", "fleet not wired")
	}
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return apierror.Unauthorized("FLEET", "authentication required")
	}
	if !a.HasPermission("coding.manage") {
		return apierror.Forbidden("FLEET", "coding.manage permission required")
	}
	return nil
}

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ---------- agent 与项目 ----------

func (s *FleetService) ListAgentPresets(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentPresetsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	out := &v1.ListAgentPresetsResponse{CliRoot: fleet.CLIRoot()}
	for _, p := range s.uc.ListPresets() {
		out.Items = append(out.Items, s.presetPB(ctx, p))
	}
	return out, nil
}

func (s *FleetService) presetPB(ctx context.Context, p fleet.AgentPreset) *v1.AgentPreset {
	state, log := fleet.InstallStatus(p.ID)
	tState, tLog, tRes := fleet.TestStatus(p.ID)
	set := s.uc.GetAgentSetting(ctx, p.ID)
	return &v1.AgentPreset{
		Id: p.ID, Label: p.Label, Command: p.Command, Available: p.Available,
		SupportsFullAccess: p.FullAccessFlag != "", FullAccessFlag: p.FullAccessFlag, ResolvedPath: p.ResolvedPath,
		Install: p.Install, InstallState: state, InstallLog: log,
		AutoEditFlag: p.AutoEditFlag, ApproveKey: p.ApproveKey, RejectKey: p.RejectKey, LoginHint: p.LoginHint,
		Readiness: fleet.AgentReadiness(p.ID), Mode: set.Mode, Args: set.Args,
		AutoUpdate: set.AutoUpdate, Version: set.Version, ToolUpdatedAt: set.UpdatedAt, SupportsTest: len(p.TestArgs) > 0,
		TestState: tState, TestLog: tLog, TestVerdict: tRes.Verdict, TestReply: tRes.Reply, TestMs: tRes.DurationMS,
		HasLogin: len(p.LoginArgs) > 0, EmailLogin: p.EmailFlag != "", KeyEnv: p.KeyEnv, KeyUrl: p.KeyURL,
		KeyHint: set.KeyHint, Account: set.Account, KeyOnly: p.KeyOnly, Model: set.Model, SupportsModel: p.ModelFlag != "",
		Effort: set.Effort, SupportsEffort: len(p.EffortArgs) > 0, Efforts: p.Efforts,
		LatestVersion: set.LatestVersion, UpdateAvailable: fleet.NewerVersion(set.Version, set.LatestVersion), LatestCheckedAt: set.LatestCheckedAt,
		AcpArgs: p.ACPArgs, KeyName: p.KeyName, HostEnv: p.HostEnv, Host: set.Host,
	}
}

func (s *FleetService) CheckToolUpdate(ctx context.Context, req *v1.InstallAgentRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.CheckToolUpdate(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return s.presetByID(ctx, req.GetId()), nil
}

func (s *FleetService) ListToolModels(ctx context.Context, req *v1.InstallAgentRequest) (*v1.ListToolModelsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	models, source, err := s.uc.ListToolModels(req.GetId())
	if err != nil && len(models) == 0 {
		return nil, err
	}
	out := &v1.ListToolModelsResponse{Source: source}
	for _, m := range models {
		out.Models = append(out.Models, &v1.ToolModel{
			Id: m.ID, Name: m.Name, Description: m.Desc, Efforts: m.Efforts, DefaultEffort: m.DefaultEffort, IsDefault: m.Default,
		})
	}
	if err != nil {
		out.Error = err.Error()
	}
	return out, nil
}

func (s *FleetService) SaveToolKey(ctx context.Context, req *v1.SaveToolKeyRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.SaveToolKey(ctx, req.GetId(), req.GetKey(), req.GetHost()); err != nil {
		return nil, err
	}
	return s.presetByID(ctx, req.GetId()), nil
}

func (s *FleetService) DeleteToolKey(ctx context.Context, req *v1.InstallAgentRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteToolKey(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return s.presetByID(ctx, req.GetId()), nil
}

func (s *FleetService) StartToolLogin(ctx context.Context, req *v1.StartToolLoginRequest) (*v1.ToolLoginState, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if _, err := s.uc.StartToolLogin(req.GetId(), req.GetEmail()); err != nil {
		return nil, err
	}
	return loginStatePB(s.uc.ToolLoginState(req.GetId())), nil
}

func (s *FleetService) GetToolLogin(ctx context.Context, req *v1.InstallAgentRequest) (*v1.ToolLoginState, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return loginStatePB(s.uc.ToolLoginState(req.GetId())), nil
}

func loginStatePB(st fleet.LoginState) *v1.ToolLoginState {
	return &v1.ToolLoginState{
		TerminalId: st.TerminalID, Running: st.Running, Done: st.Done, Ok: st.OK,
		Url: st.URL, Code: st.Code, Qr: st.QR, Account: st.Account, Message: st.Message,
	}
}

func (s *FleetService) presetByID(ctx context.Context, id string) *v1.AgentPreset {
	for _, p := range s.uc.ListPresets() {
		if p.ID == id {
			return s.presetPB(ctx, p)
		}
	}
	return &v1.AgentPreset{Id: id}
}

func (s *FleetService) TestAgent(ctx context.Context, req *v1.InstallAgentRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.TestAgent(req.GetId()); err != nil {
		return nil, err
	}
	return s.presetByID(ctx, req.GetId()), nil
}

func (s *FleetService) InstallAgent(ctx context.Context, req *v1.InstallAgentRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.InstallAgent(req.GetId()); err != nil {
		return nil, err
	}
	for _, p := range s.uc.ListPresets() {
		if p.ID == req.GetId() {
			return s.presetPB(ctx, p), nil
		}
	}
	return &v1.AgentPreset{Id: req.GetId()}, nil
}

func (s *FleetService) UpdateAgentSetting(ctx context.Context, req *v1.UpdateAgentSettingRequest) (*v1.AgentPreset, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if _, err := s.uc.SetAgentSetting(ctx, fleet.AgentSetting{ID: req.GetId(), Mode: req.GetMode(), Args: req.GetArgs(), AutoUpdate: req.GetAutoUpdate(), Model: req.GetModel(), Effort: req.GetEffort()}); err != nil {
		return nil, err
	}
	p, _ := fleet.FindPreset(req.GetId())
	return s.presetPB(ctx, p), nil
}

func projectPB(p fleet.Project) *v1.Project {
	return &v1.Project{Id: p.ID, Name: p.Name, Path: p.Path, DefaultBranch: p.DefaultBranch, IsGit: p.IsGit}
}

func (s *FleetService) ListProjects(ctx context.Context, _ *emptypb.Empty) (*v1.ListProjectsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListProjectsResponse{}
	for _, p := range items {
		out.Items = append(out.Items, projectPB(p))
	}
	return out, nil
}

func (s *FleetService) AddProject(ctx context.Context, req *v1.AddProjectRequest) (*v1.Project, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	p, err := s.uc.AddProject(ctx, req.Path, req.Name)
	if err != nil {
		return nil, err
	}
	return projectPB(p), nil
}

func (s *FleetService) ListRefs(ctx context.Context, req *v1.ListRefsRequest) (*v1.ListRefsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	local, remote, def, err := s.uc.ListRefs(ctx, req.ProjectId)
	if err != nil {
		return nil, err
	}
	return &v1.ListRefsResponse{Local: local, Remote: remote, DefaultRef: def}, nil
}

// ---------- worktree ----------

func workspacePB(w fleet.Workspace) *v1.Workspace {
	return &v1.Workspace{
		Id: w.ID, ProjectId: w.ProjectID, Name: w.Name, Branch: w.Branch, BaseRef: w.BaseRef, Path: w.Path,
		Kind: w.Kind, Status: w.Status, Error: w.Error, Pinned: w.Pinned, Archived: w.Archived, ParentId: w.ParentID,
		Origin: w.Origin, LinkedIssue: w.LinkedIssue, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, LastActiveAt: w.LastActiveAt,
	}
}

func (s *FleetService) ListWorkspaces(ctx context.Context, req *v1.ListWorkspacesRequest) (*v1.ListWorkspacesResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListWorkspaces(ctx, req.ProjectId, req.IncludeArchived)
	if err != nil {
		return nil, err
	}
	out := &v1.ListWorkspacesResponse{}
	for _, w := range items {
		out.Items = append(out.Items, workspacePB(w))
	}
	return out, nil
}

func (s *FleetService) CreateWorkspace(ctx context.Context, req *v1.CreateWorkspaceRequest) (*v1.Workspace, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	w, err := s.uc.CreateWorkspace(ctx, fleet.CreateWorkspaceInput{
		ProjectID: req.ProjectId, Name: req.Name, Branch: req.Branch, StartFrom: req.StartFrom, Agent: req.Agent,
		Prompt: req.Prompt, FullAccess: req.FullAccess, ParentID: req.ParentId, Fetch: req.Fetch,
	})
	if err != nil {
		return nil, err
	}
	return workspacePB(w), nil
}

func (s *FleetService) UpdateWorkspace(ctx context.Context, req *v1.UpdateWorkspaceRequest) (*v1.Workspace, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	w, err := s.uc.UpdateWorkspace(ctx, req.Id, fleet.WorkspacePatch{Name: req.Name, Pinned: req.Pinned, Archived: req.Archived, LinkedIssue: req.LinkedIssue})
	if err != nil {
		return nil, err
	}
	return workspacePB(w), nil
}

func (s *FleetService) DeleteWorkspace(ctx context.Context, req *v1.DeleteWorkspaceRequest) (*v1.DeleteWorkspaceResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	kept, msg, err := s.uc.DeleteWorkspace(ctx, req.Id, req.ForceBranch)
	if err != nil {
		return nil, err
	}
	return &v1.DeleteWorkspaceResponse{BranchKept: kept, Message: msg}, nil
}

// ---------- 终端 ----------

func terminalPB(t fleet.TerminalInfo) *v1.Terminal {
	return &v1.Terminal{
		Id: t.ID, WorkspaceId: t.WorkspaceID, Agent: t.Agent, Title: t.Title, Status: t.Status, FullAccess: t.FullAccess, Mode: t.Mode,
		Pid: int32(t.PID), StartedAt: ts(t.StartedAt), ExitCode: int32(t.ExitCode), LastActivityAt: ts(t.LastActivityAt),
		Preview: t.Preview, RunId: t.RunID, TaskId: t.TaskID, Role: t.Role, StatusChangedAt: ts(t.StatusChangedAt),
	}
}

func (s *FleetService) ListTerminals(ctx context.Context, req *v1.ListTerminalsRequest) (*v1.ListTerminalsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	out := &v1.ListTerminalsResponse{}
	for _, t := range s.uc.ListTerminals(req.WorkspaceId) {
		out.Items = append(out.Items, terminalPB(t))
	}
	return out, nil
}

func (s *FleetService) CreateTerminal(ctx context.Context, req *v1.CreateTerminalRequest) (*v1.Terminal, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	t, err := s.uc.CreateTerminal(ctx, fleet.CreateTerminalInput{WorkspaceID: req.WorkspaceId, Agent: req.Agent, Prompt: req.Prompt, FullAccess: req.FullAccess, Title: req.Title})
	if err != nil {
		return nil, err
	}
	return terminalPB(t), nil
}

func (s *FleetService) KillTerminal(ctx context.Context, req *v1.TerminalIdRequest) (*v1.Terminal, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	t, err := s.uc.KillTerminal(req.Id)
	if err != nil {
		return nil, err
	}
	return terminalPB(t), nil
}

func (s *FleetService) RestartTerminal(ctx context.Context, req *v1.TerminalIdRequest) (*v1.Terminal, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	t, err := s.uc.RestartTerminal(req.Id)
	if err != nil {
		return nil, err
	}
	return terminalPB(t), nil
}

func (s *FleetService) CloseTerminal(ctx context.Context, req *v1.TerminalIdRequest) (*emptypb.Empty, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.CloseTerminal(req.Id)
}

func (s *FleetService) SendInput(ctx context.Context, req *v1.SendInputRequest) (*emptypb.Empty, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.SendInput(req.Id, req.Text, req.Submit, req.Raw)
}

// ---------- diff 与批注 ----------

func (s *FleetService) GetDiff(ctx context.Context, req *v1.DiffRequest) (*v1.DiffResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	d, err := s.uc.Diff(ctx, req.WorkspaceId, req.BaseRef)
	if err != nil {
		return nil, err
	}
	out := &v1.DiffResponse{BaseRef: d.BaseRef, MergeBase: d.MergeBase, Additions: int32(d.Additions), Deletions: int32(d.Deletions)}
	for _, f := range d.Files {
		pf := &v1.DiffFile{Path: f.Path, OldPath: f.OldPath, Status: f.Status, Additions: int32(f.Additions), Deletions: int32(f.Deletions), Binary: f.Binary, Truncated: f.Truncated}
		for _, h := range f.Hunks {
			ph := &v1.DiffHunk{Header: h.Header}
			for _, l := range h.Lines {
				ph.Lines = append(ph.Lines, &v1.DiffLine{Kind: l.Kind, OldNo: int32(l.OldNo), NewNo: int32(l.NewNo), Text: l.Text})
			}
			pf.Hunks = append(pf.Hunks, ph)
		}
		out.Files = append(out.Files, pf)
	}
	return out, nil
}

func notePB(n fleet.Note) *v1.Note {
	return &v1.Note{Id: n.ID, WorkspaceId: n.WorkspaceID, Path: n.Path, Line: int32(n.Line), Side: n.Side, Body: n.Body,
		Resolved: n.Resolved, SentAt: n.SentAt, CreatedAt: n.CreatedAt, LineText: n.LineText}
}

func (s *FleetService) ListNotes(ctx context.Context, req *v1.ListNotesRequest) (*v1.ListNotesResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListNotes(ctx, req.WorkspaceId)
	if err != nil {
		return nil, err
	}
	out := &v1.ListNotesResponse{}
	for _, n := range items {
		out.Items = append(out.Items, notePB(n))
	}
	return out, nil
}

func (s *FleetService) UpsertNote(ctx context.Context, req *v1.UpsertNoteRequest) (*v1.Note, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if req.Note == nil {
		return nil, apierror.BadRequest("FLEET", "note is required")
	}
	n := req.Note
	saved, err := s.uc.UpsertNote(ctx, fleet.Note{ID: n.Id, WorkspaceID: n.WorkspaceId, Path: n.Path, Line: int(n.Line), Side: n.Side,
		Body: n.Body, Resolved: n.Resolved, LineText: n.LineText})
	if err != nil {
		return nil, err
	}
	return notePB(saved), nil
}

func (s *FleetService) DeleteNote(ctx context.Context, req *v1.NoteIdRequest) (*emptypb.Empty, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteNote(ctx, req.Id)
}

func (s *FleetService) SendNotes(ctx context.Context, req *v1.SendNotesRequest) (*v1.SendNotesResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	id, n, err := s.uc.SendNotes(ctx, req.WorkspaceId, req.TerminalId, req.Agent, req.FullAccess)
	if err != nil {
		return nil, err
	}
	return &v1.SendNotesResponse{TerminalId: id, Sent: int32(n)}, nil
}

// ---------- 源码管理 ----------

func (s *FleetService) GetGitStatus(ctx context.Context, req *v1.WorkspaceIdRequest) (*v1.GitStatus, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	st, err := s.uc.GitStatus(ctx, req.WorkspaceId)
	if err != nil {
		return nil, err
	}
	out := &v1.GitStatus{Branch: st.Branch, Upstream: st.Upstream, Ahead: int32(st.Ahead), Behind: int32(st.Behind), Head: st.Head,
		BaseRef: st.BaseRef, Additions: int32(st.Additions), Deletions: int32(st.Deletions)}
	for _, f := range st.Files {
		out.Files = append(out.Files, &v1.GitFile{Path: f.Path, Index: f.Index, Worktree: f.Worktree, Staged: f.Staged, Untracked: f.Untracked})
	}
	return out, nil
}

func gitOp(out string, err error) (*v1.GitOpResponse, error) {
	if err != nil && out == "" {
		out = err.Error()
	}
	return &v1.GitOpResponse{Ok: err == nil, Output: strings.TrimSpace(out)}, nil
}

func (s *FleetService) StageFiles(ctx context.Context, req *v1.GitPathsRequest) (*v1.GitOpResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return gitOp(s.uc.Stage(ctx, req.WorkspaceId, req.Paths))
}

func (s *FleetService) UnstageFiles(ctx context.Context, req *v1.GitPathsRequest) (*v1.GitOpResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return gitOp(s.uc.Unstage(ctx, req.WorkspaceId, req.Paths))
}

func (s *FleetService) Commit(ctx context.Context, req *v1.CommitRequest) (*v1.GitOpResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	out, err := s.uc.Commit(ctx, req.WorkspaceId, req.Message, req.Amend)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeBadRequest {
		return nil, err
	}
	return gitOp(out, err)
}

func (s *FleetService) Push(ctx context.Context, req *v1.WorkspaceIdRequest) (*v1.GitOpResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return gitOp(s.uc.Push(ctx, req.WorkspaceId))
}

// ---------- 动态与用量 ----------

func (s *FleetService) ListEvents(ctx context.Context, req *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	limit := int(req.Limit)
	if limit <= 0 || limit > 400 {
		limit = 100
	}
	items, unread, err := s.uc.ListEvents(ctx, limit, req.UnreadOnly)
	if err != nil {
		return nil, err
	}
	out := &v1.ListEventsResponse{Unread: int32(unread)}
	for _, e := range items {
		out.Items = append(out.Items, &v1.Event{Id: e.ID, Kind: e.Kind, Title: e.Title, Body: e.Body, WorkspaceId: e.WorkspaceID,
			TerminalId: e.TerminalID, Read: e.Read, CreatedAt: e.CreatedAt, RunId: e.RunID})
	}
	return out, nil
}

func (s *FleetService) MarkEventsRead(ctx context.Context, req *v1.MarkEventsReadRequest) (*emptypb.Empty, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.MarkEventsRead(ctx, req.Ids, req.All)
}

func (s *FleetService) GetUsage(ctx context.Context, _ *emptypb.Empty) (*v1.GetUsageResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	out := &v1.GetUsageResponse{}
	for _, a := range s.uc.Usage() {
		pa := &v1.AgentUsage{Agent: a.Agent, Source: a.Source, UpdatedAt: a.UpdatedAt, Sessions: a.Sessions}
		for _, w := range a.Windows {
			pa.Windows = append(pa.Windows, &v1.UsageWindow{Label: w.Label, Tokens: w.Tokens, Percent: w.Percent, ResetsAt: w.ResetsAt})
		}
		out.Items = append(out.Items, pa)
	}
	return out, nil
}

func creditPB(c fleet.PlatformCredit) *v1.PlatformCredit {
	out := &v1.PlatformCredit{Platform: c.Platform, Source: c.Source, Plan: c.Plan, Total: c.Total, Remaining: c.Remaining,
		Unit: c.Unit, Unlimited: c.Unlimited, UpdatedAt: c.UpdatedAt, Note: c.Note}
	for _, p := range c.History {
		out.History = append(out.History, &v1.CreditPoint{Date: p.Date, Remaining: p.Remaining})
	}
	return out
}

func (s *FleetService) ListCredits(ctx context.Context, _ *emptypb.Empty) (*v1.ListCreditsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListCredits(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListCreditsResponse{}
	for _, c := range items {
		out.Items = append(out.Items, creditPB(c))
	}
	return out, nil
}

func (s *FleetService) SetCredit(ctx context.Context, req *v1.SetCreditRequest) (*v1.PlatformCredit, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	c, err := s.uc.SetCredit(ctx, fleet.PlatformCredit{Platform: req.Platform, Plan: req.Plan, Total: req.Total,
		Remaining: req.Remaining, Unit: req.Unit, Note: req.Note})
	if err != nil {
		return nil, err
	}
	return creditPB(c), nil
}

// ---------- 编排 ----------

func runPB(r fleet.Run) *v1.Run {
	return &v1.Run{Id: r.ID, Objective: r.Objective, ProjectId: r.ProjectID, WorkspaceId: r.WorkspaceID,
		CoordinatorTerminalId: r.CoordinatorTerminalID, Agent: r.Agent, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (s *FleetService) CreateRun(ctx context.Context, req *v1.CreateRunRequest) (*v1.Run, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	r, err := s.uc.CreateRun(ctx, fleet.CreateRunInput{ProjectID: req.ProjectId, WorkspaceID: req.WorkspaceId, Objective: req.Objective, Agent: req.Agent, FullAccess: req.FullAccess})
	if err != nil {
		return nil, err
	}
	return runPB(r), nil
}

func (s *FleetService) ListRuns(ctx context.Context, _ *emptypb.Empty) (*v1.ListRunsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListRunsResponse{}
	for _, r := range items {
		pr := runPB(r.Run)
		pr.TaskCount, pr.DoneCount, pr.Unread = int32(r.TaskCount), int32(r.DoneCount), int32(r.Unread)
		out.Items = append(out.Items, pr)
	}
	return out, nil
}

func (s *FleetService) GetRun(ctx context.Context, req *v1.RunIdRequest) (*v1.GetRunResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	d, err := s.uc.GetRun(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out := &v1.GetRunResponse{Run: runPB(d.Run)}
	for _, t := range d.Tasks {
		out.Tasks = append(out.Tasks, &v1.Task{Id: t.ID, RunId: t.RunID, Title: t.Title, Spec: t.Spec, Status: t.Status, Deps: t.Deps,
			WorkerTerminalId: t.WorkerTerminalID, WorkerWorkspaceId: t.WorkerWorkspaceID, WorkerAgent: t.WorkerAgent, DispatchId: t.DispatchID,
			Outcome: t.Outcome, Summary: t.Summary, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, FilesModified: t.FilesModified})
	}
	for _, m := range d.Messages {
		out.Messages = append(out.Messages, messagePB(m))
	}
	return out, nil
}

func messagePB(m fleet.Message) *v1.Message {
	return &v1.Message{Id: m.ID, RunId: m.RunID, From: m.From, To: m.To, Type: m.Type, Subject: m.Subject, Body: m.Body,
		TaskId: m.TaskID, DispatchId: m.DispatchID, Read: m.Read, Answer: m.Answer, CreatedAt: m.CreatedAt, Options: m.Options}
}

func (s *FleetService) StopRun(ctx context.Context, req *v1.RunIdRequest) (*v1.Run, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	r, err := s.uc.StopRun(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return runPB(r), nil
}

func (s *FleetService) AnswerQuestion(ctx context.Context, req *v1.AnswerRequest) (*v1.Message, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	m, err := s.uc.AnswerQuestion(ctx, req.MessageId, req.Answer)
	if err != nil {
		return nil, err
	}
	return messagePB(m), nil
}

// ---------- 自动化 ----------

func automationPB(a fleet.Automation) *v1.Automation {
	return &v1.Automation{Id: a.ID, Name: a.Name, ProjectId: a.ProjectID, Agent: a.Agent, Prompt: a.Prompt, Schedule: a.Schedule,
		WorktreeMode: a.WorktreeMode, FullAccess: a.FullAccess, Enabled: a.Enabled, LastRunAt: a.LastRunAt, NextRunAt: a.NextRunAt,
		LastStatus: a.LastStatus, LastWorkspaceId: a.LastWorkspaceID, CreatedAt: a.CreatedAt}
}

func (s *FleetService) ListAutomations(ctx context.Context, _ *emptypb.Empty) (*v1.ListAutomationsResponse, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	items, err := s.uc.ListAutomations(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAutomationsResponse{}
	for _, a := range items {
		out.Items = append(out.Items, automationPB(a))
	}
	return out, nil
}

func (s *FleetService) UpsertAutomation(ctx context.Context, req *v1.UpsertAutomationRequest) (*v1.Automation, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	if req.Automation == nil {
		return nil, apierror.BadRequest("FLEET", "automation is required")
	}
	a := req.Automation
	saved, err := s.uc.UpsertAutomation(ctx, fleet.Automation{ID: a.Id, Name: a.Name, ProjectID: a.ProjectId, Agent: a.Agent, Prompt: a.Prompt,
		Schedule: a.Schedule, WorktreeMode: a.WorktreeMode, FullAccess: a.FullAccess, Enabled: a.Enabled})
	if err != nil {
		return nil, err
	}
	return automationPB(saved), nil
}

func (s *FleetService) DeleteAutomation(ctx context.Context, req *v1.AutomationIdRequest) (*emptypb.Empty, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteAutomation(ctx, req.Id)
}

func (s *FleetService) RunAutomationNow(ctx context.Context, req *v1.AutomationIdRequest) (*v1.Automation, error) {
	if err := s.guard(ctx); err != nil {
		return nil, err
	}
	a, err := s.uc.RunAutomationNow(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return automationPB(a), nil
}
