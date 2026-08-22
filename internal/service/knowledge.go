package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const maxIngestBytes = 32 << 20

var allowedIngestMIMEs = map[string]bool{
	"text/plain":         true,
	"text/markdown":      true,
	"text/csv":           true,
	"text/html":          true,
	"text/xml":           true,
	"application/json":   true,
	"application/xml":    true,
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	// Phase 9：图片经 VisionExtractor（多模态 LLM）入库。
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

var (
	knowledgeIngestTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_knowledge_ingest_documents_total",
		Help: "Total documents ingested into knowledge collections.",
	})
	knowledgeSearchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_knowledge_search_duration_seconds",
		Help:    "Duration of knowledge search requests.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})
)

// KnowledgeService implements kratos knowledge.v1.
type KnowledgeSearchDeps struct {
	Retriever *knowledge.Retriever
	Router    *knowledge.AdaptiveRouter
	Evaluator *knowledge.RetrievalEvaluator
	// Federated 全库智能路由（US-14 检索免选择）：Search collection_id 留空时启用。
	Federated *knowledge.FederatedRetriever
}

type KnowledgeService struct {
	v1.UnimplementedKnowledgeServiceServer
	uc            *biz.KnowledgeUsecase
	embedder      knowledge.Embedder
	embedderAdmin knowledge.EmbedderAdmin
	search        KnowledgeSearchDeps
	organizer     *knowledge.MarkdownOrganizer // LLM 整理为 MD（nil 时跳过整理）
	extractors    *knowledge.ExtractorRegistry // 模态路由（Vision 优先于 Text；nil 时退化 TextExtractor）
	assets        *knowledge.AssetStore        // 原图留存（nil 时跳过血缘）
	eventBus      biz.EventBus
	systemSetting biz.SystemSettingRepo
	vaultSync     VaultSyncController // P1-3：vault 同步生命周期（nil = 未装配，同步不可用）
	lg            loggateway.Logger
	// monitorBus 流程日志总线（装配层经 SetMonitorBus 注入；nil 时仅进程日志，不发流程日志事件）。
	monitorBus contract.MonitorBus
	// linkIndex 统一链接内存图（SP1-D；构造时创建并接线进 uc，启动 readiness 后
	// 经 LoadKnowledgeLinkIndex 全量构建；SP1-E 反链查询直读）。
	linkIndex *bizknowledge.LinkIndex
	// rebuildRuns SP1-H 块索引重建在途集合门（单进程部署 N-1；value 恒为 struct{}）。
	rebuildRuns sync.Map
	// reembedRuns 文档级重嵌入在途门；自动修复、人工重嵌入和语义层启用共用，
	// 防止同一文档并发执行 delete-old/insert-new。
	reembedRuns sync.Map
	// jobLock 跨进程重建/重嵌入租约（Postgres advisory lock）；nil 时仅本进程 sync.Map。
	jobLock  KnowledgeJobLocker
	agentMem *bizknowledge.AgentMemoryProjector
	// writeBackGraph 写回/晋升图谱钩子（2026-08-16 装配）：构造参数透传，biz 写回
	// 管线经 SetWriteBackGraph 收口（词条页实体共现 + typed 关系抽取）；service 晋升
	// 管线（不经写回）在 chunk 重放后显式触发（P1-a：晋升文档图谱孤立节点根治）。
	// nil（未接线/环境关闭）时两路均降级跳过。
	writeBackGraph bizknowledge.WriteBackGraphFunc
}

// VaultSyncController vault 同步循环生命周期窄接口（P1-3 生产装配）。
// 实现：*knowledge.VaultSyncSupervisor。
type VaultSyncController interface {
	// StartVault 启动单 vault 同步循环（幂等；RunVault 启动即扫一轮）。
	StartVault(vault biz.KnowledgeCollection)
	// StopVault 停止单 vault 同步循环（幂等）。
	StopVault(vaultID string)
}

// SetVaultSyncController 注入 vault 同步控制器（装配在 wire/app 层，打破
// KnowledgeService → Supervisor → Usecase 构造顺序约束，同 SetTimeoutHandler 模式）。
func (s *KnowledgeService) SetVaultSyncController(c VaultSyncController) {
	s.vaultSync = c
}

// SetMonitorBus 注入流程日志总线（装配在 wire/app 层，同 SetVaultSyncController 模式）。
func (s *KnowledgeService) SetMonitorBus(bus contract.MonitorBus) {
	s.monitorBus = bus
}

// SetAgentMemoryProjector 接线 SP7 G1 投影器（可选；nil 时 ProjectAgentMemory no-op）。
// 同时注入 chunk 重放钩子，确保投影写文档后 chunks/FTS 同步重建。
func (s *KnowledgeService) SetAgentMemoryProjector(p *bizknowledge.AgentMemoryProjector) {
	if p != nil {
		p.SetReplay(s.replayAgentMemoryChunks)
	}
	s.agentMem = p
}

// knowledgeFlow 创建知识域流程日志发射器（无会话上下文；bus 为 nil 时仅进程日志）。
func (s *KnowledgeService) knowledgeFlow(ctx context.Context) *event.TraceEmitter {
	return event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainKnowledge,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
}

func NewKnowledgeService(uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, searchDeps KnowledgeSearchDeps, organizer *knowledge.MarkdownOrganizer, extractors *knowledge.ExtractorRegistry, assets *knowledge.AssetStore, eventBus biz.EventBus, systemSetting biz.SystemSettingRepo, writeBackGraph bizknowledge.WriteBackGraphFunc, lg loggateway.Logger) *KnowledgeService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var admin knowledge.EmbedderAdmin
	if a, ok := embedder.(knowledge.EmbedderAdmin); ok {
		admin = a
	}
	s := &KnowledgeService{
		uc:             uc,
		embedder:       embedder,
		embedderAdmin:  admin,
		search:         searchDeps,
		organizer:      organizer,
		extractors:     extractors,
		assets:         assets,
		eventBus:       eventBus,
		systemSetting:  systemSetting,
		writeBackGraph: writeBackGraph,
		lg:             lg,
	}
	// SP1-D：统一链接内存图 + WS 增量事件出口（GraphDeltaPublisher 适配器）。
	// uc 为共享实例，构造期接线（serve 前单线程）；启动全量加载由 app 层
	// readiness 后触发（LoadKnowledgeLinkIndex）。
	if uc != nil {
		s.linkIndex = bizknowledge.NewLinkIndex()
		uc.SetLinkIndex(s.linkIndex, newKnowledgeGraphDeltaPublisher(eventBus))
		// 写回飞轮 chunk 重放钩子（2026-08-15）：knowledge_write 工具直调 biz
		// Usecase（不经 service 包装），重放必须在 biz 层收口才能覆盖该路径。
		s.BindDerivedIndexHooks()
	}
	return s
}

// BindDerivedIndexHooks 把 chunk 重放与图谱钩子接到共享 Usecase。
// NewKnowledgeService 与 app BeforeStart 双点调用：即使未来构造顺序变化，
// knowledge_write / AutoMemory 直调 Usecase 仍会重放派生索引。
func (s *KnowledgeService) BindDerivedIndexHooks() {
	if s == nil || s.uc == nil {
		return
	}
	s.uc.SetWriteBackReplay(s.replayWriteBackChunks)
	s.uc.SetWriteBackGraph(s.writeBackGraph)
}

// SetJobLocker 注入跨进程作业锁（装配在 app 层；nil = 仅本进程门）。
func (s *KnowledgeService) SetJobLocker(l KnowledgeJobLocker) {
	if s == nil {
		return
	}
	s.jobLock = l
}

// CreateCollection creates a knowledge collection (Vault V2 / SP1-F team library).
// vault_backend=local（缺省）：root_path 必填，文件系统即真相源；embedding_model 可选（空 = 词法库）。
// vault_backend=team：root_path 必须为空，PG 即真相源，不拉起同步循环。
func (s *KnowledgeService) CreateCollection(ctx context.Context, req *v1.CreateCollectionRequest) (*v1.KnowledgeCollection, error) {
	name := strings.TrimSpace(req.GetName())
	model := strings.TrimSpace(req.GetEmbeddingModel())
	backend := strings.TrimSpace(req.GetVaultBackend())
	if backend == "" {
		backend = bizknowledge.VaultBackendLocal
	}
	if name == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "name is required")
	}
	if backend == bizknowledge.VaultBackendLocal && strings.TrimSpace(req.GetRootPath()) == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "root_path is required")
	}
	// embedding 模型校验仅在显式指定时进行（留空 = 无语义层词法库）。
	if model != "" && s.embedderAdmin != nil {
		_, _, embedderModel, _, configured, _ := s.embedderAdmin.Config()
		if configured && embedderModel != "" && embedderModel != model {
			return nil, apierror.BadRequest("KNOWLEDGE", "embedding_model does not match current embedder model "+embedderModel)
		}
	}
	in := biz.KnowledgeCollection{
		Name:           name,
		Description:    req.GetDescription(),
		EmbeddingModel: model,
		RootPath:       req.GetRootPath(),
		VaultBackend:   backend,
	}
	// P0-2（2026-08-21）：dim 快照取当前 embedder 实际维度，禁止落入 biz 层
	// 1536 兜底——运行中建库若快照与 embedder 维度不一致（如 bge-m3=1024），
	// ingest 会被 data 层 dimension mismatch 全部拒绝，且启动对账
	// （reconcileEmbeddingDim）只在重启后生效，故障窗口内整库不可写。
	if model != "" && s.embedderAdmin != nil {
		if _, _, _, dim, _, _ := s.embedderAdmin.Config(); dim > 0 {
			in.Dim = dim
		}
	}
	// C-01: stamp caller workspace so collections are tenant-scoped.
	// System callers create shared collections (empty workspace).
	if !workspace.IsSystem(ctx) {
		in.Workspace = workspace.IDFromContext(ctx)
	}
	c, err := s.uc.CreateVault(ctx, in)
	if err != nil {
		s.lg.Error("创建知识库失败",
			loggateway.StepID("knowledge.collection.create_fail"),
			loggateway.Str("name", name),
			loggateway.Err(err),
		)
		return nil, err
	}
	// P1-3：拉起同步循环（启动即扫一轮，新 vault 立即入库）。
	// SP1-F：team 库 PG 即真相源，无文件监听，不启动 SyncEngine。
	if s.vaultSync != nil && c.VaultBackend != bizknowledge.VaultBackendTeam {
		s.vaultSync.StartVault(c)
	}
	return toProtoCollection(c), nil
}

// GetCollection returns one collection.
func (s *KnowledgeService) GetCollection(ctx context.Context, req *v1.GetCollectionRequest) (*v1.KnowledgeCollection, error) {
	c, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, c); err != nil {
		return nil, err
	}
	return toProtoCollection(c), nil
}

// ListCollections returns collections visible in the caller's workspace.
func (s *KnowledgeService) ListCollections(ctx context.Context, req *v1.ListCollectionsRequest) (*v1.ListCollectionsResponse, error) {
	ws := ""
	// C-01: tenant callers list only their workspace; system sees all (ws="").
	if !workspace.IsSystem(ctx) {
		ws = workspace.IDFromContext(ctx)
	}
	cols, total, err := s.uc.ListCollections(ctx, ws, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeCollection, 0, len(cols))
	for _, c := range cols {
		out = append(out, toProtoCollection(c))
	}
	return &v1.ListCollectionsResponse{Items: out, Total: int32(total)}, nil
}

// DeleteCollection removes a collection.
func (s *KnowledgeService) DeleteCollection(ctx context.Context, req *v1.DeleteCollectionRequest) (*emptypb.Empty, error) {
	c, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, c); err != nil {
		return nil, err
	}
	// P1-3：先停同步循环再删记录，避免 goroutine 对已删 vault 继续回写状态。
	if s.vaultSync != nil {
		s.vaultSync.StopVault(req.GetId())
	}
	if err := s.uc.DeleteCollection(ctx, req.GetId()); err != nil {
		s.lg.Error("删除知识库失败",
			loggateway.StepID("knowledge.collection.delete_fail"),
			loggateway.Str("collection_id", req.GetId()),
			loggateway.Err(err),
		)
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ServeDocumentAsset 原始文件流式输出（G2-B6）：vault 文档读 collection root +
// rel_path；历史非 vault 文档经 AssetStore 解析 asset_uri。图片/音频/视频/pdf
// inline 渲染，其余（word 等）attachment 下载。ServeContent 支持 Range（媒体拖动）。
// 路由经 custom route 注册（同 artifact 下载模式），鉴权走标准 auth 过滤器。
func (s *KnowledgeService) ServeDocumentAsset(w http.ResponseWriter, r *http.Request, id string) {
	ref, err := s.uc.ResolveDocumentAsset(r.Context(), id)
	if err != nil {
		writeAssetError(w, err)
		return
	}
	// C-01：租户门禁（NotFound 防泄漏）。doc → collection 已在 biz 内取过，
	// 此处为访问校验再取一次（廉价，与 GetCollection 其他调用点一致）。
	_, _, err = s.requireDocumentRead(r.Context(), id)
	if err != nil {
		writeAssetError(w, err)
		return
	}

	abs := ref.AbsPath
	if abs == "" {
		if s.assets == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		abs = s.assets.Resolve(ref.AssetURI)
		if abs == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	modTime := ref.ModTime
	if modTime.IsZero() {
		if info, statErr := f.Stat(); statErr == nil {
			modTime = info.ModTime()
		}
	}

	mimeType := strings.TrimSpace(ref.MimeType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(ref.Name))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	disposition := "attachment"
	if assetInlineAllowed(mimeType) {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, ref.Name))
	http.ServeContent(w, r, ref.Name, modTime, f)
}

// assetInlineAllowed 仅非可执行媒体允许 inline（HTML/JS 内联渲染有 XSS 风险，
// 与 artifact 下载 §13.7 同约束）。
func assetInlineAllowed(mime string) bool {
	return strings.HasPrefix(mime, "image/") ||
		strings.HasPrefix(mime, "audio/") ||
		strings.HasPrefix(mime, "video/") ||
		mime == "application/pdf"
}

// writeAssetError 映射 biz 错误到 HTTP 状态码（原始流不走 kratos JSON 错误编码）。
func writeAssetError(w http.ResponseWriter, err error) {
	switch {
	case apierror.IsCode(err, apierror.CodeNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case apierror.IsCode(err, apierror.CodeBadRequest):
		http.Error(w, "bad request", http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// assertCollectionAccess rejects cross-tenant collection read access (C-01).
// Returns NotFound (not Forbidden) to avoid leaking collection existence.
// Empty collection.Workspace is treated as globally shared (system/legacy) for reads.
func (s *KnowledgeService) assertCollectionAccess(ctx context.Context, c biz.KnowledgeCollection) error {
	return s.checkCollectionAccess(ctx, c, false)
}

// assertCollectionMutateAccess rejects mutate on shared/cross-tenant collections.
// Shared collections (workspace="") are read-only for tenants (fail-closed).
func (s *KnowledgeService) assertCollectionMutateAccess(ctx context.Context, c biz.KnowledgeCollection) error {
	return s.checkCollectionAccess(ctx, c, true)
}

func (s *KnowledgeService) assertDocumentReadable(ctx context.Context, doc biz.KnowledgeDocument) error {
	if bizknowledge.DocumentVisibleTo(ctx, doc) {
		return nil
	}
	return apierror.NotFound("KNOWLEDGE", "document not found")
}

func (s *KnowledgeService) requireDocumentRead(ctx context.Context, id string) (biz.KnowledgeDocument, biz.KnowledgeCollection, error) {
	doc, err := s.uc.GetDocument(ctx, id)
	if err != nil {
		return biz.KnowledgeDocument{}, biz.KnowledgeCollection{}, err
	}
	if err := s.assertDocumentReadable(ctx, doc); err != nil {
		return biz.KnowledgeDocument{}, biz.KnowledgeCollection{}, err
	}
	col, err := s.uc.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return biz.KnowledgeDocument{}, biz.KnowledgeCollection{}, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return biz.KnowledgeDocument{}, biz.KnowledgeCollection{}, err
	}
	return doc, col, nil
}

func (s *KnowledgeService) checkCollectionAccess(ctx context.Context, c biz.KnowledgeCollection, mutate bool) error {
	callerWS := workspace.IDFromContext(ctx)
	var err error
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, c.Workspace)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, c.Workspace)
	}
	if err != nil {
		s.lg.Warn("knowledge collection access denied: workspace mismatch",
			loggateway.StepID("knowledge.idor"),
			loggateway.Str("collection_id", c.ID),
			loggateway.Str("caller_ws", callerWS),
			loggateway.Str("resource_ws", c.Workspace))
		return apierror.NotFound("KNOWLEDGE", "collection not found")
	}
	return nil
}

// IngestDocument ingests a document, chunks it, embeds the chunks, and indexes them.
// The heavy work runs asynchronously; the document record is returned immediately.
func (s *KnowledgeService) IngestDocument(ctx context.Context, req *v1.IngestDocumentRequest) (*v1.KnowledgeDocument, error) {
	raw, err := base64.StdEncoding.DecodeString(req.GetContentBase64())
	if err != nil {
		return nil, apierror.BadRequest("KNOWLEDGE", "content_base64 is not valid base64")
	}
	if len(raw) == 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "content is empty")
	}
	if len(raw) > maxIngestBytes {
		return nil, apierror.BadRequest("KNOWLEDGE", "file too large: max 32MB")
	}
	detected := http.DetectContentType(raw[:min(512, len(raw))])
	if !resolveIngestMIMEAllowed(detected, req.GetMimeType(), req.GetSource()) {
		return nil, apierror.BadRequest("KNOWLEDGE", "unsupported content type: "+detected)
	}
	col, err := s.resolveIngestCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}

	// Validate metadata BEFORE creating the document record.
	isImage := knowledge.IsImageSource(req.GetSource(), req.GetMimeType())
	modality, extractorName := "text", "text"
	if isImage {
		modality, extractorName = "image", "vision"
	}
	metaJSON, err := mergeIngestMetadata(req.GetMetadataJson(), modality, extractorName)
	if err != nil {
		return nil, apierror.BadRequest("KNOWLEDGE", err.Error())
	}

	// 文本类：同步提取（本地解析快），失败即 400 不落孤儿文档。
	// 图片：视觉 LLM 提取为耗时远程调用（最长 60s），先落文档后异步提取，
	// 失败置 status=error（NFR-12），与 indexing → indexed/error 状态流一致。
	text := ""
	organized := isImage
	if !isImage {
		var extractErr error
		text, extractErr = s.extractText(ctx, raw, req.GetSource(), req.GetMimeType())
		if extractErr != nil {
			return nil, apierror.BadRequest("KNOWLEDGE", extractErr.Error())
		}
		if strings.TrimSpace(text) == "" {
			return nil, apierror.BadRequest("KNOWLEDGE", "document contains no extractable text")
		}

		// 可选：LLM 整理为结构化 Markdown（unset 默认开启；.md 本身已是 Markdown 跳过）。
		// 失败一律降级原文本，不阻塞入库（NFR-11）。
		if s.organizer != nil && organizeToMarkdownEnabled(req) && !isMarkdownSource(req.GetSource(), req.GetMimeType()) {
			if md, ok, orgErr := s.organizer.Organize(ctx, text, req.GetSource(), req.GetMimeType()); orgErr != nil {
				s.lg.Warn("Markdown 整理异常，使用原文本继续",
					loggateway.StepID("knowledge.ingest.organize_err"),
					loggateway.Str("source", req.GetSource()),
					loggateway.Err(orgErr),
				)
			} else if ok {
				text = md
				organized = true
			}
		}
		text = s.uc.MaybeAutolinkOutgoing(ctx, col.ID, "", req.GetSource(), text)
	}

	vaultRel := ""
	vaultBytes := raw
	if !isImage && isMarkdownSource(req.GetSource(), req.GetMimeType()) && text != "" {
		vaultBytes = []byte(text)
	}
	contentHash := biz.KnowledgeHashContent(string(raw))
	if text != "" {
		contentHash = biz.KnowledgeHashContent(text)
	}
	if strings.TrimSpace(req.GetTargetDir()) != "" {
		contentHash = biz.KnowledgeHashContent(string(vaultBytes))
	}
	if existing, findErr := s.uc.GetDocumentByContentHash(ctx, col.ID, contentHash); findErr == nil && existing.ID != "" {
		return toProtoDocument(existing), nil
	}

	// G1-B3：上传到 vault 子目录——原始文件先落盘（文件系统为真相源），文档镜像
	// 带 rel_path + content_hash（同步链视为已应用，不重复处理）。同名冲突
	// CodeConflict；CreateDocument 失败补偿删除已落盘文件（防孤儿）。
	if dir := strings.TrimSpace(req.GetTargetDir()); dir != "" {
		rel, uploadErr := s.uc.WriteVaultUpload(ctx, col.ID, dir, req.GetSource(), vaultBytes)
		if uploadErr != nil {
			return nil, uploadErr
		}
		vaultRel = rel
	}

	doc := biz.KnowledgeDocument{
		CollectionID: col.ID,
		Source:       req.GetSource(),
		MimeType:     req.GetMimeType(),
		SizeBytes:    int64(len(raw)),
		ContentText:  text,
		Organized:    organized,
		RelPath:      vaultRel,
		ContentHash:  contentHash,
	}
	if strings.TrimSpace(doc.Summary) == "" {
		sum, typ, sh := bizknowledge.DeriveSummaryCard(text, vaultRel, req.GetSource())
		doc.Summary = sum
		doc.DocType = typ
		doc.SummaryHash = sh
	}
	// Phase 9：原图留存血缘（asset_uri）；asset 文件名依赖 doc ID，故提前生成。
	if isImage && s.assets != nil {
		doc.ID = uuid.NewString()
		if uri, saveErr := s.assets.Save(doc.ID, doc.Source, raw); saveErr != nil {
			s.lg.Warn("原图留存失败，跳过血缘",
				loggateway.StepID("knowledge.ingest.asset_save"),
				loggateway.Str("source", doc.Source),
				loggateway.Err(saveErr),
			)
		} else {
			doc.AssetURI = uri
		}
	}
	created, err := s.uc.CreateDocument(ctx, doc)
	if err != nil {
		s.lg.Error("创建知识文档失败",
			loggateway.StepID("knowledge.document.create_fail"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Str("source", doc.Source),
			loggateway.Err(err),
		)
		if vaultRel != "" {
			if rbErr := s.uc.RemoveVaultFile(ctx, col.ID, vaultRel); rbErr != nil {
				s.lg.Warn("vault 上传补偿删除失败（文件已落盘但入库失败）",
					loggateway.StepID("knowledge.ingest.compensate_fail"),
					loggateway.Str("rel_path", vaultRel),
					loggateway.Err(rbErr),
				)
			}
		}
		return nil, err
	}
	doc = created

	strategy := knowledge.ParseChunkStrategy(req.GetChunkStrategy())
	// 词法库（无 embedding_model）：跳过 embedding，与 vault 同步链 buildChunks 一致。
	embedder := s.embedder
	if strings.TrimSpace(col.EmbeddingModel) == "" {
		embedder = nil
	}
	uc := s.uc

	params := knowledge.IngestParams{
		DocID:        doc.ID,
		CollectionID: col.ID,
		Text:         text,
		MetadataJSON: metaJSON,
		Strategy:     strategy,
		ChunkSize:    int(req.GetChunkSize()),
		ChunkOverlap: int(req.GetChunkOverlap()),
	}
	params.ApplyDefaults()

	// 流程日志：摄取开始（后续 parse/embed/done 在异步管线中发射，共用同一 emitter）。
	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.ingest.start", "知识文档摄取开始",
		event.P("collection_id", col.ID),
		event.P("source", req.GetSource()),
		event.P("size_bytes", len(raw)))

	ingestCtx := appctx.Ctx()
	safego.Go(ingestCtx, "knowledge-ingest", func() {
		if err := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "indexing", "", 0); err != nil {
			s.lg.Error("failed to update document status to indexing",
				loggateway.StepID("knowledge.ingest.status_fail"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
		}
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)

		// 图片：后台经 VisionExtractor 提取为 Markdown（耗时远程调用，最长 60s），
		// 成功后回写 content_text/organized（预览可用），再走统一 chunk/embed 流程；
		// 失败置 status=error（NFR-12）。
		if isImage {
			md, extractErr := s.extractors.Extract(ingestCtx, raw, doc.Source, doc.MimeType)
			if extractErr != nil || strings.TrimSpace(md) == "" {
				if extractErr == nil {
					extractErr = apierror.Internal(apierror.DomainKnowledge, "vision extract %q: empty response", doc.Source)
				}
				if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", extractErr.Error(), 0); statusErr != nil {
					s.lg.Error("failed to update document status to error",
						loggateway.StepID("knowledge.ingest.status_fail"),
						loggateway.Str("doc_id", doc.ID),
						loggateway.Err(statusErr),
						loggateway.Str("original_error", extractErr.Error()),
					)
				}
				s.publishKnowledgeIngest(col.ID, doc.ID, "error", extractErr.Error(), 0)
				flow.LogError("knowledge.ingest.done", "知识摄取失败",
					event.P("doc_id", doc.ID),
					event.P("error", extractErr.Error()))
				return
			}
			md = uc.MaybeAutolinkOutgoing(ingestCtx, col.ID, doc.ID, doc.Source, md)
			if contentErr := uc.UpdateDocumentContent(ingestCtx, doc.ID, md, true); contentErr != nil {
				s.lg.Warn("图片正文回写失败，继续向量化",
					loggateway.StepID("knowledge.ingest.content_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(contentErr),
				)
			}
			params.Text = md
		}

		bizChunks, err := knowledge.BuildIndexedChunks(ingestCtx, embedder, params, flow)
		if err != nil {
			if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", err.Error(), 0); statusErr != nil {
				s.lg.Error("failed to update document status to error",
					loggateway.StepID("knowledge.ingest.status_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(statusErr),
					loggateway.Str("original_error", err.Error()),
				)
			}
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			flow.LogError("knowledge.ingest.done", "知识摄取失败",
				event.P("doc_id", doc.ID),
				event.P("error", err.Error()))
			return
		}

		if err := uc.CommitIndexedDocument(ingestCtx, col.ID, doc.ID, bizChunks, 1); err != nil {
			if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", err.Error(), 0); statusErr != nil {
				s.lg.Error("failed to update document status to error",
					loggateway.StepID("knowledge.ingest.status_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(statusErr),
					loggateway.Str("original_error", err.Error()),
				)
			}
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			flow.LogError("knowledge.ingest.done", "知识摄取失败",
				event.P("doc_id", doc.ID),
				event.P("error", err.Error()))
			return
		}
		// SP1-C：team/上传文档与 local 同管线——块级双链索引（块/refs + explicit 投影）。
		// 失败降级记日志，不回滚摄取主流程（最终一致，下次重建自愈）。
		if err := uc.RebuildBlockIndex(ingestCtx, col.ID, doc.ID, params.Text); err != nil {
			s.lg.Warn("ingest block index rebuild failed",
				loggateway.StepID("knowledge.ingest.block_index_fail"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
		}
		// 数据库上传路径不经过 vault sync；摄取完成后显式触发实体共现与 typed
		// 关系抽取，避免“可检索但图谱孤立”。钩子异步且 content_hash 幂等。
		s.triggerKnowledgeGraph(ingestCtx, col, []bizknowledge.PromoteTouchedDoc{{
			DocID: doc.ID, Created: true,
		}})
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
		flow.LogDone("knowledge.ingest.done", "知识摄取完成",
			event.P("doc_id", doc.ID),
			event.P("chunk_count", len(bizChunks)))
		knowledgeIngestTotal.Inc()
	})

	return toProtoDocument(doc), nil
}

// ListDocuments returns documents for a collection.
func (s *KnowledgeService) ListDocuments(ctx context.Context, req *v1.ListDocumentsRequest) (*v1.ListDocumentsResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	docs, total, err := s.uc.ListDocuments(ctx, req.GetCollectionId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, toProtoDocument(d))
	}
	return &v1.ListDocumentsResponse{Items: out, Total: int32(total)}, nil
}

// DeleteDocument removes a document and its chunks.
func (s *KnowledgeService) DeleteDocument(ctx context.Context, req *v1.DeleteDocumentRequest) (*emptypb.Empty, error) {
	doc, col, err := s.requireDocumentRead(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteDocument(ctx, doc.ID); err != nil {
		s.lg.Error("删除知识文档失败",
			loggateway.StepID("knowledge.document.delete_fail"),
			loggateway.Str("doc_id", req.GetId()),
			loggateway.Err(err),
		)
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GetDocumentContent returns the full extracted/organized text of one document (preview).
func (s *KnowledgeService) GetDocumentContent(ctx context.Context, req *v1.GetDocumentContentRequest) (*v1.DocumentContent, error) {
	doc, _, err := s.requireDocumentRead(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	resp := &v1.DocumentContent{
		Id:          doc.ID,
		ContentText: doc.ContentText,
		Organized:   doc.Organized,
	}
	// G2-B5：vault 文档附带编辑器数据源（body 原文 + 文件 hash）。文件刚被外部
	// 删除等读失败场景降级为空（预览仍可用），编辑保存时 CAS 会判冲突。
	// 仅文本类 vault 文档返回 raw_content；图片/视频/音频等二进制文件跳过，
	// 避免 protobuf string 字段因非法 UTF-8 序列化失败。
	if doc.RelPath != "" && isTextVaultDocument(doc) {
		if raw, hash, rawErr := s.uc.GetVaultDocumentRaw(ctx, doc.ID); rawErr == nil {
			resp.RawContent = raw
			resp.BaseHash = hash
		}
	}
	return resp, nil
}

// UpdateDocumentContent 编辑保存（G2-B5）：body 写回 vault 文件（frontmatter 保留），
// CAS 冲突留双份并置 conflict=true，写后立即重索引。
func (s *KnowledgeService) UpdateDocumentContent(ctx context.Context, req *v1.UpdateDocumentContentRequest) (*v1.UpdateDocumentContentResponse, error) {
	_, col, err := s.requireDocumentRead(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	doc, conflict, err := s.uc.UpdateVaultDocumentContent(ctx, req.GetId(), req.GetContent(), req.GetBaseHash())
	if err != nil {
		s.lg.Warn("保存知识文档失败",
			loggateway.StepID("knowledge.document.save_fail"),
			loggateway.Str("doc_id", req.GetId()),
			loggateway.Err(err),
		)
		return nil, err
	}
	if conflict {
		s.lg.Warn("保存知识文档检测到并发修改（留双份）",
			loggateway.StepID("knowledge.document.save_conflict"),
			loggateway.Str("doc_id", req.GetId()),
		)
	}
	return &v1.UpdateDocumentContentResponse{Document: toProtoDocument(doc), Conflict: conflict}, nil
}

// MoveDocument moves a document (with its chunks) to another collection (US-14 整理归档).
func (s *KnowledgeService) MoveDocument(ctx context.Context, req *v1.MoveDocumentRequest) (*v1.KnowledgeDocument, error) {
	_, col, err := s.requireDocumentRead(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	doc, err := s.uc.MoveDocument(ctx, req.GetId(), req.GetTargetCollectionId())
	if err != nil {
		s.lg.Error("移动知识文档失败",
			loggateway.StepID("knowledge.document.move_fail"),
			loggateway.Str("doc_id", req.GetId()),
			loggateway.Str("target_collection_id", req.GetTargetCollectionId()),
			loggateway.Err(err),
		)
		return nil, err
	}
	return toProtoDocument(doc), nil
}

// MoveDocumentToDir 库内跨目录移动（G3-B4）：原子 fs move + rel_path 更新
// （身份/chunks 保留，内容未变不重索引）+ 入链重建。同名冲突默认 CodeConflict
// （前端弹 覆盖/改名/取消），conflict_policy=overwrite|rename 时按策略执行。
func (s *KnowledgeService) MoveDocumentToDir(ctx context.Context, req *v1.MoveDocumentToDirRequest) (*v1.KnowledgeDocument, error) {
	_, col, err := s.requireDocumentRead(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	moved, err := s.uc.MoveVaultDocumentToDir(ctx, req.GetId(), req.GetTargetDir(), req.GetConflictPolicy())
	if err != nil {
		s.lg.Warn("库内移动知识文档失败",
			loggateway.StepID("knowledge.document.move_to_dir_fail"),
			loggateway.Str("doc_id", req.GetId()),
			loggateway.Str("target_dir", req.GetTargetDir()),
			loggateway.Err(err),
		)
		return nil, err
	}
	return toProtoDocument(moved), nil
}

// Search performs a semantic search over a collection.
func (s *KnowledgeService) Search(ctx context.Context, req *v1.SearchRequest) (*v1.SearchResponse, error) {
	timer := prometheus.NewTimer(knowledgeSearchDuration)
	defer timer.ObserveDuration()

	colID := strings.TrimSpace(req.GetCollectionId())
	if colID != "" {
		// C-01: 显式单库检索校验存在性 + 读权限（IDOR 防护）。
		// 留空走 US-14 全库智能路由，无单一 Collection 可校验——
		// 候选集由 FederatedRetriever 经 workspace 过滤的 ListCollections 派生。
		col, err := s.uc.GetCollection(ctx, colID)
		if err != nil {
			return nil, err
		}
		if err := s.assertCollectionAccess(ctx, col); err != nil {
			return nil, err
		}
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "query is required")
	}

	if s.search.Retriever == nil {
		return nil, apierror.Unavailable("KNOWLEDGE", "knowledge retriever not configured")
	}
	q := biz.KnowledgeSearchQuery{
		CollectionID:     req.GetCollectionId(),
		Query:            query,
		TopK:             int(req.GetTopK()),
		MinScore:         req.GetMinScore(),
		FilterJSON:       req.GetFilterJson(),
		RerankCandidates: int(req.GetRerankCandidates()),
		// G3-B7：搜索范围选择器（vault 相对目录前缀；data 层做斜杠归一与
		// LIKE 转义，retriever/hybrid 全链路 struct 值拷贝自动携带）。
		PathPrefix: strings.TrimSpace(req.GetPathPrefix()),
	}
	if req.UseRerank != nil {
		v := req.GetUseRerank()
		q.UseRerank = &v
	}

	var chunks []biz.KnowledgeChunk
	var err error

	rewriteResult := s.rewriteSearchQuery(ctx, query, req.GetRewriteStrategy())
	modeOverride := knowledge.ParseHybridSearchMode(req.GetHybridSearch())

	if q.CollectionID == "" {
		// US-14 检索免选择：collection_id 留空 → 全库智能路由（Route 策略，
		// 名称/描述匹配度 top N=3、阈值 0.3；无匹配自动降级 Broadcast，扇出封顶）。
		if s.search.Federated == nil {
			return nil, apierror.Unavailable("KNOWLEDGE", "federated retriever not configured for collection-free search")
		}
		chunks, err = s.search.Federated.SearchAll(ctx, q, rewriteResult, modeOverride, workspace.ReadableFilterID(ctx))
	} else if s.search.Router != nil {
		chunks, err = s.search.Router.Search(ctx, q, rewriteResult, modeOverride)
	} else {
		chunks, err = s.search.Retriever.Search(ctx, q)
	}
	if err != nil {
		return nil, err
	}

	// P1（2026-08-21）：Search API 默认不跑 RetrievalEvaluator（~1.4s LLM）。
	// 补充检索仅在调用方显式打开评估时执行。
	if req.GetUseEval() {
		var assessor knowledge.ChunkAssessor
		if s.search.Evaluator != nil {
			assessor = s.search.Evaluator
		}
		chunks, err = knowledge.SearchWithEvaluation(ctx, s.search.Retriever, assessor, query, q, chunks, s.lg)
		if err != nil {
			return nil, err
		}
	}

	out := make([]*v1.KnowledgeChunk, 0, len(chunks))
	for _, ch := range chunks {
		out = append(out, &v1.KnowledgeChunk{
			Id:           ch.ID,
			DocId:        ch.DocID,
			CollectionId: ch.CollectionID,
			Content:      ch.Content,
			MetadataJson: ch.MetadataJSON,
			ChunkIndex:   int32(ch.ChunkIndex),
			Score:        ch.Score,
		})
	}
	return &v1.SearchResponse{Chunks: out}, nil
}

func (s *KnowledgeService) rewriteSearchQuery(ctx context.Context, query, strategyRaw string) *knowledge.QueryRewriteResult {
	strategy := knowledge.ParseRewriteStrategy(strategyRaw)
	if strategy == knowledge.RewriteNone {
		return nil
	}
	if s.search.Router == nil {
		return nil
	}
	rewriter := s.search.Router.QueryRewriter()
	if rewriter == nil {
		return nil
	}
	rr, err := rewriter.Rewrite(ctx, query, strategy)
	if err != nil {
		s.lg.Warn("query rewrite failed, using original query",
			loggateway.StepID("knowledge.search.rewrite_fail"),
			loggateway.Err(err),
		)
		return nil
	}
	return rr
}

// GetEmbedderConfig returns redacted embedder settings (EP-KN-01).
func (s *KnowledgeService) GetEmbedderConfig(_ context.Context, _ *v1.GetEmbedderConfigRequest) (*v1.EmbedderConfig, error) {
	return s.embedderConfigProto(), nil
}

// UpdateEmbedderConfig applies runtime embedder settings from admin UI.
func (s *KnowledgeService) UpdateEmbedderConfig(ctx context.Context, req *v1.UpdateEmbedderConfigRequest) (*v1.UpdateEmbedderConfigResponse, error) {
	if s.embedderAdmin == nil {
		return nil, apierror.Internal("KNOWLEDGE", "embedder not configured")
	}
	provider := strings.TrimSpace(req.GetProvider())
	if provider != "" && provider != knowledge.ProviderOpenAI && provider != knowledge.ProviderOllama &&
		provider != knowledge.ProviderGemini && provider != knowledge.ProviderHuggingFace {
		return nil, apierror.BadRequest("KNOWLEDGE", "provider must be openai, ollama, gemini, or huggingface")
	}
	// Persist to DB FIRST. If persistence fails we must NOT update the in-memory
	// embedder, because that would diverge the in-memory config from the DB
	// config and the new setting would be silently lost on the next restart.
	// The previous code logged a warning and returned success, leaving the
	// admin UI with a false "saved" impression.
	if err := PersistKnowledgeEmbed(ctx, s.systemSetting,
		provider, req.GetBaseUrl(), strings.TrimSpace(req.GetApiKey()), req.GetModel(), int(req.GetDim())); err != nil {
		s.lg.Error("写入 system_settings 失败 — 拒绝更新内存态以避免与 DB 不一致",
			loggateway.StepID("knowledge.embedder.persist"),
			loggateway.Err(err),
		)
		return nil, apierror.Internal("KNOWLEDGE", "failed to persist embedder config: %v", err)
	}
	// Persist succeeded — now it is safe to apply the same patch to the
	// in-memory embedder. After this, in-memory and DB state are consistent.
	s.embedderAdmin.Update(provider, req.GetBaseUrl(), req.GetApiKey(), req.GetModel(), int(req.GetDim()))
	return &v1.UpdateEmbedderConfigResponse{Config: s.embedderConfigProto()}, nil
}

func (s *KnowledgeService) embedderConfigProto() *v1.EmbedderConfig {
	if s.embedderAdmin == nil {
		return &v1.EmbedderConfig{}
	}
	provider, baseURL, model, dim, configured, hasAPIKey := s.embedderAdmin.Config()
	return &v1.EmbedderConfig{
		Provider:   provider,
		BaseUrl:    baseURL,
		Model:      model,
		Dim:        int32(dim),
		Configured: configured,
		HasApiKey:  hasAPIKey,
	}
}

// publishKnowledgeIngest publishes a knowledge ingest lifecycle event as a
// v2 SystemNoticeEvent (NOT persisted, WS-only broadcast).
func (s *KnowledgeService) publishKnowledgeIngest(collectionID, docID, status, errMsg string, chunkCount int) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":    "knowledge_ingest",
		"collection_id": collectionID,
		"document_id":   docID,
		"status":        status,
		"error_message": errMsg,
		"chunk_count":   chunkCount,
	}
	msg := "Knowledge document ingest: " + status
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "knowledge_ingest", msg, meta))
}

// --- proto conversion helpers ---

func toProtoCollection(c biz.KnowledgeCollection) *v1.KnowledgeCollection {
	return &v1.KnowledgeCollection{
		Id:             c.ID,
		Name:           c.Name,
		Description:    c.Description,
		EmbeddingModel: c.EmbeddingModel,
		Dim:            int32(c.Dim),
		Status:         c.Status,
		DocumentCount:  int32(c.DocumentCount),
		ChunkCount:     int32(c.ChunkCount),
		Workspace:      c.Workspace,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
		// Vault 字段（V2；空 = 历史 Collection 未迁移）。
		RootPath:   c.RootPath,
		SyncState:  c.SyncState,
		LastSyncAt: c.LastSyncAt,
		// SP1-F 存储后端维度（local / team）。
		VaultBackend: c.VaultBackend,
	}
}

// organizeToMarkdownEnabled 解析 organize_to_markdown 开关：unset 默认 true。
func organizeToMarkdownEnabled(req *v1.IngestDocumentRequest) bool {
	return req.OrganizeToMarkdown == nil || req.GetOrganizeToMarkdown()
}

// isMarkdownSource 判定来源本身已是 Markdown（无需再经 LLM 整理）。
func isMarkdownSource(source, mimeType string) bool {
	if strings.EqualFold(strings.TrimSpace(mimeType), "text/markdown") {
		return true
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// isTextVaultDocument 判定文档是否为纯文本类型（编辑器可预览）。
// 图片/视频/音频/二进制文件返回 false，避免 raw_content 含非法 UTF-8。
func isTextVaultDocument(doc biz.KnowledgeDocument) bool {
	if strings.HasPrefix(doc.MimeType, "text/") ||
		doc.MimeType == "application/json" ||
		doc.MimeType == "application/xml" {
		return true
	}
	switch strings.ToLower(filepath.Ext(doc.Source)) {
	case ".md", ".markdown", ".txt", ".csv", ".json", ".xml", ".html", ".htm":
		return true
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp", ".svg":
		return false
	case ".mp4", ".avi", ".mov", ".mkv", ".webm":
		return false
	case ".mp3", ".wav", ".aac", ".flac", ".ogg":
		return false
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return false
	}
	// 未知扩展名：无 MIME 类型时按文本处理（兜底安全）。
	return doc.MimeType == ""
}

func toProtoDocument(d biz.KnowledgeDocument) *v1.KnowledgeDocument {
	return &v1.KnowledgeDocument{
		Id:               d.ID,
		CollectionId:     d.CollectionID,
		Source:           d.Source,
		MimeType:         d.MimeType,
		SizeBytes:        d.SizeBytes,
		ChunkCount:       int32(d.ChunkCount),
		Status:           d.Status,
		ErrorMessage:     d.ErrorMessage,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
		ExtractSupported: isExtractSupported(d.MimeType),
		Organized:        d.Organized,
		// Vault + 摘要卡字段（P3 列表/hover 卡一级密度）。
		RelPath: d.RelPath,
		Summary: d.Summary,
		Tags:    d.Tags,
		DocType: d.DocType,
	}
}

// extractSupportedMimes is the set of MIME types the backend can currently
// extract and index as searchable text. Used to populate extract_supported in
// API responses so the frontend can show appropriate guidance (REV-D).
// Extend this set when new document readers are registered in trpc-agent-go.
var extractSupportedMimes = map[string]bool{
	"text/plain":         true,
	"text/markdown":      true,
	"text/csv":           true,
	"text/html":          true,
	"text/xml":           true,
	"application/json":   true,
	"application/xml":    true,
	"application/pdf":    true,
	"application/msword": true, // .doc
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true, // .docx
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true, // .xlsx
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true, // .pptx
}

// isExtractSupported returns true when the backend is expected to be able to
// extract searchable text from this MIME type. Falls back to true for text/*
// types that are not in the explicit set (they are plain UTF-8 readable).
func isExtractSupported(mimeType string) bool {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	if extractSupportedMimes[m] {
		return true
	}
	return strings.HasPrefix(m, "text/")
}

func isAllowedIngestMIME(detected string) bool {
	if allowedIngestMIMEs[detected] {
		return true
	}
	return strings.HasPrefix(detected, "text/")
}

// ooxmlIngestMIMETypes 是 OOXML（DOCX/XLSX/PPTX）声明 MIME 白名单。
// 这些文件是 ZIP 容器，http.DetectContentType 返回 application/zip，
// 必须按声明 MIME 或扩展名二次判定，否则 Office 文件被白名单误拒、
// 或普通 zip 因 text/* 兜底被误放行。
var ooxmlIngestMIMETypes = map[string]struct{}{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {},
}

// resolveIngestMIMEAllowed 判定一次上传是否允许入库。
// 非 ZIP 直接走 isAllowedIngestMIME；ZIP 容器仅 OOXML 放行（声明 MIME 优先，扩展名兜底）。
func resolveIngestMIMEAllowed(detected, declared, source string) bool {
	if detected != "application/zip" {
		return isAllowedIngestMIME(detected)
	}
	if _, ok := ooxmlIngestMIMETypes[strings.ToLower(strings.TrimSpace(declared))]; ok {
		return true
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".docx", ".xlsx", ".pptx":
		return true
	}
	return false
}

// extractText 经 ExtractorRegistry 按模态路由提取文本；
// registry 为 nil 时退化 TextExtractor（单测/兼容路径）。
func (s *KnowledgeService) extractText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if s.extractors != nil {
		return s.extractors.Extract(ctx, raw, source, mimeType)
	}
	return knowledge.ExtractDocumentText(ctx, raw, source, mimeType)
}

// resolveIngestCollection 解析入库目标 Collection（US-14 上传免预选）：
// collectionID 非空时按原逻辑校验存在性；为空时懒创建「默认知识库」。
// Embedder 未配置时仍创建词法收件箱（team Vault），不阻断粘贴/拖入。
func (s *KnowledgeService) resolveIngestCollection(ctx context.Context, collectionID string) (biz.KnowledgeCollection, error) {
	if strings.TrimSpace(collectionID) != "" {
		return s.uc.GetCollection(ctx, collectionID)
	}
	model, dim := "", 0
	if s.embedderAdmin != nil {
		_, _, m, d, _, _ := s.embedderAdmin.Config()
		model, dim = m, d
	}
	// C-01: 与 CreateCollection 一致的租户盖章——system 创建共享默认库（ws=""），
	// 租户创建自有默认库，避免懒创建的库被 mutate 检查视为共享只读。
	ws := ""
	if !workspace.IsSystem(ctx) {
		ws = workspace.IDFromContext(ctx)
	}
	return s.uc.EnsureDefaultCollection(ctx, model, dim, ws)
}

// isImageIngest 报告本次入库是否为图片模态（委托 knowledge.IsImageSource）。
func isImageIngest(source, mimeType string) bool {
	return knowledge.IsImageSource(source, mimeType)
}

// mergeIngestMetadata 将用户 metadata 与系统模态标记（modality/extractor）
// 合并为一个 JSON 对象；用户字段与系统键冲突时以系统键为准（血缘可信）。
func mergeIngestMetadata(userJSON, modality, extractor string) (string, error) {
	merged := map[string]any{}
	if s := strings.TrimSpace(userJSON); s != "" {
		if !json.Valid([]byte(s)) {
			return "", apierror.BadRequest(apierror.DomainKnowledge, "metadata_json must be valid JSON")
		}
		if err := json.Unmarshal([]byte(s), &merged); err != nil {
			return "", apierror.BadRequest(apierror.DomainKnowledge, "metadata_json must be a JSON object")
		}
	}
	merged["modality"] = modality
	merged["extractor"] = extractor
	out, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
