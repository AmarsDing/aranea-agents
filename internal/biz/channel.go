package biz

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
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
}

type ChannelCatalogItem struct {
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
	ID           string
	ChannelID    string
	AgentID      string
	Status       string
	PayloadJSON  string
	ErrorMessage string
	CreatedAt    string
	UpdatedAt    string
}

type ChannelTestResult struct {
	OK      bool
	Status  string
	Message string
	Details map[string]any
}

type ChannelRepo interface {
	List(ctx context.Context) ([]Channel, error)
	Get(ctx context.Context, id string) (Channel, error)
	GetByKey(ctx context.Context, channelKey string) (Channel, error)
	Create(ctx context.Context, row Channel) (Channel, error)
	Update(ctx context.Context, row Channel) (Channel, error)
	Delete(ctx context.Context, id string) error
	ListCredentials(ctx context.Context, channelID string) ([]ChannelCredential, error)
	UpsertCredential(ctx context.Context, cred ChannelCredential) (ChannelCredential, error)
	DeleteCredential(ctx context.Context, channelID, credentialKey string) error
	ListDeliveries(ctx context.Context, channelID string, limit int) ([]ChannelDelivery, error)
	AddDelivery(ctx context.Context, d ChannelDelivery) (ChannelDelivery, error)
	ListPendingDeliveries(ctx context.Context, limit int) ([]ChannelDelivery, error)
	UpdateDelivery(ctx context.Context, d ChannelDelivery) error
}

type ChannelUsecase struct {
	repo ChannelRepo
}

func NewChannelUsecase(repo ChannelRepo) *ChannelUsecase {
	return &ChannelUsecase{repo: repo}
}

func (u *ChannelUsecase) Catalog() []ChannelCatalogItem {
	return catalogSorted()
}

func (u *ChannelUsecase) List(ctx context.Context) ([]Channel, error) {
	return u.repo.List(ctx)
}

func (u *ChannelUsecase) Get(ctx context.Context, id string) (Channel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Channel{}, errors.BadRequest("CHANNEL", "id is required")
	}
	return u.repo.Get(ctx, id)
}

// GetByKey loads a channel by unique channel_key (webhook path segment).
func (u *ChannelUsecase) GetByKey(ctx context.Context, channelKey string) (Channel, error) {
	channelKey = strings.TrimSpace(channelKey)
	if channelKey == "" {
		return Channel{}, errors.BadRequest("CHANNEL", "channel_key is required")
	}
	return u.repo.GetByKey(ctx, channelKey)
}

func (u *ChannelUsecase) Create(ctx context.Context, row Channel, credentials []ChannelCredentialInput) (Channel, error) {
	row.Resource = "channels"
	if strings.TrimSpace(row.ID) == "" {
		row.ID = uuid.NewString()
	}
	if err := normalizeChannel(&row); err != nil {
		return Channel{}, err
	}
	created, err := u.repo.Create(ctx, row)
	if err != nil {
		return Channel{}, err
	}
	if _, err = u.UpsertCredentials(ctx, created.ID, credentials); err != nil {
		return Channel{}, err
	}
	return created, nil
}

func (u *ChannelUsecase) Update(ctx context.Context, id string, row Channel, credentials []ChannelCredentialInput) (Channel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Channel{}, errors.BadRequest("CHANNEL", "id is required")
	}
	current, err := u.repo.Get(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	row.ID = id
	row.Resource = "channels"
	if row.Key == "" {
		row.Key = current.Key
	}
	if row.Name == "" {
		row.Name = current.Name
	}
	if row.ConfigJSON == "" {
		row.ConfigJSON = current.ConfigJSON
	}
	if row.MetadataJSON == "" {
		row.MetadataJSON = current.MetadataJSON
	}
	if row.Status == "" {
		row.Status = current.Status
	}
	if err := normalizeChannel(&row); err != nil {
		return Channel{}, err
	}
	updated, err := u.repo.Update(ctx, row)
	if err != nil {
		return Channel{}, err
	}
	if _, err = u.UpsertCredentials(ctx, updated.ID, credentials); err != nil {
		return Channel{}, err
	}
	return updated, nil
}

func (u *ChannelUsecase) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.BadRequest("CHANNEL", "id is required")
	}
	return u.repo.Delete(ctx, id)
}

func (u *ChannelUsecase) Toggle(ctx context.Context, id string, enabled bool) (Channel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Channel{}, errors.BadRequest("CHANNEL", "id is required")
	}
	row, err := u.repo.Get(ctx, id)
	if err != nil {
		return Channel{}, err
	}
	row.Enabled = enabled
	if row.Status == "" || row.Status == "deleted" {
		row.Status = "active"
	}
	return u.repo.Update(ctx, row)
}

func (u *ChannelUsecase) ListCredentials(ctx context.Context, channelID string) ([]ChannelCredential, error) {
	items, err := u.repo.ListCredentials(ctx, strings.TrimSpace(channelID))
	if err != nil {
		return nil, err
	}
	return sanitizeCredentials(items), nil
}

// ListCredentialsRaw returns credentials including secret_ref (for server-side runtime only).
func (u *ChannelUsecase) ListCredentialsRaw(ctx context.Context, channelID string) ([]ChannelCredential, error) {
	return u.repo.ListCredentials(ctx, strings.TrimSpace(channelID))
}

func (u *ChannelUsecase) UpsertCredentials(ctx context.Context, channelID string, inputs []ChannelCredentialInput) ([]ChannelCredential, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.BadRequest("CHANNEL", "channel id is required")
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
			secretRef, encErr = EncryptChannelSecretRef(ctx, secret)
			if encErr != nil {
				return nil, encErr
			}
		}
		status := strings.TrimSpace(input.Status)
		if status == "" {
			status = "active"
		}
		metadata := strings.TrimSpace(input.MetadataJSON)
		if metadata == "" {
			metadata = "{}"
		}
		if !json.Valid([]byte(metadata)) {
			return nil, channelValidationError("credential %s metadata_json must be valid JSON", key)
		}
		created, err := u.repo.UpsertCredential(ctx, ChannelCredential{
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
	items, err := u.repo.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range items {
		if !row.Enabled {
			continue
		}
		credentials, err := u.repo.ListCredentials(ctx, row.ID)
		if err != nil {
			continue
		}
		result, err := EvaluateChannelTest(row, credentials)
		if err != nil {
			continue
		}
		_, _ = u.updateTestMetadata(ctx, row, result)
	}
	return nil
}

func (u *ChannelUsecase) DeleteCredential(ctx context.Context, channelID, key string) error {
	return u.repo.DeleteCredential(ctx, strings.TrimSpace(channelID), strings.TrimSpace(key))
}

func (u *ChannelUsecase) ListDeliveries(ctx context.Context, channelID string, limit int) ([]ChannelDelivery, error) {
	return u.repo.ListDeliveries(ctx, strings.TrimSpace(channelID), limit)
}

// AddInboundDelivery records a runtime webhook/delivery row (payload must not contain message bodies).
func (u *ChannelUsecase) AddInboundDelivery(ctx context.Context, channelID, status, payloadJSON, errMsg string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return errors.BadRequest("CHANNEL", "channel id is required")
	}
	_, err := u.repo.AddDelivery(ctx, ChannelDelivery{
		ID:           uuid.NewString(),
		ChannelID:    channelID,
		Status:       strings.TrimSpace(status),
		PayloadJSON:  strings.TrimSpace(payloadJSON),
		ErrorMessage: strings.TrimSpace(errMsg),
	})
	return err
}

func (u *ChannelUsecase) Test(ctx context.Context, id string) (ChannelTestResult, error) {
	row, err := u.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return ChannelTestResult{}, err
	}
	credentials, err := u.repo.ListCredentials(ctx, row.ID)
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
	cfg, err := parseChannelConfig(row.ConfigJSON)
	if err != nil {
		return result, err
	}
	payload, _ := json.Marshal(map[string]any{
		"type":          cfg.Type,
		"receive_mode":  cfg.ReceiveMode,
		"credential_ok": credentialCount(credentials),
		"result_status": result.Status,
	})
	_, _ = u.repo.AddDelivery(ctx, ChannelDelivery{
		ID:           uuid.NewString(),
		ChannelID:    row.ID,
		Status:       result.Status,
		PayloadJSON:  string(payload),
		ErrorMessage: errorMessageForTest(result),
	})
	return u.updateTestMetadata(ctx, row, result)
}

func (u *ChannelUsecase) updateTestMetadata(ctx context.Context, row Channel, result ChannelTestResult) (ChannelTestResult, error) {
	var metadata map[string]any
	if json.Unmarshal([]byte(defaultJSON(row.MetadataJSON)), &metadata) != nil {
		metadata = map[string]any{}
	}
	if result.OK {
		metadata["last_error_code"] = ""
		metadata["last_error_message"] = ""
		metadata["connected_at"] = nowUTCString()
		row.Status = "active"
	} else {
		metadata["last_error_code"] = result.Status
		metadata["last_error_message"] = result.Message
		if result.Status == "error" {
			row.Status = "error"
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return result, err
	}
	row.MetadataJSON = string(raw)
	_, err = u.repo.Update(ctx, row)
	return result, err
}
