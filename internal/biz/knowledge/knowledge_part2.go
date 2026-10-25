package knowledge

import (
	"aranea-agents/pkg/apierror"
	"context"
	"strings"
)

// EnsureDefaultCollection 返回「默认知识库」，不存在则懒创建（US-14 上传免预选兜底）。
// 默认收件箱是 team Vault（PG 即真相源，无 root_path / SyncEngine）。
// embeddingModel 可空：空 = 词法收件箱；有值则同时绑语义层。
// 按 name + workspace 精确匹配复用——不引入 is_default 标记列，避免多默认库歧义。
// ws 为调用方 workspace（system 调用传 "" 表示共享库）；创建时盖章到 Collection，
// 与 service 层 CreateCollection 的 C-01 租户盖章行为一致——否则懒创建的默认库
// workspace="" 会被 assertCollectionMutateAccess 视为共享只读，导致首次入库 404。
func (u *Usecase) EnsureDefaultCollection(ctx context.Context, embeddingModel string, dim int, ws string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	existing, err := u.collections.GetCollectionByName(ctx, ws, DefaultCollectionName)
	if err == nil {
		return existing, nil
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		return Collection{}, err
	}
	model := strings.TrimSpace(embeddingModel)
	if model == "" {
		dim = 0
	}
	created, err := u.CreateVault(ctx, Collection{
		Name:           DefaultCollectionName,
		Description:    "未指定知识库的文档自动归入此处",
		EmbeddingModel: model,
		Dim:            dim,
		VaultBackend:   VaultBackendTeam,
		Workspace:      ws,
	})
	if err == nil {
		return created, nil
	}

	if apierror.IsCode(err, apierror.CodeConflict) {
		return u.collections.GetCollectionByName(ctx, ws, DefaultCollectionName)
	}
	return Collection{}, err
}

// MoveDocument 文档跨库移动（US-14 整理归档：默认库收件箱 → 分类库）。
// 仅托管库之间支持移动；本地文件库需迁移文件与同步状态，暂明确拒绝。
// 校验目标库存在且与源库 dim 兼容后，委托 Repo 在单事务内完成 documents/chunks 随迁与计数校正。
func (u *Usecase) MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	id = strings.TrimSpace(id)
	targetCollectionID = strings.TrimSpace(targetCollectionID)
	if id == "" {
		return Document{}, ErrIDRequired
	}
	if targetCollectionID == "" {
		return Document{}, ErrCollectionIDRequired
	}
	doc, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if doc.CollectionID == targetCollectionID {
		return doc, nil
	}
	src, err := u.collections.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return Document{}, err
	}
	dst, err := u.collections.GetCollection(ctx, targetCollectionID)
	if err != nil {
		return Document{}, err
	}
	// Local files cannot be moved by merely changing their database collection.
	// Until a filesystem migration exists, use copy-based promotion to a team vault.
	if src.RootPath != "" || dst.RootPath != "" || src.VaultBackend == VaultBackendLocal || dst.VaultBackend == VaultBackendLocal {
		return Document{}, apierror.BadRequest("KNOWLEDGE", "cross-collection moves involving a local vault are not supported; promote a copy to a team vault instead")
	}
	if src.Dim != dst.Dim {
		return Document{}, ErrMoveDimensionMismatch
	}
	return u.documents.MoveDocument(ctx, id, targetCollectionID)
}

// EmbedSetting is persisted platform default for the knowledge embedder.
type EmbedSetting struct {
	Provider  string
	BaseURL   string
	Model     string
	Dim       int
	APIKey    string
	HasAPIKey bool
}

// EmbedConfigured reports whether stored settings are sufficient for the provider.
func EmbedConfigured(s EmbedSetting) bool {
	p := strings.TrimSpace(s.Provider)
	if p == "" {
		return false
	}
	switch p {
	case "ollama":
		return true
	case "huggingface":
		return strings.TrimSpace(s.BaseURL) != ""
	default:
		return s.HasAPIKey
	}
}

// ApplyEmbedPatch merges an update onto current settings.
func ApplyEmbedPatch(cur EmbedSetting, provider, baseURL, apiKey, model string, dim int, updateAPIKey bool) EmbedSetting {
	out := cur
	if p := strings.TrimSpace(provider); p != "" {
		out.Provider = p
	}
	out.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	out.Model = strings.TrimSpace(model)
	if dim > 0 {
		out.Dim = dim
	}
	if updateAPIKey && strings.TrimSpace(apiKey) != "" {
		out.APIKey = strings.TrimSpace(apiKey)
		out.HasAPIKey = true
	}
	return out
}
