package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformchannel"
	"aranea-agents/internal/data/ent/platformchannelcredential"
	"aranea-agents/internal/data/ent/platformchanneldelivery"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect/sql"
)

type channelRepo struct {
	data *Data
}

var (
	_ biz.ChannelReader         = (*channelRepo)(nil)
	_ biz.ChannelWriter         = (*channelRepo)(nil)
	_ biz.ChannelCredentialRepo = (*channelRepo)(nil)
	_ biz.ChannelDeliveryRepo   = (*channelRepo)(nil)
	_ biz.ChannelDeliveryReader = (*channelRepo)(nil)
)

// NewChannelRepo implements channel sub-interfaces (legacy channels / channel_* tables).
func NewChannelRepo(d *Data) *channelRepo {
	return &channelRepo{data: d}
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
	rows, err := r.data.RW().Read(ctx).PlatformChannel.Query().
		Where(platformchannel.DeletedAtEQ("")).
		Order(platformchannel.BySortOrder(sql.OrderAsc()), platformchannel.ByCreatedAt(sql.OrderDesc())).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
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
		return biz.Channel{}, apierror.BadRequest("CHANNEL", "channel id is required")
	}
	e, err := r.data.RW().Read(ctx).PlatformChannel.Query().
		Where(
			platformchannel.IDEQ(id),
			platformchannel.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		return biz.Channel{}, entErrToBizErr(err, "CHANNEL")
	}
	return entToChannel(e), nil
}

func (r *channelRepo) GetByKey(ctx context.Context, key string) (biz.Channel, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return biz.Channel{}, apierror.BadRequest("CHANNEL", "channel key is required")
	}
	e, err := r.data.RW().Read(ctx).PlatformChannel.Query().
		Where(
			platformchannel.ChannelKeyEQ(key),
			platformchannel.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		return biz.Channel{}, entErrToBizErr(err, "CHANNEL")
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Create(ctx context.Context, row biz.Channel) (biz.Channel, error) {
	now := nowRFC3339()
	if row.CreatedAt == "" {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	b := r.data.RW().Write(ctx).PlatformChannel.Create().
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
		return biz.Channel{}, entErrToBizErr(err, "CHANNEL")
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Update(ctx context.Context, row biz.Channel) (biz.Channel, error) {
	e, err := r.data.RW().Write(ctx).PlatformChannel.UpdateOneID(row.ID).
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
		return biz.Channel{}, entErrToBizErr(err, "CHANNEL")
	}
	return entToChannel(e), nil
}

func (r *channelRepo) Delete(ctx context.Context, id string) error {
	cleanID := strings.TrimSpace(id)
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := nowRFC3339()
		if _, err := r.data.RW().Write(txCtx).PlatformChannel.UpdateOneID(cleanID).
			SetDeletedAt(now).
			SetStatus("deleted").
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return entErrToBizErr(err, "CHANNEL")
		}
		return cascadeDeleteByChannel(txCtx, r.data, cleanID)
	})
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
	rows, err := r.data.RW().Read(ctx).PlatformChannelCredential.Query().
		Where(
			platformchannelcredential.ChannelIDEQ(strings.TrimSpace(channelID)),
			platformchannelcredential.DeletedAtEQ(""),
		).
		Order(platformchannelcredential.ByCredentialKey()).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
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
		return biz.ChannelCredential{}, apierror.BadRequest("CHANNEL", "id, channel_id and credential_key are required")
	}
	now := nowRFC3339()
	if cred.Status == "" {
		cred.Status = "active"
	}
	if cred.MetadataJSON == "" {
		cred.MetadataJSON = "{}"
	}
	if cred.CreatedAt == "" {
		cred.CreatedAt = now
	}
	cred.UpdatedAt = now

	id, err := r.data.RW().Write(ctx).PlatformChannelCredential.Create().
		SetID(cred.ID).
		SetChannelID(cred.ChannelID).
		SetCredentialKey(cred.CredentialKey).
		SetStatus(cred.Status).
		SetSecretRef(cred.SecretRef).
		SetMetadataJSON(cred.MetadataJSON).
		SetCreatedAt(cred.CreatedAt).
		SetUpdatedAt(cred.UpdatedAt).
		SetDeletedAt("").
		OnConflictColumns(platformchannelcredential.FieldChannelID, platformchannelcredential.FieldCredentialKey).
		UpdateNewValues().
		Update(func(u *ent.PlatformChannelCredentialUpsert) {
			u.SetIgnore(platformchannelcredential.FieldCreatedAt)
		}).
		ID(ctx)
	if err != nil {
		return biz.ChannelCredential{}, entErrToBizErr(err, "CHANNEL")
	}
	// Read back the upserted row to return consistent state
	e, err := r.data.RW().Read(ctx).PlatformChannelCredential.Get(ctx, id)
	if err != nil {
		return biz.ChannelCredential{}, entErrToBizErr(err, "CHANNEL")
	}
	return credentialEntToBiz(e), nil
}

func (r *channelRepo) DeleteCredential(ctx context.Context, channelID, credentialKey string) error {
	now := nowRFC3339()
	_, err := r.data.RW().Write(ctx).PlatformChannelCredential.Update().
		Where(
			platformchannelcredential.ChannelIDEQ(strings.TrimSpace(channelID)),
			platformchannelcredential.CredentialKeyEQ(strings.TrimSpace(credentialKey)),
			platformchannelcredential.DeletedAtEQ(""),
		).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return entErrToBizErr(err, "CHANNEL")
}

func deliveryEntToBiz(e *ent.PlatformChannelDelivery) biz.ChannelDelivery {
	return biz.ChannelDelivery{
		ID:             e.ID,
		ChannelID:      e.ChannelID,
		AgentID:        e.AgentID,
		IdempotencyKey: e.IdempotencyKey,
		Status:         e.Status,
		PayloadJSON:    e.PayloadJSON,
		ErrorMessage:   e.ErrorMessage,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

func (r *channelRepo) ListDeliveries(ctx context.Context, channelID string, limit int) ([]biz.ChannelDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.data.RW().Read(ctx).PlatformChannelDelivery.Query().
		Where(platformchanneldelivery.ChannelIDEQ(strings.TrimSpace(channelID))).
		Order(platformchanneldelivery.ByCreatedAt(sql.OrderDesc())).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "CHANNEL")
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
	b := r.data.RW().Write(ctx).PlatformChannelDelivery.Create().
		SetID(d.ID).
		SetChannelID(d.ChannelID).
		SetAgentID(strings.TrimSpace(d.AgentID)).
		SetIdempotencyKey(strings.TrimSpace(d.IdempotencyKey)).
		SetStatus(d.Status).
		SetPayloadJSON(d.PayloadJSON).
		SetErrorMessage(d.ErrorMessage).
		SetCreatedAt(d.CreatedAt).
		SetUpdatedAt(d.UpdatedAt)
	e, err := b.Save(ctx)
	if err != nil {
		return biz.ChannelDelivery{}, entErrToBizErr(err, "CHANNEL")
	}
	return deliveryEntToBiz(e), nil
}

func (r *channelRepo) AddDeliveryIfNotExists(ctx context.Context, d biz.ChannelDelivery) (biz.ChannelDelivery, bool, error) {
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

	idempotencyKey := strings.TrimSpace(d.IdempotencyKey)
	if idempotencyKey == "" {
		// No idempotency key; fall back to regular insert.
		result, err := r.AddDelivery(ctx, d)
		return result, true, err
	}

	b := r.data.RW().Write(ctx).PlatformChannelDelivery.Create().
		SetID(d.ID).
		SetChannelID(d.ChannelID).
		SetAgentID(strings.TrimSpace(d.AgentID)).
		SetIdempotencyKey(idempotencyKey).
		SetStatus(d.Status).
		SetPayloadJSON(d.PayloadJSON).
		SetErrorMessage(d.ErrorMessage).
		SetCreatedAt(d.CreatedAt).
		SetUpdatedAt(d.UpdatedAt).
		// Unique index is on (channel_id, idempotency_key); both columns must match.
		// On conflict, do NOT update any field — idempotency means "same request, same result".
		OnConflictColumns(platformchanneldelivery.FieldChannelID, platformchanneldelivery.FieldIdempotencyKey).
		Ignore()

	e, err := b.ID(ctx)
	if err != nil {
		// Check if it's a conflict error — load the existing row by the full unique key.
		existing, findErr := r.data.RW().Read(ctx).PlatformChannelDelivery.Query().
			Where(
				platformchanneldelivery.ChannelID(d.ChannelID),
				platformchanneldelivery.IdempotencyKey(idempotencyKey),
			).
			Only(ctx)
		if findErr != nil {
			r.data.lg.Warn("query existing delivery failed after insert conflict",
				loggateway.Domain("CHANNEL"),
				loggateway.Err(findErr))
			return biz.ChannelDelivery{}, false, entErrToBizErr(err, "CHANNEL")
		}
		return deliveryEntToBiz(existing), false, nil
	}
	// Load the upserted row by ID to return full entity.
	upserted, findErr := r.data.RW().Read(ctx).PlatformChannelDelivery.Get(ctx, e)
	if findErr != nil {
		return biz.ChannelDelivery{}, true, entErrToBizErr(findErr, "CHANNEL")
	}
	return deliveryEntToBiz(upserted), true, nil
}

func (r *channelRepo) ListPendingDeliveries(ctx context.Context, limit int) ([]biz.ChannelDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.data.RW().Read(ctx).PlatformChannelDelivery.Query().
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
		return nil, entErrToBizErr(err, "CHANNEL")
	}
	out := make([]biz.ChannelDelivery, 0, len(rows))
	for _, e := range rows {
		out = append(out, deliveryEntToBiz(e))
	}
	return out, nil
}

func (r *channelRepo) UpdateDelivery(ctx context.Context, d biz.ChannelDelivery) error {
	_, err := r.data.RW().Write(ctx).PlatformChannelDelivery.UpdateOneID(strings.TrimSpace(d.ID)).
		SetStatus(strings.TrimSpace(d.Status)).
		SetPayloadJSON(defaultJSON(d.PayloadJSON)).
		SetErrorMessage(strings.TrimSpace(d.ErrorMessage)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return entErrToBizErr(err, "CHANNEL")
}
