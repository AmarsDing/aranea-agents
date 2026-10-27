// fakes_test.go —— W2 Runner 钉阵内存替身（无 PG：biz 仓口契约的内存语义镜）。
// CAS 语义（TryClaimRun/FinalizeRunWithClaim/ReclaimZombieRuns）在此按
// SQL 谓词逐字镜像——PG 真值钉留在挂起批（knowledge_source_sync_repo_test.go），
// 双阵营语义一致性由本文件谓词与 repo SQL 同构保证。
package sourcesync

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
)

var errFakeInfra = errors.New("fake infra failure")

// ──────────────────────────────── ConnectorRepo 内存镜 ────────────────────────────────

type memConnectorRow struct {
	c       bizsync.Connector
	deleted bool
}

type memConnectorRepo struct {
	mu   sync.Mutex
	rows map[string]*memConnectorRow
	// finalizeLoss 钉：FinalizedRunWithClaim 假装失地
	forceFinalizeMiss bool
	// 故障注入（评审 W2 修 #6）：仓口基础设施失败分支钉
	getErr      error
	claimErr    error
	finalizeErr error
	// onSoftDelete 级联钩子（W5：镜像 repo 同事务级联软删凭据——fixture 侧
	// 挂 memCredentialRepo.cascadeDelete，钉级联语义不进编排层错位）
	onSoftDelete func(id string)
	// fencing 钉同步点：HeartbeatClaim 首次返回失地即关闭
	lostSeen chan struct{}
	lostOnce sync.Once
}

func newMemConnectorRepo() *memConnectorRepo {
	return &memConnectorRepo{rows: map[string]*memConnectorRow{}}
}

func (m *memConnectorRepo) seed(c bizsync.Connector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[c.ID] = &memConnectorRow{c: c}
}

func (m *memConnectorRepo) get(id string) bizsync.Connector {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rows[id].c
}

func (m *memConnectorRepo) CreateConnector(_ context.Context, c bizsync.Connector) error {
	m.seed(c)
	return nil
}

func (m *memConnectorRepo) GetConnector(_ context.Context, id string) (bizsync.Connector, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return bizsync.Connector{}, false, m.getErr
	}
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return bizsync.Connector{}, false, nil
	}
	return r.c, true, nil
}

// UpdateConnector SQL 谓词逐字镜像：W5 管理面 CAS——id ∧ 未软删 ∧
// run_token 恒空串守卫（在途轮原子拒管理写，防全行 replace 吞 active claim）。
func (m *memConnectorRepo) UpdateConnector(_ context.Context, c bizsync.Connector) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[c.ID]
	if !ok || r.deleted || r.c.RunToken != "" {
		return false, nil
	}
	m.rows[c.ID].c = c
	return true, nil
}

func (m *memConnectorRepo) SoftDeleteConnector(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return false, nil
	}
	r.deleted = true
	r.c.RunToken = ""
	if m.onSoftDelete != nil {
		// 镜像 repo 同事务级联（⑦ DELETE 默认级联软删本 connector 全部凭据）
		m.onSoftDelete(id)
	}
	return true, nil
}

func (m *memConnectorRepo) ListConnectorsByWorkspace(_ context.Context, workspaceID string) ([]bizsync.Connector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []bizsync.Connector
	for _, r := range m.rows {
		if !r.deleted && r.c.WorkspaceID == workspaceID {
			out = append(out, r.c)
		}
	}
	return out, nil
}

func (m *memConnectorRepo) ListDueConnectors(_ context.Context, now time.Time, limit int) ([]bizsync.Connector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []bizsync.Connector
	for _, r := range m.rows {
		c := r.c
		if r.deleted || !c.Enabled || c.NextRunAt.IsZero() || c.NextRunAt.After(now) {
			continue
		}
		if c.SyncState == bizsync.SyncStateRunning || c.SyncState == bizsync.SyncStateAuthPaused {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NextRunAt.Before(out[j].NextRunAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// TryClaimRun 谓词镜（SQL 同构）：enabled ∧ 未软删 ∧（idle|error ∨ running∧updated_at 过期）。
// 过期边界严格镜像 SQL `updated_at < now-leaseTTL`：等值即未过期（评审 W2 修 #12）。
func (m *memConnectorRepo) TryClaimRun(_ context.Context, id, runToken string, leaseTTL time.Duration, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimErr != nil {
		return false, m.claimErr
	}
	r, ok := m.rows[id]
	if !ok || r.deleted || !r.c.Enabled || runToken == "" {
		return false, nil
	}
	switch r.c.SyncState {
	case bizsync.SyncStateIdle, bizsync.SyncStateError:
		// 新轮守旧轮
	case bizsync.SyncStateRunning:
		if r.c.UpdatedAt.IsZero() || !r.c.UpdatedAt.Add(leaseTTL).Before(now) {
			return false, nil
		}
	default:
		return false, nil
	}
	r.c.RunToken = runToken
	r.c.SyncState = bizsync.SyncStateRunning
	r.c.UpdatedAt = now
	return true, nil
}

func (m *memConnectorRepo) HeartbeatClaim(_ context.Context, id, runToken string, now time.Time) (bool, error) {
	m.mu.Lock()
	r, ok := m.rows[id]
	if !ok || r.deleted || r.c.RunToken != runToken || r.c.SyncState != bizsync.SyncStateRunning {
		m.mu.Unlock()
		if m.lostSeen != nil {
			m.lostOnce.Do(func() { close(m.lostSeen) })
		}
		return false, nil
	}
	r.c.UpdatedAt = now
	m.mu.Unlock()
	return true, nil
}

func (m *memConnectorRepo) FinalizeRunWithClaim(_ context.Context, c bizsync.Connector, expectedRunToken string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.finalizeErr != nil {
		return false, m.finalizeErr
	}
	if m.forceFinalizeMiss {
		return false, nil
	}
	r, ok := m.rows[c.ID]
	if !ok || r.deleted || r.c.RunToken != expectedRunToken {
		return false, nil
	}
	r.c.SyncState = c.SyncState
	r.c.Cursor = c.Cursor
	r.c.NextRunAt = c.NextRunAt
	r.c.LastRunAt = c.LastRunAt
	r.c.LastSuccessAt = c.LastSuccessAt
	r.c.ConsecutiveFailures = c.ConsecutiveFailures
	r.c.ErrorCode = c.ErrorCode
	r.c.ErrorMessage = c.ErrorMessage
	r.c.RunToken = ""
	// 服务器钟镜（评审 W2 修 #11）：SQL 侧 updated_at=now() 而非入参钟。
	r.c.UpdatedAt = time.Now()
	return true, nil
}

func (m *memConnectorRepo) ReclaimZombieRuns(_ context.Context, now time.Time, leaseTTL time.Duration) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if r.deleted || r.c.SyncState != bizsync.SyncStateRunning {
			continue
		}
		// 严格 < 边界与 SQL 同（评审 W2 修 #12）
		if r.c.UpdatedAt.IsZero() || !r.c.UpdatedAt.Add(leaseTTL).Before(now) {
			continue
		}
		r.c.SyncState = bizsync.SyncStateIdle
		r.c.RunToken = ""
		r.c.ConsecutiveFailures++
		r.c.UpdatedAt = now
		n++
	}
	return n, nil
}

// TriggerConnectorSync SQL 谓词逐字镜像：id ∧ 未软删 ∧ enabled ∧ run_token 恒空串
// ∧ sync_state<>'auth_paused' → next_run_at=now、updated_at=now。
func (m *memConnectorRepo) TriggerConnectorSync(_ context.Context, id string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok || r.deleted || !r.c.Enabled || r.c.RunToken != "" || r.c.SyncState == bizsync.SyncStateAuthPaused {
		return false, nil
	}
	r.c.NextRunAt = now
	r.c.UpdatedAt = now
	return true, nil
}

// WakeConnectorFromAuthPaused SQL 谓词逐字镜像：id ∧ 未软删 ∧
// sync_state='auth_paused' → idle+error_code 清零+consecutive_failures=0。
func (m *memConnectorRepo) WakeConnectorFromAuthPaused(_ context.Context, id string, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok || r.deleted || r.c.SyncState != bizsync.SyncStateAuthPaused {
		return false, nil
	}
	r.c.SyncState = bizsync.SyncStateIdle
	r.c.ErrorCode = bizsync.ErrCodeNone
	r.c.ConsecutiveFailures = 0
	r.c.UpdatedAt = now
	return true, nil
}

// steal fencing 钉助手：模拟另一属主 CAS 夺权（token 改写 + 租约面刷新）。
func (m *memConnectorRepo) steal(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.rows[id]; ok {
		r.c.RunToken = "ss:tok:stolen"
		r.c.SyncState = bizsync.SyncStateRunning
		r.c.UpdatedAt = time.Now()
	}
}

// ──────────────────────────────── ItemRepo 内存镜 ────────────────────────────────

func memItemKey(connectorID, externalID string) string { return connectorID + "|" + externalID }

type memItemRepo struct {
	mu   sync.Mutex
	rows map[string]bizsync.Item
	// 故障注入（评审 W2 修 #6）：仓口基础设施失败分支钉
	getErr        error
	upsertSeenErr error
	markAbsentErr error
	listErr       error
}

func newMemItemRepo() *memItemRepo { return &memItemRepo{rows: map[string]bizsync.Item{}} }

func (m *memItemRepo) seed(it bizsync.Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[memItemKey(it.ConnectorID, it.ExternalID)] = it
}

func (m *memItemRepo) get(connectorID, externalID string) (bizsync.Item, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.rows[memItemKey(connectorID, externalID)]
	return it, ok
}

// UpsertItemSeen 语义镜③-2：出席即 active/miss=0/error 清空/deleted 复位；
// 绑定列（document_id/content_hash/own_lifecycle/first_seen）INSERT 生效、CONFLICT 保留。
func (m *memItemRepo) UpsertItemSeen(_ context.Context, connectorID, externalID, documentID, fingerprint string, ownLifecycle bool, lastSeenRunID string, seenAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertSeenErr != nil {
		return m.upsertSeenErr
	}
	k := memItemKey(connectorID, externalID)
	if it, ok := m.rows[k]; ok {
		it.Fingerprint = fingerprint
		it.Status = bizsync.ItemStatusActive
		it.MissCount = 0
		it.ErrorMessage = ""
		it.DeletedAt = time.Time{}
		it.LastSeenRunID = lastSeenRunID
		it.LastSeenAt = seenAt
		m.rows[k] = it
		return nil
	}
	m.rows[k] = bizsync.Item{
		ConnectorID: connectorID, ExternalID: externalID, DocumentID: documentID,
		Fingerprint: fingerprint, OwnLifecycle: ownLifecycle, Status: bizsync.ItemStatusActive,
		LastSeenRunID: lastSeenRunID, FirstSeenAt: seenAt, LastSeenAt: seenAt,
	}
	return nil
}

func (m *memItemRepo) GetItem(_ context.Context, connectorID, externalID string) (bizsync.Item, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return bizsync.Item{}, false, m.getErr
	}
	it, ok := m.rows[memItemKey(connectorID, externalID)]
	return it, ok, nil
}

func (m *memItemRepo) SetItemDocument(_ context.Context, connectorID, externalID, documentID string, ownLifecycle bool) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memItemKey(connectorID, externalID)
	it, ok := m.rows[k]
	if !ok {
		return false, nil
	}
	it.DocumentID = documentID
	it.OwnLifecycle = ownLifecycle
	m.rows[k] = it
	return true, nil
}

func (m *memItemRepo) SetItemContentHash(_ context.Context, connectorID, externalID, contentHash string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memItemKey(connectorID, externalID)
	it, ok := m.rows[k]
	if !ok {
		return false, nil
	}
	it.ContentHash = contentHash
	m.rows[k] = it
	return true, nil
}

func (m *memItemRepo) SetItemError(_ context.Context, connectorID, externalID, message string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memItemKey(connectorID, externalID)
	it, ok := m.rows[k]
	if !ok {
		return false, nil
	}
	it.ErrorMessage = message
	m.rows[k] = it
	return true, nil
}

func (m *memItemRepo) ListItemsByStatus(_ context.Context, connectorID string, status bizsync.ItemStatus, limit int) ([]bizsync.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []bizsync.Item
	for _, it := range m.rows {
		if it.ConnectorID == connectorID && it.Status == status {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalID < out[j].ExternalID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memItemRepo) CountActiveItems(_ context.Context, connectorID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, it := range m.rows {
		if it.ConnectorID == connectorID && it.Status == bizsync.ItemStatusActive {
			n++
		}
	}
	return n, nil
}

func (m *memItemRepo) MarkAbsentItemsMissing(_ context.Context, connectorID, runToken string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markAbsentErr != nil {
		return 0, m.markAbsentErr
	}
	n := 0
	for k, it := range m.rows {
		if it.ConnectorID != connectorID || it.Status != bizsync.ItemStatusActive || it.LastSeenRunID == runToken {
			continue
		}
		it.Status = bizsync.ItemStatusMissing
		it.MissCount++
		m.rows[k] = it
		n++
	}
	return n, nil
}

func (m *memItemRepo) MarkItemDeleted(_ context.Context, connectorID, externalID string, when time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := memItemKey(connectorID, externalID)
	it, ok := m.rows[k]
	if !ok {
		return false, nil
	}
	it.Status = bizsync.ItemStatusDeleted
	it.DeletedAt = when
	m.rows[k] = it
	return true, nil
}

func (m *memItemRepo) UnbindItemsByDocumentID(_ context.Context, documentID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for k, it := range m.rows {
		if it.DocumentID != documentID || it.Status == bizsync.ItemStatusDeleted {
			continue
		}
		it.DocumentID = ""
		it.OwnLifecycle = false
		m.rows[k] = it
		n++
	}
	return n, nil
}

// ──────────────────────────────── RunRepo 内存镜 ────────────────────────────────

type memRunRepo struct {
	mu   sync.Mutex
	runs []bizsync.Run
	// 故障注入（评审 W2 修 #6）：审计行插入失败仍续收轮钉
	insertErr error
}

func (m *memRunRepo) InsertRunWithRetention(_ context.Context, run bizsync.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.insertErr != nil {
		return m.insertErr
	}
	m.runs = append(m.runs, run)
	return nil
}

func (m *memRunRepo) ListRuns(_ context.Context, connectorID string, limit int) ([]bizsync.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []bizsync.Run
	for _, r := range m.runs {
		if r.ConnectorID != connectorID {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memRunRepo) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.runs)
}

func (m *memRunRepo) last() bizsync.Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.runs) == 0 {
		return bizsync.Run{}
	}
	return m.runs[len(m.runs)-1]
}

// ──────────────────────────────── CredentialRepo 内存镜（W5） ────────────────────────────────

type memCredentialRow struct {
	c       bizsync.Credential
	deleted bool
}

func memCredentialKey(connectorID, credentialKey string) string {
	return connectorID + "|" + credentialKey
}

type memCredentialRepo struct {
	mu   sync.Mutex
	rows map[string]*memCredentialRow
	// 故障注入：仓口基础设施失败分支钉
	upsertErr error
	getErr    error
}

func newMemCredentialRepo() *memCredentialRepo {
	return &memCredentialRepo{rows: map[string]*memCredentialRow{}}
}

// UpsertCredential partial-arbiter 谓词逐字镜像（idx_src_credential_active：
// (connector_id, credential_key) WHERE deleted_at 恒空串）：活跃冲突原位更新，
// 软删墓碑行不仲裁——先行软删再创=新行；SecretRef 空串硬拒（拒写闸对齐）。
func (m *memCredentialRepo) UpsertCredential(_ context.Context, c bizsync.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.upsertErr != nil {
		return m.upsertErr
	}
	if c.ID == "" || c.ConnectorID == "" || c.CredentialKey == "" || c.SecretRef == "" {
		return errors.New("mem: credential id/connector/key/secret_ref required")
	}
	k := memCredentialKey(c.ConnectorID, c.CredentialKey)
	if r, ok := m.rows[k]; ok && !r.deleted {
		r.c.Status = c.Status
		r.c.SecretRef = c.SecretRef
		r.c.MetadataJSON = c.MetadataJSON
		r.c.UpdatedAt = time.Now()
		return nil
	}
	m.rows[k] = &memCredentialRow{c: c}
	return nil
}

func (m *memCredentialRepo) GetCredential(_ context.Context, connectorID, credentialKey string) (bizsync.Credential, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return bizsync.Credential{}, false, m.getErr
	}
	r, ok := m.rows[memCredentialKey(connectorID, credentialKey)]
	if !ok || r.deleted {
		return bizsync.Credential{}, false, nil
	}
	return r.c, true, nil
}

func (m *memCredentialRepo) SoftDeleteCredential(_ context.Context, connectorID, credentialKey string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[memCredentialKey(connectorID, credentialKey)]
	if !ok || r.deleted {
		return false, nil
	}
	r.deleted = true
	return true, nil
}

// cascadeDelete connector 级联软删（镜像 repo 同事务级联：整 connector
// 凭据全软删；fixture 经 memConnectorRepo.onSoftDelete 缝合调用）。
func (m *memCredentialRepo) cascadeDelete(connectorID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, r := range m.rows {
		if !r.deleted && strings.HasPrefix(k, connectorID+"|") {
			r.deleted = true
		}
	}
}

func (m *memCredentialRepo) ListCredentialsByConnector(_ context.Context, connectorID string) ([]bizsync.Credential, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []bizsync.Credential
	for k, r := range m.rows {
		if !r.deleted && strings.HasPrefix(k, connectorID+"|") {
			out = append(out, r.c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CredentialKey < out[j].CredentialKey })
	return out, nil
}

// ──────────────────────────────── Walker / Applier / Locker / Frames ────────────────────────────────

type fakeWalker struct {
	kind       bizsync.Kind
	snaps      []Snapshot
	walkErr    error
	nextCursor string
	fetchErrs  map[string]error
	fetchSize  map[string]int64
	verdicts   map[string]bizsync.DeletionVerdict
	walkCalls  int
	fetchCalls map[string]int
	// 游标连续性钉（评审 W2 修 #7）：每次 Walk 收到的 prev 原样记录
	seenPrevs []CursorState
	// fencing 钉钩子：第 idx 次 emit 前触发（可窃取 claim）；ConfirmDeleted 内触发；
	// walk 完满即将返回新游标前触发（pre-⑥-3 闸钉同步点）
	beforeEmit func(idx int)
	onConfirm  func(externalID string)
	onWalkEnd  func()
}

func (w *fakeWalker) Kind() bizsync.Kind { return w.kind }

// Walk 契约忠实替身：emit 出错即中止且游标返回 prev（游标不动）。
func (w *fakeWalker) Walk(_ context.Context, _ bizsync.Connector, prev CursorState, emit func(Snapshot) error) (CursorState, error) {
	w.walkCalls++
	w.seenPrevs = append(w.seenPrevs, prev)
	if w.walkErr != nil {
		return prev, w.walkErr
	}
	for i, s := range w.snaps {
		if w.beforeEmit != nil {
			w.beforeEmit(i)
		}
		if err := emit(s); err != nil {
			return prev, err
		}
	}
	if w.onWalkEnd != nil {
		w.onWalkEnd()
	}
	return w.nextCursor, nil
}

func (w *fakeWalker) Fetch(_ context.Context, _ bizsync.Connector, externalID string) (*Payload, error) {
	if w.fetchCalls == nil {
		w.fetchCalls = map[string]int{}
	}
	w.fetchCalls[externalID]++
	if err := w.fetchErrs[externalID]; err != nil {
		return nil, err
	}
	size := w.fetchSize[externalID]
	if size == 0 {
		size = 16
	}
	body := io.NopCloser(strings.NewReader(strings.Repeat("x", int(size))))
	return &Payload{Body: body, MIME: "text/plain", SizeBytes: size}, nil
}

func (w *fakeWalker) ConfirmDeleted(_ context.Context, _ bizsync.Connector, externalID string) bizsync.DeletionVerdict {
	if w.onConfirm != nil {
		w.onConfirm(externalID)
	}
	if v, ok := w.verdicts[externalID]; ok {
		return v
	}
	return bizsync.VerdictUnknown
}

type fakeApplier struct {
	upsertErrs  map[string]error
	deleteErr   error
	upsertCalls []string
	deleteCalls []bizsync.Item
	outcomeFn   func(snap Snapshot) ApplyOutcome
}

func (a *fakeApplier) Upsert(_ context.Context, _ bizsync.Connector, snap Snapshot, payload *Payload) (ApplyOutcome, error) {
	if payload != nil && payload.Body != nil {
		_ = payload.Body.Close() // 所有权契约：Applier 关闭
	}
	a.upsertCalls = append(a.upsertCalls, snap.ExternalID)
	if err := a.upsertErrs[snap.ExternalID]; err != nil {
		return ApplyOutcome{}, err
	}
	if a.outcomeFn != nil {
		return a.outcomeFn(snap), nil
	}
	return ApplyOutcome{
		DocumentID:   "doc:" + snap.ExternalID,
		OwnLifecycle: true,
		ContentHash:  "hash:" + snap.Fingerprint,
	}, nil
}

func (a *fakeApplier) Delete(_ context.Context, _ bizsync.Connector, item bizsync.Item) error {
	a.deleteCalls = append(a.deleteCalls, item)
	return a.deleteErr
}

type fakeLocker struct {
	ok       bool
	err      error
	acquired int
	released int
}

func (l *fakeLocker) TryAcquire(_ context.Context, _ string) (func(), bool, error) {
	if l.err != nil {
		return nil, false, l.err
	}
	if !l.ok {
		return nil, false, nil
	}
	l.acquired++
	return func() { l.released++ }, true, nil
}

type fakeFrames struct {
	mu       sync.Mutex
	started  []RunEvent
	finished []RunEvent
}

func (f *fakeFrames) OnRunStarted(_ context.Context, ev RunEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, ev)
}

func (f *fakeFrames) OnRunFinished(_ context.Context, ev RunEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, ev)
}

// ──────────────────────────────── 场景装配 ────────────────────────────────

var fixedClock = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

type runnerFixture struct {
	conns  *memConnectorRepo
	items  *memItemRepo
	runs   *memRunRepo
	walker *fakeWalker
	appl   *fakeApplier
	locker *fakeLocker
	frames *fakeFrames
	runner *Runner
	conn   bizsync.Connector
}

func newRunnerFixture(t *testing.T, mutate func(*bizsync.Connector)) *runnerFixture {
	t.Helper()
	f := &runnerFixture{
		conns:  newMemConnectorRepo(),
		items:  newMemItemRepo(),
		runs:   &memRunRepo{},
		appl:   &fakeApplier{},
		locker: &fakeLocker{ok: true},
		frames: &fakeFrames{},
	}
	conn := bizsync.Connector{
		ID: "src:conn:1", Kind: bizsync.KindSharedDir, Name: "c1",
		WorkspaceID: "ws1", CollectionID: "col1",
		IntervalSeconds:     900,
		Enabled:             true,
		SyncState:           bizsync.SyncStateIdle,
		NextRunAt:           fixedClock.Add(-time.Minute),
		MaxItemsPerRun:      500,
		MaxDeletesPerRun:    200,
		MaxFetchBytesPerRun: 512 << 20,
		CreatedAt:           fixedClock.Add(-time.Hour),
		UpdatedAt:           fixedClock.Add(-time.Hour),
	}
	if mutate != nil {
		mutate(&conn)
	}
	f.conn = conn
	f.conns.seed(conn)
	f.walker = &fakeWalker{kind: conn.Kind, nextCursor: "cur:n+1"}
	switch conn.Kind {
	case bizsync.KindNotion:
		f.walker.kind = bizsync.KindNotion
	}
	f.runner = NewRunner(Deps{
		Connectors: f.conns, Items: f.items, Runs: f.runs,
		Locker:   f.locker,
		Walkers:  map[bizsync.Kind]Walker{f.walker.Kind(): f.walker},
		Appliers: map[bizsync.Kind]Applier{f.walker.Kind(): f.appl},
		Now:      func() time.Time { return fixedClock },
		Frames:   f.frames,
	})
	return f
}
