package data

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformchannel"
	"aranea-agents/internal/data/ent/platformchannelcredential"
	"aranea-agents/internal/data/ent/platformchanneldelivery"

	"entgo.io/ent/dialect/sql"
)

type channelRepo struct {
	data *Data
}

var _ biz.ChannelRepo = (*channelRepo)(nil)

// NewChannelRepo implements biz.ChannelRepo (legacy channels / channel_* tables).
func NewChannelRepo(d *Data) biz.ChannelRepo {
	return &channelRepo{data: d}
}

func (r *channelRepo) entClient() *ent.Client {
	return r.data.Ent()
}

func (r *channelRepo) readClient(ctx context.Context) *ent.Client {
	return r.data.ReadClient(ctx)
}

func entToChannel(e *ent.PlatformChannel) biz.Channel {
	return biz.Channel{
		ID:           e.ID,
		Resource:     "channels",
		Key:          e.ChannelKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
		DeletedAt:    e.DeletedAt,
	}
}

func (r *channelRepo) List(ctx context.Context) ([]biz.Channel, error) {
	rows, err := r.readClient(ctx).PlatformChannel.Query().
		Where(platformchannel.DeletedAtEQ("")).
		Order(platformchannel.BySortOrder(sql.OrderAsc()), platformchannel.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Channel, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToChannel(e))
	}
	return out, nil
}

func (r *channelRepo) Get(ctx context.Context, id string) (biz.Channel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return biz.Channel{}, errors.New("channel id is required")
	}
	e, err := r.readClient(ctx).PlatformChannel.Query().
		Where(
			platformchannel.IDEQ(id),
			platformchannel.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		return biz.Channel{}, err
	}
	return entToChannel(e), nil
}

func (r *channelRepo) GetByKey(ctx context.Context, key string) (biz.Channel, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return biz.Channel{}, errors.New("channel key is required")
	}
	e, err := r.readClient(ctx).PlatformChannel.Query().
		Where(
			platformchannel.ChannelKeyEQ(key),
			platformchannel.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		return biz.Channel{}, err
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Create(ctx context.Context, row biz.Channel) (biz.Channel, error) {
	now := nowRFC3339()
	if row.CreatedAt == "" {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	b := r.entClient().PlatformChannel.Create().
		SetID(row.ID).
		SetChannelKey(row.Key).
		SetName(row.Name).
		SetDescription(row.Description).
		SetStatus(row.Status).
		SetEnabled(row.Enabled).
		SetSortOrder(row.SortOrder).
		SetConfigJSON(row.ConfigJSON).
		SetMetadataJSON(row.MetadataJSON).
		SetCreatedAt(row.CreatedAt).
		SetUpdatedAt(row.UpdatedAt).
		SetDeletedAt("")
	e, err := b.Save(ctx)
	if err != nil {
		return biz.Channel{}, err
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Update(ctx context.Context, row biz.Channel) (biz.Channel, error) {
	e, err := r.entClient().PlatformChannel.UpdateOneID(row.ID).
		SetChannelKey(row.Key).
		SetName(row.Name).
		SetDescription(row.Description).
		SetStatus(row.Status).
		SetEnabled(row.Enabled).
		SetSortOrder(row.SortOrder).
		SetConfigJSON(row.ConfigJSON).
		SetMetadataJSON(row.MetadataJSON).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	if err != nil {
		return biz.Channel{}, err
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Delete(ctx context.Context, id string) error {
	now := nowRFC3339()
	_, err := r.entClient().PlatformChannel.UpdateOneID(strings.TrimSpace(id)).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func credentialEntToBiz(e *ent.PlatformChannelCredential) biz.ChannelCredential {
	return biz.ChannelCredential{
		ID:            e.ID,
		ChannelID:     e.ChannelID,
		CredentialKey: e.CredentialKey,
		Status:        e.Status,
		SecretRef:     e.SecretRef,
		MetadataJSON:  e.MetadataJSON,
		CreatedAt:     e.CreatedAt,
		UpdatedAt:     e.UpdatedAt,
		DeletedAt:     e.DeletedAt,
	}
}

func (r *channelRepo) ListCredentials(ctx context.Context, channelID string) ([]biz.ChannelCredential, error) {
	rows, err := r.readClient(ctx).PlatformChannelCredential.Query().
		Where(
			platformchannelcredential.ChannelIDEQ(strings.TrimSpace(channelID)),
			platformchannelcredential.DeletedAtEQ(""),
		).
		Order(platformchannelcredential.ByCredentialKey()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChannelCredential, 0, len(rows))
	for _, e := range rows {
		out = append(out, credentialEntToBiz(e))
	}
	return out, nil
}

func (r *channelRepo) UpsertCredential(ctx context.Context, cred biz.ChannelCredential) (biz.ChannelCredential, error) {
	cred.ChannelID = strings.TrimSpace(cred.ChannelID)
	cred.CredentialKey = strings.TrimSpace(cred.CredentialKey)
	if cred.ID == "" || cred.ChannelID == "" || cred.CredentialKey == "" {
		return biz.ChannelCredential{}, errors.New("id, channel_id and credential_key are required")
	}
	existing, err := r.readClient(ctx).PlatformChannelCredential.Query().
		Where(
			platformchannelcredential.ChannelIDEQ(cred.ChannelID),
			platformchannelcredential.CredentialKeyEQ(cred.CredentialKey),
		).
		Only(ctx)
	now := nowRFC3339()
	if cred.Status == "" {
		cred.Status = "active"
	}
	if cred.MetadataJSON == "" {
		cred.MetadataJSON = "{}"
	}
	if ent.IsNotFound(err) {
		if cred.CreatedAt == "" {
			cred.CreatedAt = now
		}
		cred.UpdatedAt = now
		e, err := r.entClient().PlatformChannelCredential.Create().
			SetID(cred.ID).
			SetChannelID(cred.ChannelID).
			SetCredentialKey(cred.CredentialKey).
			SetStatus(cred.Status).
			SetSecretRef(cred.SecretRef).
			SetMetadataJSON(cred.MetadataJSON).
			SetCreatedAt(cred.CreatedAt).
			SetUpdatedAt(cred.UpdatedAt).
			SetDeletedAt("").
			Save(ctx)
		if err != nil {
			return biz.ChannelCredential{}, err
		}
		return credentialEntToBiz(e), nil
	}
	if err != nil {
		return biz.ChannelCredential{}, err
	}
	e, err := r.entClient().PlatformChannelCredential.UpdateOneID(existing.ID).
		SetStatus(cred.Status).
		SetSecretRef(cred.SecretRef).
		SetMetadataJSON(cred.MetadataJSON).
		SetUpdatedAt(now).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.ChannelCredential{}, err
	}
	return credentialEntToBiz(e), nil
}

func (r *channelRepo) DeleteCredential(ctx context.Context, channelID, credentialKey string) error {
	now := nowRFC3339()
	_, err := r.entClient().PlatformChannelCredential.Update().
		Where(
			platformchannelcredential.ChannelIDEQ(strings.TrimSpace(channelID)),
			platformchannelcredential.CredentialKeyEQ(strings.TrimSpace(credentialKey)),
			platformchannelcredential.DeletedAtEQ(""),
		).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func deliveryEntToBiz(e *ent.PlatformChannelDelivery) biz.ChannelDelivery {
	return biz.ChannelDelivery{
		ID:           e.ID,
		ChannelID:    e.ChannelID,
		AgentID:      e.AgentID,
		Status:       e.Status,
		PayloadJSON:  e.PayloadJSON,
		ErrorMessage: e.ErrorMessage,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

func (r *channelRepo) ListDeliveries(ctx context.Context, channelID string, limit int) ([]biz.ChannelDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.readClient(ctx).PlatformChannelDelivery.Query().
		Where(platformchanneldelivery.ChannelIDEQ(strings.TrimSpace(channelID))).
		Order(platformchanneldelivery.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChannelDelivery, 0, len(rows))
	for _, e := range rows {
		out = append(out, deliveryEntToBiz(e))
	}
	return out, nil
}

func (r *channelRepo) AddDelivery(ctx context.Context, d biz.ChannelDelivery) (biz.ChannelDelivery, error) {
	now := nowRFC3339()
	if d.Status == "" {
		d.Status = "pending"
	}
	if d.PayloadJSON == "" {
		d.PayloadJSON = "{}"
	}
	if d.CreatedAt == "" {
		d.CreatedAt = now
	}
	d.UpdatedAt = now
	b := r.entClient().PlatformChannelDelivery.Create().
		SetID(d.ID).
		SetChannelID(d.ChannelID).
		SetAgentID(strings.TrimSpace(d.AgentID)).
		SetStatus(d.Status).
		SetPayloadJSON(d.PayloadJSON).
		SetErrorMessage(d.ErrorMessage).
		SetCreatedAt(d.CreatedAt).
		SetUpdatedAt(d.UpdatedAt)
	e, err := b.Save(ctx)
	if err != nil {
		return biz.ChannelDelivery{}, err
	}
	return deliveryEntToBiz(e), nil
}

func (r *channelRepo) ListPendingDeliveries(ctx context.Context, limit int) ([]biz.ChannelDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.readClient(ctx).PlatformChannelDelivery.Query().
		Where(
			platformchanneldelivery.StatusIn(
				biz.ChannelDeliveryStatusPending,
				biz.ChannelDeliveryStatusRetry,
			),
		).
		Order(platformchanneldelivery.ByCreatedAt(sql.OrderAsc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.ChannelDelivery, 0, len(rows))
	for _, e := range rows {
		out = append(out, deliveryEntToBiz(e))
	}
	return out, nil
}

func (r *channelRepo) UpdateDelivery(ctx context.Context, d biz.ChannelDelivery) error {
	_, err := r.entClient().PlatformChannelDelivery.UpdateOneID(strings.TrimSpace(d.ID)).
		SetStatus(strings.TrimSpace(d.Status)).
		SetPayloadJSON(defaultJSON(d.PayloadJSON)).
		SetErrorMessage(strings.TrimSpace(d.ErrorMessage)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}
