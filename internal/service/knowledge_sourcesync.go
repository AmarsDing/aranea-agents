// knowledge_sourcesync.go —— G-38 W5 SourceSync 管理面 custom HTTP（⑦ API 契约）。
//
// 拍板锚点：37-knowledge.design.md §G-38 ⑦（custom JSON + ADR-KN-20261023-G38-API
// 同 PR；理由＝迭代期字段不稳＋ADR-KN-20261020-K 先例＋knowledge.proto 并行脏区
// 叠加 make api 生成物合并成本）。handler 薄化纪律（P4 生效条件四）：本层只做
// 参数解析 / 状态码映射 / workspace 盖章 / 集合访问闸 / recordAudit；业务裁决
// 全部在 knowledgesync.AdminService，晋升 proto 时本层可整体替换而编排层零改。
//
// 端点（11 条 HTTP 路由，7 组契约）：
//
//	GET    /v1/knowledge/connectors              列表（含 auth_paused/error）
//	POST   /v1/knowledge/connectors              创建（从 ctx 盖 workspace）
//	GET    /v1/knowledge/connectors/{id}         详情
//	PATCH  /v1/knowledge/connectors/{id}         声明面局部更新（运行态不可写）
//	DELETE /v1/knowledge/connectors/{id}         软删（级联凭据，保留已摄取文档）
//	PUT    /v1/knowledge/connectors/{id}/credential     写凭据（自动唤醒 auth_paused）
//	GET    /v1/knowledge/connectors/{id}/credential     只返 configured+masked_preview
//	DELETE /v1/knowledge/connectors/{id}/credential     软删单键
//	POST   /v1/knowledge/connectors/{id}/sync           手动触发 → 202/409
//	GET    /v1/knowledge/connectors/{id}/runs?limit     轮次史
//	GET    /v1/knowledge/connectors/{id}/items?status   指纹行（missing 排查）
//
// 鉴权：默认 JWT Filter（路由不经 proto 注册但落同一 kratos http server）；
// 反枚举：connector 层由 AdminService 按 caller workspace 404，集合层由
// assertCollectionAccess/MutateAccess（C-01，Forbidden→404 映射）双重兜底。
// 审计（④）：凭据 Upsert/Delete 必记，Summary 只含 credential_key，**严禁 secret**。
package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
)

// ──────────────────────────────── 请求/响应体（⑦ 契约形状） ────────────────────────────────

// sourceConnectorBody PATCH 语义体：指针字段 nil=未提供（继承既有值）。
type sourceConnectorBody struct {
	Kind                *string         `json:"kind"`
	Name                *string         `json:"name"`
	CollectionID        *string         `json:"collection_id"`
	ConfigJSON          json.RawMessage `json:"config_json"`
	IntervalSeconds     *int            `json:"interval_seconds"`
	Enabled             *bool           `json:"enabled"`
	MaxItemsPerRun      *int            `json:"max_items_per_run"`
	MaxDeletesPerRun    *int            `json:"max_deletes_per_run"`
	MaxFetchBytesPerRun *int64          `json:"max_fetch_bytes_per_run"`
}

type sourceCredentialBody struct {
	CredentialKey string `json:"credential_key"`
	Secret        string `json:"secret"`
	Status        string `json:"status"`
	MetadataJSON  string `json:"metadata_json"`
}

// sourceConnectorView 出参（config_json 已 sanitize；时间 RFC3339，零值空串）。
type sourceConnectorView struct {
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	CollectionID        string `json:"collection_id"`
	ConfigJSON          string `json:"config_json"`
	IntervalSeconds     int    `json:"interval_seconds"`
	Enabled             bool   `json:"enabled"`
	SyncState           string `json:"sync_state"`
	NextRunAt           string `json:"next_run_at"`
	LastRunAt           string `json:"last_run_at"`
	LastSuccessAt       string `json:"last_success_at"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	ErrorCode           string `json:"error_code"`
	ErrorMessage        string `json:"error_message"`
	MaxItemsPerRun      int    `json:"max_items_per_run"`
	MaxDeletesPerRun    int    `json:"max_deletes_per_run"`
	MaxFetchBytesPerRun int64  `json:"max_fetch_bytes_per_run"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type sourceRunView struct {
	ID           string `json:"id"`
	ConnectorID  string `json:"connector_id"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Status       string `json:"status"`
	Created      int    `json:"created"`
	Updated      int    `json:"updated"`
	Unchanged    int    `json:"unchanged"`
	FetchFailed  int    `json:"fetch_failed"`
	Deleted      int    `json:"deleted"`
	MissUnknown  int    `json:"miss_unknown"`
	BytesPulled  int64  `json:"bytes_pulled"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type sourceItemView struct {
	ConnectorID   string `json:"connector_id"`
	ExternalID    string `json:"external_id"`
	DocumentID    string `json:"document_id"`
	Fingerprint   string `json:"fingerprint"`
	ContentHash   string `json:"content_hash"`
	OwnLifecycle  bool   `json:"own_lifecycle"`
	Status        string `json:"status"`
	MissCount     int    `json:"miss_count"`
	LastSeenRunID string `json:"last_seen_run_id"`
	ErrorMessage  string `json:"error_message"`
	FirstSeenAt   string `json:"first_seen_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

// ──────────────────────────────── 端点 ────────────────────────────────

// ServeSourceConnectors GET/POST /v1/knowledge/connectors
func (s *KnowledgeService) ServeSourceConnectors(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	ctx := r.Context()
	switch r.Method {
	case http.MethodGet:
		items, err := admin.ListConnectors(ctx, s.sourceSyncWorkspace(ctx))
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		out := make([]sourceConnectorView, 0, len(items))
		for _, c := range items {
			out = append(out, connectorToView(c))
		}
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": out})
	case http.MethodPost:
		var body sourceConnectorBody
		if !decodeSourceBody(w, r, &body) {
			return
		}
		in, err := connectorInputFromBody(body)
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		// 集合写闸（⑦：绑定共享库 ws='' 仅 system ctx 放行，先于 IDOR 判定）
		if err := s.assertSourceConnectorCollection(ctx, in.CollectionID, true); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		in.WorkspaceID = s.sourceSyncWorkspace(ctx)
		c, err := admin.CreateConnector(ctx, in)
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		recordAudit(ctx, s.mon, bizMonitorEntry("create", c.ID, "kind="+string(c.Kind)+" collection="+c.CollectionID))
		writeKnowledgeJSON(w, http.StatusCreated, connectorToView(c))
	default:
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ServeSourceConnector GET/PATCH/DELETE /v1/knowledge/connectors/{id}
func (s *KnowledgeService) ServeSourceConnector(w http.ResponseWriter, r *http.Request, id string) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	ctx := r.Context()
	callerWS := s.sourceSyncWorkspace(ctx)
	// 先取址（含 404 反枚举）再分派——写闸需集合 ID
	c, err := admin.GetConnector(ctx, id, callerWS)
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, false); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		writeKnowledgeJSON(w, http.StatusOK, connectorToView(c))
	case http.MethodPatch:
		if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, true); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		var body sourceConnectorBody
		if !decodeSourceBody(w, r, &body) {
			return
		}
		merged := mergeConnectorBody(body, c)
		updated, err := admin.UpdateConnector(ctx, id, callerWS, merged)
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		// 集合可能被 PATCH 改绑——新目标库也要过写闸（改绑后旧库不再校验）。
		if updated.CollectionID != c.CollectionID {
			if err := s.assertSourceConnectorCollection(ctx, updated.CollectionID, true); err != nil {
				writeSourceSyncJSONError(w, err)
				return
			}
		}
		recordAudit(ctx, s.mon, bizMonitorEntry("update", id, "collection="+updated.CollectionID))
		writeKnowledgeJSON(w, http.StatusOK, connectorToView(updated))
	case http.MethodDelete:
		if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, true); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		if err := admin.DeleteConnector(ctx, id, callerWS); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		// DELETE 审计含级联凭据语义（④：凭据软删同笔，审计只记 connector 面）
		recordAudit(ctx, s.mon, bizMonitorEntry("delete", id, "cascade=credentials retained_documents=true"))
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ServeSourceConnectorSync POST /v1/knowledge/connectors/{id}/sync → 202/409
func (s *KnowledgeService) ServeSourceConnectorSync(w http.ResponseWriter, r *http.Request, id string) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	ctx := r.Context()
	c, err := admin.GetConnector(ctx, id, s.sourceSyncWorkspace(ctx))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, true); err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	if err := admin.TriggerSync(ctx, id, s.sourceSyncWorkspace(ctx)); err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	recordAudit(ctx, s.mon, bizMonitorEntry("sync", id, "trigger=manual"))
	writeKnowledgeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

// ServeSourceConnectorRuns GET /v1/knowledge/connectors/{id}/runs?limit
func (s *KnowledgeService) ServeSourceConnectorRuns(w http.ResponseWriter, r *http.Request, id string) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	ctx := r.Context()
	c, err := admin.GetConnector(ctx, id, s.sourceSyncWorkspace(ctx))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, false); err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	runs, err := admin.ListRuns(ctx, id, s.sourceSyncWorkspace(ctx), queryLimit(r, 50))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	out := make([]sourceRunView, 0, len(runs))
	for _, run := range runs {
		out = append(out, runToView(run))
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ServeSourceConnectorItems GET /v1/knowledge/connectors/{id}/items?status&limit
func (s *KnowledgeService) ServeSourceConnectorItems(w http.ResponseWriter, r *http.Request, id string) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	ctx := r.Context()
	c, err := admin.GetConnector(ctx, id, s.sourceSyncWorkspace(ctx))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, false); err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	items, err := admin.ListItems(ctx, id, s.sourceSyncWorkspace(ctx), r.URL.Query().Get("status"), queryLimit(r, 100))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	out := make([]sourceItemView, 0, len(items))
	for _, it := range items {
		out = append(out, itemToView(it))
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ServeSourceConnectorCredential PUT/GET/DELETE /v1/knowledge/connectors/{id}/credential
func (s *KnowledgeService) ServeSourceConnectorCredential(w http.ResponseWriter, r *http.Request, id string) {
	admin, ok := s.requireSourceSyncAdmin(w)
	if !ok {
		return
	}
	ctx := r.Context()
	c, err := admin.GetConnector(ctx, id, s.sourceSyncWorkspace(ctx))
	if err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	mutate := r.Method != http.MethodGet
	if err := s.assertSourceConnectorCollection(ctx, c.CollectionID, mutate); err != nil {
		writeSourceSyncJSONError(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		views, err := admin.ListCredentials(ctx, id, s.sourceSyncWorkspace(ctx))
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": views})
	case http.MethodPut:
		var body sourceCredentialBody
		if !decodeSourceBody(w, r, &body) {
			return
		}
		view, err := admin.UpsertCredential(ctx, id, s.sourceSyncWorkspace(ctx), knowledgesync.CredentialInput{
			CredentialKey: body.CredentialKey,
			Secret:        body.Secret,
			Status:        body.Status,
			MetadataJSON:  body.MetadataJSON,
		})
		if err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		// ④ 硬约束：Summary 只记 credential_key，严禁 secret（连掩码都不记）。
		recordAudit(ctx, s.mon, bizMonitorEntry("credentials", id, "upsert key="+view.CredentialKey))
		writeKnowledgeJSON(w, http.StatusOK, view)
	case http.MethodDelete:
		key := strings.TrimSpace(r.URL.Query().Get("credential_key"))
		if key == "" {
			writeSourceSyncJSONError(w, apierror.BadRequest("KNOWLEDGE", "credential_key query parameter is required"))
			return
		}
		if err := admin.DeleteCredential(ctx, id, s.sourceSyncWorkspace(ctx), key); err != nil {
			writeSourceSyncJSONError(w, err)
			return
		}
		recordAudit(ctx, s.mon, bizMonitorEntry("credentials", id, "delete key="+key))
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		writeKnowledgeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// ──────────────────────────────── 内部辅助 ────────────────────────────────

// writeSourceSyncJSONError 本层错误映射：复用 writeKnowledgeJSONError 全表
// 并追加 Unavailable→503（装配守卫语义——「feature 未启用」必须可与 500 内部错
// 区分，前端据此探测灰度）。不动共享映射表：as-of 端点（ADR-KN-20261020-K）
// 的 500 契约钉有其既定期望，本层特性级语义独立演进。
func writeSourceSyncJSONError(w http.ResponseWriter, err error) {
	switch {
	case apierror.IsCode(err, apierror.CodeUnavailable):
		writeKnowledgeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
	default:
		writeKnowledgeJSONError(w, err)
	}
}

// requireSourceSyncAdmin 装配守卫（nil → 503 语义，前端可探测未启用）。
func (s *KnowledgeService) requireSourceSyncAdmin(w http.ResponseWriter) (*knowledgesync.AdminService, bool) {
	if s.sourceSyncAdmin == nil {
		writeSourceSyncJSONError(w, apierror.Unavailable("KNOWLEDGE", "source sync not configured"))
		return nil, false
	}
	return s.sourceSyncAdmin, true
}

// sourceSyncWorkspace 调用方 workspace（系统调用方空串=跨租户可见，create 时
// 亦落空串=共享 connector，⑦ 盖章口径与 CreateDocument owner 同宗）。
func (s *KnowledgeService) sourceSyncWorkspace(ctx context.Context) string {
	if workspace.IsSystem(ctx) {
		return ""
	}
	return workspace.IDFromContext(ctx)
}

// bizMonitorEntry 构造审计条目（action="<verb>.source_connector"，③ 规范形；
// credentials.delete 自动分 warning 级）。
func bizMonitorEntry(verb, connectorID, summary string) biz.AdminAuditEntry {
	return biz.AdminAuditEntry{
		Action:     biz.AuditAction(verb, "source_connector"),
		Resource:   "source_connector",
		ResourceID: connectorID,
		Summary:    summary,
	}
}

// assertSourceConnectorCollection 目标库访问闸（⑦）：共享库（workspace 为空串）
// 写面仅 system ctx 放行；读面走 assertCollectionAccess（共享可读）。
func (s *KnowledgeService) assertSourceConnectorCollection(ctx context.Context, collectionID string, mutate bool) error {
	col, err := s.uc.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	if !mutate {
		return s.assertCollectionAccess(ctx, col)
	}
	if strings.TrimSpace(col.Workspace) == "" && !workspace.IsSystem(ctx) {
		// 先于 assertCollectionMutateAccess 判定（404 反枚举——共享库对租户
		// 不可见，不能暴露「存在但只读」这一层信息）。
		return apierror.NotFound("KNOWLEDGE", "collection not found")
	}
	return s.assertCollectionMutateAccess(ctx, col)
}

// decodeSourceBody 严格解码（未知字段拒：custom JSON 契约无向后兼容包袱）。
func decodeSourceBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeSourceSyncJSONError(w, apierror.BadRequest("KNOWLEDGE", "invalid JSON body"))
		return false
	}
	return true
}

// connectorInputFromBody 创建用：全字段（未提供=零值，缺省由编排层补齐）。
func connectorInputFromBody(b sourceConnectorBody) (knowledgesync.ConnectorInput, error) {
	cfg, err := configJSONString(b.ConfigJSON)
	if err != nil {
		return knowledgesync.ConnectorInput{}, err
	}
	in := knowledgesync.ConnectorInput{
		ConfigJSON: cfg,
		Enabled:    b.Enabled,
	}
	if b.Kind != nil {
		in.Kind = *b.Kind
	}
	if b.Name != nil {
		in.Name = *b.Name
	}
	if b.CollectionID != nil {
		in.CollectionID = *b.CollectionID
	}
	if b.IntervalSeconds != nil {
		in.IntervalSeconds = *b.IntervalSeconds
	}
	if b.MaxItemsPerRun != nil {
		in.MaxItemsPerRun = *b.MaxItemsPerRun
	}
	if b.MaxDeletesPerRun != nil {
		in.MaxDeletesPerRun = *b.MaxDeletesPerRun
	}
	if b.MaxFetchBytesPerRun != nil {
		in.MaxFetchBytesPerRun = *b.MaxFetchBytesPerRun
	}
	return in, nil
}

// mergeConnectorBody PATCH 语义合并：未提供字段继承既有（声明面）；
// 运行态（sync_state/cursor/next_run_at/...）永不可经管理面改写。
func mergeConnectorBody(b sourceConnectorBody, cur bizsync.Connector) knowledgesync.ConnectorInput {
	in := knowledgesync.ConnectorInput{
		Kind:                string(cur.Kind),
		Name:                cur.Name,
		CollectionID:        cur.CollectionID,
		ConfigJSON:          cur.ConfigJSON,
		IntervalSeconds:     cur.IntervalSeconds,
		Enabled:             &cur.Enabled,
		MaxItemsPerRun:      cur.MaxItemsPerRun,
		MaxDeletesPerRun:    cur.MaxDeletesPerRun,
		MaxFetchBytesPerRun: cur.MaxFetchBytesPerRun,
	}
	if b.Kind != nil {
		in.Kind = *b.Kind
	}
	if b.Name != nil {
		in.Name = *b.Name
	}
	if b.CollectionID != nil {
		in.CollectionID = *b.CollectionID
	}
	// 空对象与空串/null 同档：视为未提供（PATCH 语义下显式 "{}" 不是清空意图——
	// shared_dir config 必填 path，清空必过不了 ValidateConfig，按继承处理）。
	if cfg, err := configJSONString(b.ConfigJSON); err == nil && cfg != "" && cfg != "{}" {
		in.ConfigJSON = cfg
	}
	if b.IntervalSeconds != nil {
		in.IntervalSeconds = *b.IntervalSeconds
	}
	if b.Enabled != nil {
		in.Enabled = b.Enabled
	}
	if b.MaxItemsPerRun != nil {
		in.MaxItemsPerRun = *b.MaxItemsPerRun
	}
	if b.MaxDeletesPerRun != nil {
		in.MaxDeletesPerRun = *b.MaxDeletesPerRun
	}
	if b.MaxFetchBytesPerRun != nil {
		in.MaxFetchBytesPerRun = *b.MaxFetchBytesPerRun
	}
	return in
}

// configJSONString config_json 双形态（对象/字符串）归一为文本：
// 对象→字节原样；字符串→解引号值。空/缺失→空串（由调用方决定继承或拒绝）。
func configJSONString(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", apierror.BadRequest("KNOWLEDGE", "config_json must be a JSON object or string")
		}
		return strings.TrimSpace(s), nil
	}
	return trimmed, nil
}

func queryLimit(r *http.Request, def int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func connectorToView(c bizsync.Connector) sourceConnectorView {
	return sourceConnectorView{
		ID:                  c.ID,
		Kind:                string(c.Kind),
		Name:                c.Name,
		CollectionID:        c.CollectionID,
		ConfigJSON:          c.ConfigJSON,
		IntervalSeconds:     c.IntervalSeconds,
		Enabled:             c.Enabled,
		SyncState:           string(c.SyncState),
		NextRunAt:           timeStr(c.NextRunAt),
		LastRunAt:           timeStr(c.LastRunAt),
		LastSuccessAt:       timeStr(c.LastSuccessAt),
		ConsecutiveFailures: c.ConsecutiveFailures,
		ErrorCode:           string(c.ErrorCode),
		ErrorMessage:        c.ErrorMessage,
		MaxItemsPerRun:      c.MaxItemsPerRun,
		MaxDeletesPerRun:    c.MaxDeletesPerRun,
		MaxFetchBytesPerRun: c.MaxFetchBytesPerRun,
		CreatedAt:           timeStr(c.CreatedAt),
		UpdatedAt:           timeStr(c.UpdatedAt),
	}
}

func runToView(r bizsync.Run) sourceRunView {
	return sourceRunView{
		ID:           r.ID,
		ConnectorID:  r.ConnectorID,
		StartedAt:    timeStr(r.StartedAt),
		FinishedAt:   timeStr(r.FinishedAt),
		Status:       string(r.Status),
		Created:      r.Created,
		Updated:      r.Updated,
		Unchanged:    r.Unchanged,
		FetchFailed:  r.FetchFailed,
		Deleted:      r.Deleted,
		MissUnknown:  r.MissUnknown,
		BytesPulled:  r.BytesPulled,
		ErrorCode:    string(r.ErrorCode),
		ErrorMessage: r.ErrorMessage,
	}
}

func itemToView(it bizsync.Item) sourceItemView {
	return sourceItemView{
		ConnectorID:   it.ConnectorID,
		ExternalID:    it.ExternalID,
		DocumentID:    it.DocumentID,
		Fingerprint:   it.Fingerprint,
		ContentHash:   it.ContentHash,
		OwnLifecycle:  it.OwnLifecycle,
		Status:        string(it.Status),
		MissCount:     it.MissCount,
		LastSeenRunID: it.LastSeenRunID,
		ErrorMessage:  it.ErrorMessage,
		FirstSeenAt:   timeStr(it.FirstSeenAt),
		LastSeenAt:    timeStr(it.LastSeenAt),
	}
}

func timeStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
