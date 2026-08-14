package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// Channel status constants — replace magic strings across the module.
const (
	ChannelStatusActive  = "active"
	ChannelStatusDeleted = "deleted"
	ChannelStatusError   = "error"
	ChannelStatusPending = "pending"
)

// Channel mirrors legacy PlatformResource for resource "channels".
type Channel struct {
	ID           string
	Resource     string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ParentID     string
	Level        string
	AgentID      string
	Provider     string
	Model        string
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces, e.g., system builtins);
	// non-empty = tenant-private (visible only to owning workspace).
	WorkspaceID string
}

type ChannelTypeItem struct {
	Type             string
	Label            string
	Description      string
	Group            string
	ReceiveModes     []string
	Icon             string
	Bundled          bool
	SupportsTest     bool
	SupportsWebhook  bool
	ConfigSchema     map[string]any
	CredentialSchema map[string]any
	UIHints          map[string]any
	SortOrder        int
}

type ChannelCredential struct {
	ID            string
	ChannelID     string
	CredentialKey string
	Status        string
	SecretRef     string
	MetadataJSON  string
	CreatedAt     string
	UpdatedAt     string
	DeletedAt     string
	Configured    bool
	MaskedPreview string
}

type ChannelCredentialInput struct {
	CredentialKey string
	Secret        string
	SecretRef     string
	Status        string
	MetadataJSON  string
}

type ChannelDelivery struct {
	ID             string
	ChannelID      string
	AgentID        string
	IdempotencyKey string
	Status         string
	PayloadJSON    string
	ErrorMessage   string
	CreatedAt      string
	UpdatedAt      string
}

// ChannelUpdateOptions uses pointer fields to distinguish "not provided" (nil)
// from "explicitly set to zero value". This solves the bool zero-value problem
// where Enabled=false cannot be differentiated from "caller didn't set Enabled".
type ChannelUpdateOptions struct {
	Key          *string
	Name         *string
	Description  *string
	Enabled      *bool
	SortOrder    *int
	AgentID      *string
	Provider     *string
	Model        *string
	ConfigJSON   *string
	MetadataJSON *string
	Status       *string
}

type ChannelTestResult struct {
	OK      bool
	Status  string
	Message string
	Details map[string]any
}

// ChannelLiveTester performs live connectivity tests for a specific channel type.
// Implementations are registered per channel type and called by ChannelService after
// the structural EvaluateChannelTest passes.
// Stability:evolving
type ChannelLiveTester interface {
	TestLive(ctx context.Context, configJSON string, credentials []ChannelCredential) ChannelTestResult
}

// ChannelLiveTesterFunc is a convenience adapter for single-function testers.
type ChannelLiveTesterFunc func(ctx context.Context, configJSON string, credentials []ChannelCredential) ChannelTestResult

func (f ChannelLiveTesterFunc) TestLive(ctx context.Context, configJSON string, credentials []ChannelCredential) ChannelTestResult {
	return f(ctx, configJSON, credentials)
}

// Stability:stable
// ChannelListQuery is the pagination/filter input for admin channel lists.
type ChannelListQuery struct {
	Search string
	Type   string // channel config.type (matched via config_json contains)
	Status string // row status, or "enabled"/"disabled" for the enabled flag
	Limit  int
	Offset int
}

// ChannelListResult is a page of channels plus the filter-scoped total.
type ChannelListResult struct {
	Items  []Channel
	Total  int
	Limit  int
	Offset int
}

// Stability:evolving
type ChannelReader interface {
	List(ctx context.Context) ([]Channel, error)
	ListPaged(ctx context.Context, q ChannelListQuery) (ChannelListResult, error)
	Get(ctx context.Context, id string) (Channel, error)
	GetByKey(ctx context.Context, channelKey string) (Channel, error)
}

// Stability:evolving
type ChannelWriter interface {
	Create(ctx context.Context, row Channel) (Channel, error)
	Update(ctx context.Context, row Channel) (Channel, error)
	Delete(ctx context.Context, id string) error
}

// Stability:stable
type ChannelCredentialRepo interface {
	ListCredentials(ctx context.Context, channelID string) ([]ChannelCredential, error)
	UpsertCredential(ctx context.Context, cred ChannelCredential) (ChannelCredential, error)
	DeleteCredential(ctx context.Context, channelID, credentialKey string) error
}

// ChannelDeliveryReader provides read-only access to channel deliveries.
// Stability:stable
type ChannelDeliveryReader interface {
	ListDeliveries(ctx context.Context, channelID string, limit int) ([]ChannelDelivery, error)
	ListPendingDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error)
}

// ChannelDeliveryRepo provides full access to channel deliveries.
// Embeds ChannelDeliveryReader for convenience.
// Stability:stable
type ChannelDeliveryRepo interface {
	ChannelDeliveryReader
	AddDelivery(ctx context.Context, d ChannelDelivery) (ChannelDelivery, error)
	// AddDeliveryIfNotExists atomically inserts a delivery row, returning (row, true) on insert
	// or (existing, false) when the idempotency_key already exists for the same channel_id.
	// The unique constraint is on (channel_id, idempotency_key).
	AddDeliveryIfNotExists(ctx context.Context, d ChannelDelivery) (ChannelDelivery, bool, error)
	UpdateDelivery(ctx context.Context, d ChannelDelivery) error
	// ClaimPendingDeliveries atomically claims due outbound rows for one worker
	// instance (pending / due retry / sending whose lease expired). Multi-instance
	// safe via Postgres FOR UPDATE SKIP LOCKED + status CAS.
	ClaimPendingDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error)
}

// TECH-DEBT(COG): 复合 usecase 方法数>5, 上限=5 —— ChannelUsecase 同时承载 channel
// CRUD、credential、delivery、health check、peer session、test 执行等职责，
// 后续应按职责拆分（AS-COG-01 / DB-DEBT-05 同类）。
type ChannelUsecase struct {
	reader                 ChannelReader
	writer                 ChannelWriter
	credentials            ChannelCredentialRepo
	deliveries             ChannelDeliveryRepo
	peerUsecase            *ChannelPeerUsecase
	agents                 AgentRepository
	teams                  TeamReader
	crypto                 *CredentialCrypto
	healthCheckConcurrency int
	lg                     loggateway.Logger
	txProvider             ChannelTxProvider
}

// ChannelTxProvider provides transactional execution for atomic channel +
// credential writes. When set via SetTxProvider, Create/Update wrap the
// channel write and credential upsert in a single transaction so a credential
// failure rolls back the channel row (red line #24).
// Stability:stable
type ChannelTxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

const defaultHealthCheckConcurrency = 8

func NewChannelUsecase(
	reader ChannelReader,
	writer ChannelWriter,
	credentials ChannelCredentialRepo,
	deliveries ChannelDeliveryRepo,
	peerUsecase *ChannelPeerUsecase,
	agents AgentRepository,
	teams TeamReader,
	crypto *CredentialCrypto,
	lg loggateway.Logger,
) *ChannelUsecase {
	return &ChannelUsecase{reader: reader, writer: writer, credentials: credentials, deliveries: deliveries, peerUsecase: peerUsecase, agents: agents, teams: teams, crypto: crypto, healthCheckConcurrency: defaultHealthCheckConcurrency, lg: lg}
}

// ProvideChannelUsecase is the Wire provider that constructs a ChannelUsecase
// and injects the transaction provider so Create/Update wrap channel + credential
// writes in a single transaction (red line #24). Tests call NewChannelUsecase
// directly (without a txProvider) to preserve legacy non-transactional behavior.
func ProvideChannelUsecase(
	reader ChannelReader,
	writer ChannelWriter,
	credentials ChannelCredentialRepo,
	deliveries ChannelDeliveryRepo,
	peerUsecase *ChannelPeerUsecase,
	agents AgentRepository,
	teams TeamReader,
	crypto *CredentialCrypto,
	tp ChannelTxProvider,
	lg loggateway.Logger,
) *ChannelUsecase {
	uc := NewChannelUsecase(reader, writer, credentials, deliveries, peerUsecase, agents, teams, crypto, lg)
	uc.SetTxProvider(tp)
	return uc
}

// SetHealthCheckConcurrency configures the maximum concurrent health checks.
// If n <= 0, the default (8) is used.
func (u *ChannelUsecase) SetHealthCheckConcurrency(n int) {
	if n <= 0 {
		n = defaultHealthCheckConcurrency
	}
	u.healthCheckConcurrency = n
}

// SetTxProvider sets the transaction provider used to wrap multi-step writes
// (channel + credentials) in a single atomic transaction. When not set, the
// writes execute non-transactionally (preserving legacy behavior for tests
// and offline tooling).
func (u *ChannelUsecase) SetTxProvider(tp ChannelTxProvider) {
	u.txProvider = tp
}

func (u *ChannelUsecase) ChannelTypes() []ChannelTypeItem {
	return catalogSorted()
}

func (u *ChannelUsecase) List(ctx context.Context) ([]Channel, error) {
	return u.reader.List(ctx)
}

// ListPaged returns a page of channels for the admin registry UI.
func (u *ChannelUsecase) ListPaged(ctx context.Context, q ChannelListQuery) (ChannelListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.reader.ListPaged(ctx, q)
}

func (u *ChannelUsecase) DecryptSecretRef(ctx context.Context, ref string) (string, error) {
	return u.crypto.DecryptChannelSecretRef(ctx, ref)
}

func (u *ChannelUsecase) EncryptSecretRef(ctx context.Context, plain string) (string, error) {
	return u.crypto.EncryptChannelSecretRef(ctx, plain)
}

func (u *ChannelUsecase) Get(ctx context.Context, id string) (Channel, error) {
	id, err := requireNonEmpty(id, "CHANNEL", "id")
	if err != nil {
		return Channel{}, err
	}
	return u.reader.Get(ctx, id)
}

// GetByKey loads a channel by unique channel_key (webhook path segment).
func (u *ChannelUsecase) GetByKey(ctx context.Context, channelKey string) (Channel, error) {
	channelKey, err := requireNonEmpty(channelKey, "CHANNEL", "channel_key")
	if err != nil {
		return Channel{}, err
	}
	return u.reader.GetByKey(ctx, channelKey)
}

func (u *ChannelUsecase) Create(ctx context.Context, row Channel, credentials []ChannelCredentialInput) (Channel, error) {
	row.Resource = "channels"
	if strings.TrimSpace(row.ID) == "" {
		row.ID = uuid.NewString()
	}
	if err := normalizeChannel(&row); err != nil {
		return Channel{}, err
	}
	if u.txProvider != nil {
		var created Channel
		err := u.txProvider.ExecInTx(ctx, func(txCtx context.Context) error {
			c, err := u.writer.Create(txCtx, row)
			if err != nil {
				return err
			}
			created = c
			if _, err = u.UpsertCredentials(txCtx, created.ID, credentials); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return Channel{}, err
		}
		return created, nil
	}
	created, err := u.writer.Create(ctx, row)
	if err != nil {
		return Channel{}, err
	}
	if _, err = u.UpsertCredentials(ctx, created.ID, credentials); err != nil {
		return Channel{}, err
	}
	return created, nil
}

func (u *ChannelUsecase) Update(ctx context.Context, id string, opts ChannelUpdateOptions, credentials []ChannelCredentialInput) (Channel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Channel{}, apierror.BadRequest("CHANNEL", "id is required")
	}
	current, err := u.reader.Get(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	row := current // start from current state
	if opts.Key != nil {
		row.Key = *opts.Key
	}
	if opts.Name != nil {
		row.Name = *opts.Name
	}
	if opts.Description != nil {
		row.Description = *opts.Description
	}
	if opts.Enabled != nil {
		row.Enabled = *opts.Enabled
	}
	if opts.SortOrder != nil {
		row.SortOrder = *opts.SortOrder
	}
	if opts.AgentID != nil {
		row.AgentID = *opts.AgentID
	}
	if opts.Provider != nil {
		row.Provider = *opts.Provider
	}
	if opts.Model != nil {
		row.Model = *opts.Model
	}
	if opts.ConfigJSON != nil {
		row.ConfigJSON = *opts.ConfigJSON
	}
	if opts.MetadataJSON != nil {
		row.MetadataJSON = *opts.MetadataJSON
	}
	if opts.Status != nil {
		row.Status = *opts.Status
	}
	row.ID = id
	row.Resource = "channels"
	if err := normalizeChannel(&row); err != nil {
		return Channel{}, err
	}
	// Channel type is immutable — changing it would invalidate existing credentials,
	// peer sessions, and runtime leases that are type-specific.
	if ChannelTypeFromConfig(row.ConfigJSON) != ChannelTypeFromConfig(current.ConfigJSON) {
		return Channel{}, apierror.BadRequest("CHANNEL", "channel type cannot be changed after creation; delete and recreate the channel instead")
	}
	if u.txProvider != nil {
		var updated Channel
		err := u.txProvider.ExecInTx(ctx, func(txCtx context.Context) error {
			upd, err := u.writer.Update(txCtx, row)
			if err != nil {
				return err
			}
			updated = upd
			if _, err = u.UpsertCredentials(txCtx, updated.ID, credentials); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return Channel{}, err
		}
		return updated, nil
	}
	updated, err := u.writer.Update(ctx, row)
	if err != nil {
		return Channel{}, err
	}
	if _, err = u.UpsertCredentials(ctx, updated.ID, credentials); err != nil {
		return Channel{}, err
	}
	return updated, nil
}

func (u *ChannelUsecase) Delete(ctx context.Context, id string) error {
	id, err := requireNonEmpty(id, "CHANNEL", "id")
	if err != nil {
		return err
	}
	return u.writer.Delete(ctx, id)
}

func (u *ChannelUsecase) Toggle(ctx context.Context, id string, enabled bool) (Channel, error) {
	id, err := requireNonEmpty(id, "CHANNEL", "id")
	if err != nil {
		return Channel{}, err
	}
	row, err := u.reader.Get(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	row.Enabled = enabled
	if row.Status == "" || row.Status == ChannelStatusDeleted {
		row.Status = ChannelStatusActive
	}
	return u.writer.Update(ctx, row)
}

func (u *ChannelUsecase) ListCredentials(ctx context.Context, channelID string) ([]ChannelCredential, error) {
	items, err := u.credentials.ListCredentials(ctx, strings.TrimSpace(channelID))
	if err != nil {
		return nil, err
	}
	return sanitizeCredentials(items), nil
}

// ListCredentialsRaw returns credentials including secret_ref (for server-side runtime only).
func (u *ChannelUsecase) ListCredentialsRaw(ctx context.Context, channelID string) ([]ChannelCredential, error) {
	return u.credentials.ListCredentials(ctx, strings.TrimSpace(channelID))
}

func (u *ChannelUsecase) UpsertCredentials(ctx context.Context, channelID string, inputs []ChannelCredentialInput) ([]ChannelCredential, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, apierror.BadRequest("CHANNEL", "channel id is required")
	}
	var result []ChannelCredential
	for _, input := range inputs {
		key := strings.TrimSpace(input.CredentialKey)
		if key == "" {
			continue
		}
		secret := strings.TrimSpace(input.Secret)
		secretRef := strings.TrimSpace(input.SecretRef)
		if secret == "" && secretRef == "" {
			continue
		}
		if secretRef == "" {
			var encErr error
			secretRef, encErr = u.crypto.EncryptChannelSecretRef(ctx, secret)
			if encErr != nil {
				return nil, encErr
			}
		}
		status := strings.TrimSpace(input.Status)
		if status == "" {
			status = ChannelStatusActive
		}
		metadata := strings.TrimSpace(input.MetadataJSON)
		if metadata == "" {
			metadata = "{}"
		}
		if !json.Valid([]byte(metadata)) {
			return nil, channelValidationError("credential %s metadata_json must be valid JSON", key)
		}
		created, err := u.credentials.UpsertCredential(ctx, ChannelCredential{
			ID:            uuid.NewString(),
			ChannelID:     channelID,
			CredentialKey: key,
			Status:        status,
			SecretRef:     secretRef,
			MetadataJSON:  metadata,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, created)
	}
	return sanitizeCredentials(result), nil
}

// RunHealthChecks re-evaluates enabled channels and refreshes status/metadata.
func (u *ChannelUsecase) RunHealthChecks(ctx context.Context) error {
	items, err := u.reader.List(ctx)
	if err != nil {
		return err
	}
	// Limit concurrency to avoid resource exhaustion with many channels.
	concurrency := u.healthCheckConcurrency
	if concurrency <= 0 {
		concurrency = defaultHealthCheckConcurrency
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, row := range items {
		if !row.Enabled {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		ch := row
		safego.Go(ctx, "channel.EvaluateTestAll", func() {
			defer wg.Done()
			defer func() { <-sem }()
			writeCtx, writeCancel := context.WithTimeout(ctx, 30*time.Second)
			defer writeCancel()
			credentials, err := u.credentials.ListCredentials(writeCtx, ch.ID)
			if err != nil {
				u.lg.Warn("list credentials for health check failed", loggateway.Err(err), loggateway.Str("channel_id", ch.ID))
				return
			}
			result, err := EvaluateChannelTest(ch, credentials)
			if err != nil {
				u.lg.Warn("evaluate channel test for health check failed", loggateway.Err(err), loggateway.Str("channel_id", ch.ID))
				return
			}
			if _, err := u.updateTestMetadata(writeCtx, ch, result); err != nil {
				u.lg.Warn("update channel test metadata failed", loggateway.Err(err), loggateway.Str("channel_id", ch.ID))
			}
		})
	}
	wg.Wait()
	return nil
}

func (u *ChannelUsecase) DeleteCredential(ctx context.Context, channelID, key string) error {
	return u.credentials.DeleteCredential(ctx, strings.TrimSpace(channelID), strings.TrimSpace(key))
}

func (u *ChannelUsecase) ListDeliveries(ctx context.Context, channelID string, limit int) ([]ChannelDelivery, error) {
	return u.deliveries.ListDeliveries(ctx, strings.TrimSpace(channelID), limit)
}

// AddInboundDelivery records a runtime webhook/delivery row (payload must not contain message bodies).
func (u *ChannelUsecase) AddInboundDelivery(ctx context.Context, channelID, status, payloadJSON, errMsg string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return apierror.BadRequest("CHANNEL", "channel id is required")
	}
	_, err := u.deliveries.AddDelivery(ctx, ChannelDelivery{
		ID:           uuid.NewString(),
		ChannelID:    channelID,
		Status:       strings.TrimSpace(status),
		PayloadJSON:  strings.TrimSpace(payloadJSON),
		ErrorMessage: strings.TrimSpace(errMsg),
	})
	return err
}

func (u *ChannelUsecase) Test(ctx context.Context, id string) (ChannelTestResult, error) {
	row, err := u.reader.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return ChannelTestResult{}, err
	}
	credentials, err := u.credentials.ListCredentials(ctx, row.ID)
	if err != nil {
		return ChannelTestResult{}, err
	}
	result, err := EvaluateChannelTest(row, credentials)
	if err != nil {
		return ChannelTestResult{}, err
	}
	return u.CommitChannelTest(ctx, row, credentials, result)
}

// CommitChannelTest persists a test delivery row and updates channel metadata from result.
func (u *ChannelUsecase) CommitChannelTest(ctx context.Context, row Channel, credentials []ChannelCredential, result ChannelTestResult) (ChannelTestResult, error) {
	cfg, err := row.ParseConfig()
	if err != nil {
		return result, err
	}
	payload, _ := json.Marshal(map[string]any{
		"type":          cfg.Type,
		"receive_mode":  cfg.ReceiveMode,
		"credential_ok": credentialCount(credentials),
		"result_status": result.Status,
	})
	if _, err := u.deliveries.AddDelivery(ctx, ChannelDelivery{
		ID:           uuid.NewString(),
		ChannelID:    row.ID,
		Status:       result.Status,
		PayloadJSON:  string(payload),
		ErrorMessage: errorMessageForTest(result),
		// (channel_id, idempotency_key) 有唯一索引：测试记录须携带唯一 idem key，
		// 否则空串导致同渠道第 2 次起测试历史永不落库
		IdempotencyKey: "test:" + uuid.NewString(),
	}); err != nil {
		u.lg.Warn("add channel delivery failed", loggateway.Err(err), loggateway.Str("channel_id", row.ID))
	}
	return u.updateTestMetadata(ctx, row, result)
}

func (u *ChannelUsecase) updateTestMetadata(ctx context.Context, row Channel, result ChannelTestResult) (ChannelTestResult, error) {
	var metadata map[string]any
	if json.Unmarshal([]byte(defaultJSON(row.MetadataJSON)), &metadata) != nil {
		metadata = map[string]any{}
	}
	if metadata == nil {
		// metadata_json held the JSON literal "null": Unmarshal succeeds but
		// leaves the map nil — writing to it would panic (CH-R4).
		metadata = map[string]any{}
	}
	if result.OK {
		metadata["last_error_code"] = ""
		metadata["last_error_message"] = ""
		metadata["connected_at"] = nowUTCString()
		row.Status = ChannelStatusActive
	} else {
		metadata["last_error_code"] = result.Status
		metadata["last_error_message"] = result.Message
		if result.Status == ChannelStatusError {
			row.Status = ChannelStatusError
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return result, err
	}
	row.MetadataJSON = string(raw)
	_, err = u.writer.Update(ctx, row)
	return result, err
}

func (ch Channel) ParseConfig() (ChannelConfig, error) {
	return parseChannelConfig(ch.ConfigJSON)
}

func (u *ChannelUsecase) DeletePeerBindingsByChannelID(ctx context.Context, channelID string) (int, error) {
	if u.peerUsecase == nil {
		return 0, nil
	}
	return u.peerUsecase.DeletePeerBindingsByChannelID(ctx, channelID)
}

func (u *ChannelUsecase) GetPeerSession(ctx context.Context, channelID, peerKey string) (ChannelPeerSession, error) {
	if u.peerUsecase == nil {
		return ChannelPeerSession{}, shared.ErrNotFound
	}
	return u.peerUsecase.GetPeerSession(ctx, channelID, peerKey)
}

func (u *ChannelUsecase) CreatePeerSession(ctx context.Context, row ChannelPeerSession) (ChannelPeerSession, error) {
	if u.peerUsecase == nil {
		return ChannelPeerSession{}, apierror.Internal("CHANNEL", "peer session repository not configured")
	}
	return u.peerUsecase.CreatePeerSession(ctx, row)
}

func (u *ChannelUsecase) UpdatePeerSessionID(ctx context.Context, channelID, peerKey, sessionID string) (ChannelPeerSession, error) {
	if u.peerUsecase == nil {
		return ChannelPeerSession{}, apierror.Internal("CHANNEL", "peer session repository not configured")
	}
	return u.peerUsecase.UpdatePeerSessionID(ctx, channelID, peerKey, sessionID)
}

func (u *ChannelUsecase) TryClaimInbound(ctx context.Context, channelID, platform, messageKey, peerID, text string) (bool, error) {
	if u.peerUsecase == nil {
		return false, nil
	}
	return u.peerUsecase.TryClaimInbound(ctx, channelID, platform, messageKey, peerID, text)
}

func (u *ChannelUsecase) ResolveChannelTarget(ctx context.Context, routing ChannelRouting, peerID string) (string, string, string, error) {
	return ResolveChannelTarget(ctx, u.agents, u.teams, routing, peerID)
}

func (u *ChannelUsecase) GetTeamByID(ctx context.Context, teamID string) (Team, error) {
	if u.teams == nil {
		return Team{}, apierror.Internal("CHANNEL", "team repository not configured")
	}
	return u.teams.GetTeamByID(ctx, teamID)
}

func (u *ChannelUsecase) AgentKeyResolver(ctx context.Context) func(agentID string) string {
	if u.agents == nil {
		return nil
	}
	return func(agentID string) string {
		ag, err := u.agents.GetAgentByID(ctx, strings.TrimSpace(agentID))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(ag.AgentKey)
	}
}

func (ch Channel) ParseMetadata() (map[string]any, error) {
	var m map[string]any
	raw := defaultJSON(ch.MetadataJSON)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}, err
	}
	return m, nil
}
