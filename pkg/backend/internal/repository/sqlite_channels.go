package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) ListChannelCredentials(channelID string) ([]domain.ChannelCredential, error) {
	rows, err := r.db.Query(`SELECT id, channel_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at, deleted_at FROM channel_credential WHERE channel_id = ? AND deleted_at = '' ORDER BY credential_key ASC`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChannelCredential
	for rows.Next() {
		item, err := scanChannelCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) UpsertChannelCredential(credential domain.ChannelCredential) (domain.ChannelCredential, error) {
	if credential.ID == "" || credential.ChannelID == "" || credential.CredentialKey == "" {
		return domain.ChannelCredential{}, errors.New("id, channel_id and credential_key are required")
	}
	now := nowISO()
	if credential.Status == "" {
		credential.Status = "active"
	}
	if credential.MetadataJSON == "" {
		credential.MetadataJSON = "{}"
	}
	if !json.Valid([]byte(credential.MetadataJSON)) {
		return domain.ChannelCredential{}, errors.New("credential metadata_json must be valid JSON")
	}
	if credential.CreatedAt == "" {
		credential.CreatedAt = now
	}
	credential.UpdatedAt = now
	_, err := r.db.Exec(`
		INSERT INTO channel_credential(id, channel_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(channel_id, credential_key) DO UPDATE SET
			status = excluded.status,
			secret_ref = excluded.secret_ref,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at,
			deleted_at = ''`,
		credential.ID, credential.ChannelID, credential.CredentialKey, credential.Status, credential.SecretRef, credential.MetadataJSON, credential.CreatedAt, credential.UpdatedAt,
	)
	if err != nil {
		return domain.ChannelCredential{}, err
	}
	rows, err := r.db.Query(`SELECT id, channel_id, credential_key, status, secret_ref, metadata_json, created_at, updated_at, deleted_at FROM channel_credential WHERE channel_id = ? AND credential_key = ? AND deleted_at = '' LIMIT 1`, credential.ChannelID, credential.CredentialKey)
	if err != nil {
		return domain.ChannelCredential{}, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanChannelCredential(rows)
	}
	return domain.ChannelCredential{}, sql.ErrNoRows
}

func (r *SQLiteRepository) DeleteChannelCredential(channelID string, credentialKey string) error {
	now := nowISO()
	_, err := r.db.Exec(`UPDATE channel_credential SET deleted_at = ?, updated_at = ? WHERE channel_id = ? AND credential_key = ? AND deleted_at = ''`, now, now, channelID, credentialKey)
	return err
}

func (r *SQLiteRepository) AddChannelDelivery(delivery domain.ChannelDelivery) (domain.ChannelDelivery, error) {
	if delivery.ID == "" || delivery.ChannelID == "" {
		return domain.ChannelDelivery{}, errors.New("id and channel_id are required")
	}
	now := nowISO()
	if delivery.Status == "" {
		delivery.Status = "pending"
	}
	if delivery.PayloadJSON == "" {
		delivery.PayloadJSON = "{}"
	}
	if !json.Valid([]byte(delivery.PayloadJSON)) {
		return domain.ChannelDelivery{}, errors.New("delivery payload_json must be valid JSON")
	}
	if delivery.CreatedAt == "" {
		delivery.CreatedAt = now
	}
	delivery.UpdatedAt = now
	_, err := r.db.Exec(`INSERT INTO channel_delivery(id, channel_id, agent_id, status, payload_json, error_message, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		delivery.ID, delivery.ChannelID, delivery.AgentID, delivery.Status, delivery.PayloadJSON, delivery.ErrorMessage, delivery.CreatedAt, delivery.UpdatedAt)
	return delivery, err
}

func (r *SQLiteRepository) ListChannelDeliveries(channelID string, limit int) ([]domain.ChannelDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(`SELECT id, channel_id, agent_id, status, payload_json, error_message, created_at, updated_at FROM channel_delivery WHERE channel_id = ? ORDER BY created_at DESC LIMIT ?`, channelID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChannelDelivery
	for rows.Next() {
		var item domain.ChannelDelivery
		if err := rows.Scan(&item.ID, &item.ChannelID, &item.AgentID, &item.Status, &item.PayloadJSON, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) ListEnabledChannelRuntimeConfigs() ([]domain.ChannelRuntimeConfig, error) {
	rows, err := r.db.Query(`SELECT id, channel_key, status, enabled, config_json, metadata_json FROM channel WHERE enabled = 1 AND deleted_at = '' AND status IN ('active', 'pending_auth') ORDER BY sort_order ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChannelRuntimeConfig
	for rows.Next() {
		var item domain.ChannelRuntimeConfig
		if err := rows.Scan(&item.ID, &item.Key, &item.Status, &item.Enabled, &item.ConfigJSON, &item.MetadataJSON); err != nil {
			return nil, err
		}
		var cfg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(item.ConfigJSON), &cfg) == nil {
			item.Type = cfg.Type
		}
		credentials, err := r.ListChannelCredentials(item.ID)
		if err != nil {
			return nil, err
		}
		item.Credentials = credentials
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanChannelCredential(row scanner) (domain.ChannelCredential, error) {
	var v domain.ChannelCredential
	err := row.Scan(&v.ID, &v.ChannelID, &v.CredentialKey, &v.Status, &v.SecretRef, &v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	v.Configured = strings.TrimSpace(v.SecretRef) != ""
	v.MaskedPreview = maskedSecretRef(v.SecretRef)
	return v, err
}

func maskedSecretRef(secretRef string) string {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return ""
	}
	if len(secretRef) <= 8 {
		return "********"
	}
	return secretRef[:4] + "..." + secretRef[len(secretRef)-4:]
}
