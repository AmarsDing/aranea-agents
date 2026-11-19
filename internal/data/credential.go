package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/credential"
	"aranea-agents/internal/data/ent/credentialexecution"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

type credentialRepo struct {
	data *Data
}

var _ biz.CredentialRepo = (*credentialRepo)(nil)

// NewCredentialRepo implements biz.CredentialRepo for credentials /
// credential_executions（E 阶段凭据托管）。
//
// 明文三不进：本层任何错误返回不得拼接 secret_enc 内容；ent 谓词错误只会
// 含列名/谓词结构，不含值，天然安全。
func NewCredentialRepo(d *Data) biz.CredentialRepo {
	return &credentialRepo{data: d}
}

func entToBizCredential(e *ent.Credential) biz.Credential {
	if e == nil {
		return biz.Credential{}
	}
	return biz.Credential{
		ID:          e.ID,
		WorkspaceID: e.WorkspaceID,
		Name:        e.Name,
		Site:        e.Site,
		URL:         e.URL,
		Username:    e.Username,
		SecretKind:  e.SecretKind,
		Sensitivity: e.Sensitivity,
		Note:        e.Note,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func entToBizCredentialExecution(e *ent.CredentialExecution) biz.CredentialExecution {
	if e == nil {
		return biz.CredentialExecution{}
	}
	return biz.CredentialExecution{
		ID:           e.ID,
		CredentialID: e.CredentialID,
		Site:         e.Site,
		Action:       e.Action,
		ActorType:    e.ActorType,
		ActorKey:     e.ActorKey,
		AgentID:      e.AgentID,
		SessionID:    e.SessionID,
		Verdict:      e.Verdict,
		Reason:       e.Reason,
		CreatedAt:    e.CreatedAt,
	}
}

// scopeWorkspace 追加 workspace 谓词；系统工作区（后台任务）放行全量。
func scopeCredentialWorkspace(q *ent.CredentialQuery, workspaceID string) *ent.CredentialQuery {
	if workspaceID == "" || workspaceID == workspace.SystemWorkspaceID {
		return q
	}
	return q.Where(credential.WorkspaceIDEQ(workspaceID))
}

func (r *credentialRepo) ListCredentials(ctx context.Context, workspaceID string, q biz.CredentialListQuery) ([]biz.Credential, int64, error) {
	query := r.data.RW().Read(ctx).Credential.Query()
	query = scopeCredentialWorkspace(query, workspaceID)
	if q.Site != "" {
		query = query.Where(credential.SiteEQ(q.Site))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := query.
		Order(credential.ByUpdatedAt(entsql.OrderDesc())).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]biz.Credential, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizCredential(e))
	}
	return out, int64(total), nil
}

func (r *credentialRepo) GetCredential(ctx context.Context, workspaceID string, id int64) (biz.Credential, error) {
	cred, _, err := r.getCredential(ctx, workspaceID, id)
	return cred, err
}

// getCredential 返回 (元数据, 密文)；密文仅执行面使用。
func (r *credentialRepo) getCredential(ctx context.Context, workspaceID string, id int64) (biz.Credential, string, error) {
	query := r.data.RW().Read(ctx).Credential.Query().Where(credential.IDEQ(id))
	query = scopeCredentialWorkspace(query, workspaceID)
	e, err := query.Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Credential{}, "", apierror.NotFound("CREDENTIAL", "credential not found")
		}
		return biz.Credential{}, "", err
	}
	return entToBizCredential(e), e.SecretEnc, nil
}

func (r *credentialRepo) GetCredentialSecret(ctx context.Context, workspaceID string, id int64) (biz.Credential, string, error) {
	return r.getCredential(ctx, workspaceID, id)
}

func (r *credentialRepo) CreateCredential(ctx context.Context, c biz.Credential, secretEnc string) (biz.Credential, error) {
	e, err := r.data.RW().Write(ctx).Credential.Create().
		SetWorkspaceID(c.WorkspaceID).
		SetName(c.Name).
		SetSite(c.Site).
		SetURL(c.URL).
		SetUsername(c.Username).
		SetSecretKind(c.SecretKind).
		SetSensitivity(c.Sensitivity).
		SetNote(c.Note).
		SetSecretEnc(secretEnc).
		SetCreatedAt(c.CreatedAt).
		SetUpdatedAt(c.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Credential{}, err
	}
	return entToBizCredential(e), nil
}

func (r *credentialRepo) UpdateCredential(ctx context.Context, workspaceID string, id int64, in biz.CredentialUpdateInput, secretEnc *string) (biz.Credential, error) {
	// 先按 workspace 校验归属（不泄露存在性：跨租户与不存在同样 NotFound）。
	if _, _, err := r.getCredential(ctx, workspaceID, id); err != nil {
		return biz.Credential{}, err
	}
	up := r.data.RW().Write(ctx).Credential.UpdateOneID(id)
	if in.Name != nil {
		up.SetName(*in.Name)
	}
	if in.URL != nil {
		up.SetURL(*in.URL)
	}
	if in.Username != nil {
		up.SetUsername(*in.Username)
	}
	if in.Sensitivity != nil && *in.Sensitivity != "" {
		up.SetSensitivity(*in.Sensitivity)
	}
	if in.Note != nil {
		up.SetNote(*in.Note)
	}
	if secretEnc != nil {
		up.SetSecretEnc(*secretEnc)
	}
	e, err := up.Save(ctx)
	if err != nil {
		return biz.Credential{}, err
	}
	return entToBizCredential(e), nil
}

func (r *credentialRepo) DeleteCredential(ctx context.Context, workspaceID string, id int64) error {
	if _, _, err := r.getCredential(ctx, workspaceID, id); err != nil {
		return err
	}
	return r.data.RW().Write(ctx).Credential.DeleteOneID(id).Exec(ctx)
}

func (r *credentialRepo) FindCredentialBySite(ctx context.Context, workspaceID, site string) (biz.Credential, error) {
	query := r.data.RW().Read(ctx).Credential.Query().Where(credential.SiteEQ(site))
	query = scopeCredentialWorkspace(query, workspaceID)
	rows, err := query.Order(credential.ByUpdatedAt(entsql.OrderDesc())).Limit(1).All(ctx)
	if err != nil {
		return biz.Credential{}, err
	}
	if len(rows) == 0 {
		return biz.Credential{}, apierror.NotFound("CREDENTIAL", "no credential for site")
	}
	return entToBizCredential(rows[0]), nil
}

func (r *credentialRepo) InsertCredentialExecution(ctx context.Context, in biz.CredentialExecution) error {
	_, err := r.data.RW().Write(ctx).CredentialExecution.Create().
		SetCredentialID(in.CredentialID).
		SetSite(in.Site).
		SetAction(in.Action).
		SetActorType(in.ActorType).
		SetActorKey(in.ActorKey).
		SetAgentID(in.AgentID).
		SetSessionID(in.SessionID).
		SetVerdict(in.Verdict).
		SetReason(in.Reason).
		SetCreatedAt(in.CreatedAt).
		Save(ctx)
	return err
}

func (r *credentialRepo) ListCredentialExecutions(ctx context.Context, workspaceID string, q biz.CredentialExecutionQuery) ([]biz.CredentialExecution, int64, error) {
	query := r.data.RW().Read(ctx).CredentialExecution.Query()
	// 审计表无 workspace 列（执行行已含 actor/agent 定位维度），管理面查询
	// 依赖 credential_id 归属过滤；site 过滤为全局（审计视角）。
	if q.CredentialID > 0 {
		query = query.Where(credentialexecution.CredentialIDEQ(q.CredentialID))
	}
	if q.Site != "" {
		query = query.Where(credentialexecution.SiteEQ(q.Site))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := query.
		Order(credentialexecution.ByCreatedAt(entsql.OrderDesc())).
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	out := make([]biz.CredentialExecution, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizCredentialExecution(e))
	}
	return out, int64(total), nil
}
