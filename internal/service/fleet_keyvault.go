package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/fleet"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// toolKeyVault 把「连接 › 凭据」适配成运行台的 API Key 保管处：Key 加密存成 token 类凭据，
// 在凭据页可见、可删；运行台只记凭据 id，启动工具时按 id 解密注入。
type toolKeyVault struct{ uc *biz.CredentialUsecase }

func (v toolKeyVault) Save(ctx context.Context, prevID int64, name, site, secret string) (int64, string, error) {
	ws := workspace.IDFromContext(ctx)
	if prevID > 0 {
		_, err := v.uc.Update(ctx, prevID, biz.CredentialUpdateInput{Secret: &secret})
		if err == nil {
			return prevID, ws, nil
		}
		if !apierror.IsCode(err, apierror.CodeNotFound) {
			return 0, "", err
		}
	}
	c, err := v.uc.Create(ctx, biz.CredentialCreateInput{
		Name: name, Site: site, SecretKind: biz.CredentialKindToken, Secret: secret,
		Note: "编码工具的 API Key，启动该工具时自动注入",
	})
	if err != nil {
		return 0, "", err
	}
	return c.ID, ws, nil
}

func (v toolKeyVault) Reveal(ctx context.Context, ws string, id int64) (string, error) {
	return v.uc.RevealSecretForExecution(workspace.WithContext(ctx, ws), id, "inject",
		biz.ExecutionActor{ActorType: "system", ActorKey: "fleet"})
}

func (v toolKeyVault) Delete(ctx context.Context, ws string, id int64) error {
	return v.uc.Delete(workspace.WithContext(ctx, ws), id)
}

var _ fleet.KeyVault = toolKeyVault{}
