package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// GraphDefinitionProvider is the narrow interface the execution side needs
// to resolve graph definitions without depending on the full DefinitionUsecase.
type GraphDefinitionProvider interface {
	GetGraph(ctx context.Context, id string) (*GraphDefinition, error)
}

// GraphDefinitionUsecase handles graph definition CRUD, templates, and versioning.
// Separated from execution lifecycle to isolate concerns and enable independent testing.
type GraphDefinitionUsecase struct {
	reader    GraphReader
	writer    GraphWriter
	factory   GraphDefinitionFactory
	nodeInfo  GraphNodeInfoProvider
	teamGuard TeamGraphGuard // B6/B7：Team 侧守卫（可选，装配期注入）
	mu        sync.RWMutex
	defs      map[string]*GraphDefinition
	lg        loggateway.Logger
}

// NewGraphDefinitionUsecase creates a definition usecase with in-memory definition cache.
func NewGraphDefinitionUsecase(repo GraphRepo, factory GraphDefinitionFactory, nodeInfo GraphNodeInfoProvider, lg loggateway.Logger) *GraphDefinitionUsecase {
	return &GraphDefinitionUsecase{
		reader:   repo,
		writer:   repo,
		factory:  factory,
		nodeInfo: nodeInfo,
		defs:     make(map[string]*GraphDefinition),
		lg:       lg,
	}
}

func (uc *GraphDefinitionUsecase) CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	// Y10：team_owned/team_source/team_id 是 Team 生命周期权威标记，公共
	// 创建入口一律剥离——owned 关系只能经 Team 物化/迁移（CreateOwnedGraph）
	// 或换绑/ORG-11b 回填建立，调用方不可自声明。
	stripTeamGraphMarkers(def)
	return uc.createGraph(ctx, def)
}

// CreateOwnedGraph persists a team-owned asset as part of the owner team's
// materialize lifecycle（B4 物化新建 / 迁移回填），保留 team_owned/team_source/
// team_id 标记。与 UpdateOwnedGraph / DeleteOwnedGraph 对称：调用方即 Team
// 保存钩子，归属校验已在 Team 侧完成。
func (uc *GraphDefinitionUsecase) CreateOwnedGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	return uc.createGraph(ctx, def)
}

func (uc *GraphDefinitionUsecase) createGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	if def.ID == "" {
		def.ID = uuid.New().String()
	}
	now := time.Now()
	def.CreatedAt = now
	def.UpdatedAt = now
	if def.Version <= 0 {
		def.Version = 1
	}
	syncVersionMetadata(def)
	saved, err := uc.writer.SaveDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return saved, nil
}

// stripTeamGraphMarkers removes caller-supplied team ownership markers（Y10）：
// 公共 CRUD 入口不可自声明 team_owned/team_source/team_id。
func stripTeamGraphMarkers(def *GraphDefinition) {
	def.TeamID = ""
	if def.Metadata != nil {
		delete(def.Metadata, GraphMetadataTeamOwnedKey)
		delete(def.Metadata, GraphMetadataTeamSourceKey)
	}
}

func (uc *GraphDefinitionUsecase) GetGraph(ctx context.Context, id string) (*GraphDefinition, error) {
	uc.mu.RLock()
	if def, ok := uc.defs[id]; ok {
		uc.mu.RUnlock()
		return def, nil
	}
	uc.mu.RUnlock()
	def, err := uc.reader.GetDefinition(ctx, id)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[id] = def
	uc.mu.Unlock()
	return def, nil
}

func (uc *GraphDefinitionUsecase) ListGraphs(ctx context.Context, pageSize int, pageToken string) ([]*GraphDefinition, string, error) {
	return uc.reader.ListDefinitions(ctx, pageSize, pageToken)
}

// ListGraphsByWorkspace returns graph definitions visible to the given workspace (P2-B).
func (uc *GraphDefinitionUsecase) ListGraphsByWorkspace(ctx context.Context, pageSize int, pageToken string, workspaceID string) ([]*GraphDefinition, string, error) {
	return uc.reader.ListDefinitionsByWorkspace(ctx, pageSize, pageToken, workspaceID)
}

func (uc *GraphDefinitionUsecase) UpdateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	previous, err := uc.reader.GetDefinition(ctx, def.ID)
	if err != nil {
		return nil, err
	}
	if isTeamOwnedGraph(previous) {
		// owned 图：Team 标记以 DB 现状为准强制恢复（编辑器不感知服务端
		// team 标记；owned 关系只能经 Team 保存钩子改变）。
		preserveTeamGraphMarkers(def, previous)
		// B6：team-owned 图的保存经 Team guard（活跃 Run 锁定 + 反向同步
		// source/members）；guard 在事务内执行实际写库。
		if uc.guard() != nil && previous.TeamID != "" {
			var saved *GraphDefinition
			err := uc.guard().OnTeamOwnedGraphSaved(ctx, def, func(txCtx context.Context) error {
				var uerr error
				saved, uerr = uc.updateGraph(txCtx, def, previous)
				return uerr
			})
			if err != nil {
				return nil, err
			}
			return saved, nil
		}
		return uc.updateGraph(ctx, def, previous)
	}
	// 非 owned 图（Y10）：剥离伪造的 owned/team_source 自声明；team_id 以
	// DB 现状为准（ORG-11b external 链接回填经 Team 侧直写，不经本路径）。
	stripTeamGraphMarkers(def)
	def.TeamID = previous.TeamID
	return uc.updateGraph(ctx, def, previous)
}

func (uc *GraphDefinitionUsecase) updateGraph(ctx context.Context, def *GraphDefinition, previous *GraphDefinition) (*GraphDefinition, error) {
	appendVersionHistory(def, previous, uc.lg)
	now := time.Now()
	def.UpdatedAt = now
	syncVersionMetadata(def)
	saved, err := uc.writer.UpdateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	syncVersionMetadata(saved)
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return saved, nil
}

// preserveTeamGraphMarkers keeps server-side team markers authoritative across
// Graph editor saves：编辑器不感知 team_owned/team_source/team_id，owned 关系
// 只能经 Team 保存钩子改变，保存时从 DB 现状强制恢复。
func preserveTeamGraphMarkers(def, previous *GraphDefinition) {
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	def.Metadata[GraphMetadataTeamOwnedKey] = true
	if _, ok := def.Metadata[GraphMetadataTeamSourceKey]; !ok {
		if previous.Metadata != nil {
			if src, ok := previous.Metadata[GraphMetadataTeamSourceKey]; ok {
				def.Metadata[GraphMetadataTeamSourceKey] = src
			}
		}
	}
	// Y10：team_id 无条件以 DB 现状为准——条件恢复（仅空时回填）允许调用方
	// 改绑他队，经 B6 guard 反向同步即构成跨 team 写。
	def.TeamID = previous.TeamID
}

func (uc *GraphDefinitionUsecase) DeleteGraph(ctx context.Context, id string) error {
	// B7 删除保护：team-owned 图（属主存在）/被 external 引用的独立图拒绝删除。
	if uc.guard() != nil {
		if def, err := uc.reader.GetDefinition(ctx, id); err == nil && def != nil {
			if err := uc.guard().CheckGraphDeletable(ctx, def); err != nil {
				return err
			}
		}
	}
	return uc.deleteGraph(ctx, id)
}

// UpdateOwnedGraph updates a team-owned asset as part of the owner team's
// materialize lifecycle（B4 按 preset 重建 / 迁移回填），跳过 B6 反向同步
// guard——调用方即 Team 保存钩子，guard 的「编辑器保存 → source=custom 镜像 +
// 成员反向派生」不适用于物化路径（否则 team_source 会被误置为 custom）。
// 与 DeleteOwnedGraph 跳过 B7 删除保护对称。
func (uc *GraphDefinitionUsecase) UpdateOwnedGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	previous, err := uc.reader.GetDefinition(ctx, def.ID)
	if err != nil {
		return nil, err
	}
	return uc.updateGraph(ctx, def, previous)
}

// DeleteOwnedGraph deletes a team-owned asset as part of the owner team's
// lifecycle（B5 级联删 / D2 换绑），跳过 B7 用户态删除保护——调用方已完成
// owned 归属校验（deleteOwnedGraphAsset / TeamUsecase.Delete）。
func (uc *GraphDefinitionUsecase) DeleteOwnedGraph(ctx context.Context, id string) error {
	return uc.deleteGraph(ctx, id)
}

func (uc *GraphDefinitionUsecase) deleteGraph(ctx context.Context, id string) error {
	err := uc.writer.DeleteDefinition(ctx, id)
	if err != nil {
		return err
	}
	uc.mu.Lock()
	delete(uc.defs, id)
	uc.mu.Unlock()
	return nil
}

// SetTeamGraphGuard wires the Team-side guard（B6/B7）after construction
// （wire 装配期调用，打破 TeamUsecase ↔ GraphDefinitionUsecase 构造环）.
func (uc *GraphDefinitionUsecase) SetTeamGraphGuard(g TeamGraphGuard) {
	uc.mu.Lock()
	uc.teamGuard = g
	uc.mu.Unlock()
}

func (uc *GraphDefinitionUsecase) guard() TeamGraphGuard {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.teamGuard
}

func (uc *GraphDefinitionUsecase) ReorderGraphs(ctx context.Context, ids []string) error {
	return uc.writer.ReorderGraphs(ctx, ids)
}

func (uc *GraphDefinitionUsecase) VisualizeGraph(ctx context.Context, graphID string, format string) (*GraphVisualization, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := defToBuildConfig(def)
	return uc.factory.Visualize(ctx, cfg)
}

func (uc *GraphDefinitionUsecase) ValidateGraph(ctx context.Context, graphID string) (*GraphValidationResult, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	cfg := defToBuildConfig(def)
	return uc.factory.Validate(ctx, cfg)
}

func (uc *GraphDefinitionUsecase) ListGraphTemplates(ctx context.Context) []GraphTemplateRef {
	return uc.factory.ListTemplates()
}

func (uc *GraphDefinitionUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string, workspaceID string) (*GraphDefinition, error) {
	if strings.HasPrefix(templateID, UserTemplateIDPrefix) {
		graphID := strings.TrimPrefix(templateID, UserTemplateIDPrefix)
		src, err := uc.GetGraph(ctx, graphID)
		if err != nil {
			return nil, err
		}
		if ReadUserTemplateMeta(src) == nil {
			return nil, ErrGraphTemplateNotFound
		}
		def := cloneGraphDefinition(src, uc.lg)
		def.ID = ""
		def.Name = name
		def.Description = description
		def.Version = 0
		def.WorkspaceID = workspaceID // P2-B: set caller workspace
		if def.Metadata != nil {
			delete(def.Metadata, GraphMetadataUserTemplateKey)
			delete(def.Metadata, GraphMetadataVersionHistoryKey)
		}
		return uc.CreateGraph(ctx, def)
	}
	tmpl, ok := uc.factory.GetTemplate(templateID)
	if !ok {
		return nil, ErrGraphTemplateNotFound
	}
	def := uc.factory.TemplateToDef(tmpl, name, description)
	def.WorkspaceID = workspaceID // P2-B: set caller workspace
	return uc.CreateGraph(ctx, def)
}

func (uc *GraphDefinitionUsecase) ExportGraph(ctx context.Context, graphID string) ([]byte, *GraphDefinition, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, nil, err
	}
	export := cloneGraphDefinition(def, uc.lg)
	syncVersionMetadata(export)
	raw, err := json.Marshal(export)
	if err != nil {
		return nil, nil, err
	}
	return raw, export, nil
}

func (uc *GraphDefinitionUsecase) ImportGraph(ctx context.Context, raw []byte, name, description, workspaceID string) (*GraphDefinition, error) {
	var def GraphDefinition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, apierror.BadRequest("GRAPH", "invalid graph json")
	}
	def.ID = ""
	def.WorkspaceID = workspaceID // P2-B: set caller workspace (ignore exported value)
	if strings.TrimSpace(name) != "" {
		def.Name = name
	}
	if strings.TrimSpace(description) != "" {
		def.Description = description
	}
	def.Version = 0
	if def.Metadata != nil {
		delete(def.Metadata, GraphMetadataVersionHistoryKey)
	}
	cfg := BuildConfigFromGraphDefinition(&def)
	if err := uc.ensureValidBuildConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return uc.CreateGraph(ctx, &def)
}

func (uc *GraphDefinitionUsecase) ensureValidBuildConfig(ctx context.Context, cfg GraphBuildConfig) error {
	result, err := uc.factory.Validate(ctx, cfg)
	if err != nil {
		return err
	}
	if result != nil && result.HasErrors() {
		return apierror.BadRequest("GRAPH", "graph failed validation")
	}
	return nil
}

func (uc *GraphDefinitionUsecase) ListGraphVersions(ctx context.Context, graphID string) ([]GraphVersionEntry, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	return ListGraphVersionEntries(def), nil
}

func (uc *GraphDefinitionUsecase) RollbackGraphVersion(ctx context.Context, graphID string, version int) (*GraphDefinition, error) {
	current, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	snapshot := FindGraphVersionSnapshot(current, version, uc.lg)
	if snapshot == nil {
		return nil, apierror.NotFound("GRAPH", "graph version not found")
	}
	restored := cloneGraphDefinition(snapshot, uc.lg)
	restored.ID = graphID
	restored.CreatedAt = current.CreatedAt
	return uc.UpdateGraph(ctx, restored)
}

func (uc *GraphDefinitionUsecase) SaveGraphAsTemplate(ctx context.Context, graphID, templateName, category, description string) (*UserTemplateMeta, error) {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(category) == "" {
		category = "custom"
	}
	meta := UserTemplateMeta{
		TemplateID:  UserTemplateIDPrefix + graphID,
		Name:        templateName,
		Category:    category,
		Description: description,
	}
	WriteUserTemplateMeta(def, meta)
	def.UpdatedAt = time.Now()
	saved, err := uc.writer.UpdateDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.defs[saved.ID] = saved
	uc.mu.Unlock()
	return ReadUserTemplateMeta(saved), nil
}

func (uc *GraphDefinitionUsecase) ListUserTemplateGraphs(ctx context.Context) ([]*GraphDefinition, error) {
	return uc.reader.ListUserTemplateDefinitions(ctx, 200)
}

// ListUserTemplateGraphsByWorkspace returns user template graphs visible to the given workspace (P2-B).
func (uc *GraphDefinitionUsecase) ListUserTemplateGraphsByWorkspace(ctx context.Context, workspaceID string) ([]*GraphDefinition, error) {
	return uc.reader.ListUserTemplateDefinitionsByWorkspace(ctx, 200, workspaceID)
}

func (uc *GraphDefinitionUsecase) FindNodeDef(ctx context.Context, graphID string, nodeID string) *NodeTaskMeta {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil
	}
	cfg := defToBuildConfig(def)
	return uc.nodeInfo.FindNodeDef(cfg, nil, nodeID)
}

func (uc *GraphDefinitionUsecase) FindGraphNode(ctx context.Context, graphID string, nodeID string) *NodeDef {
	def, err := uc.GetGraph(ctx, graphID)
	if err != nil {
		return nil
	}
	return nodeDefFromConfig(defToBuildConfig(def), nodeID)
}

// Reader returns the underlying GraphReader for narrow-interface consumers.
func (uc *GraphDefinitionUsecase) Reader() GraphReader { return uc.reader }

// Writer returns the underlying GraphWriter for narrow-interface consumers.
func (uc *GraphDefinitionUsecase) Writer() GraphWriter { return uc.writer }

// Compile-time interface assertion.
var _ GraphDefinitionProvider = (*GraphDefinitionUsecase)(nil)
