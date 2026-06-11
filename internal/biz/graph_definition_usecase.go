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

func (uc *GraphDefinitionUsecase) UpdateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error) {
	previous, err := uc.reader.GetDefinition(ctx, def.ID)
	if err != nil {
		return nil, err
	}
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

func (uc *GraphDefinitionUsecase) DeleteGraph(ctx context.Context, id string) error {
	err := uc.writer.DeleteDefinition(ctx, id)
	if err != nil {
		return err
	}
	uc.mu.Lock()
	delete(uc.defs, id)
	uc.mu.Unlock()
	return nil
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

func (uc *GraphDefinitionUsecase) CreateGraphFromTemplate(ctx context.Context, templateID string, name string, description string) (*GraphDefinition, error) {
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

func (uc *GraphDefinitionUsecase) ImportGraph(ctx context.Context, raw []byte, name, description string) (*GraphDefinition, error) {
	var def GraphDefinition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, apierror.BadRequest("GRAPH", "invalid graph json")
	}
	def.ID = ""
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
