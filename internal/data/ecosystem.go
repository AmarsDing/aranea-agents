package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizecosystem "aranea-agents/internal/biz/ecosystem"
)

type ecosystemRepo struct {
	data *Data
}

var _ bizecosystem.Repo = (*ecosystemRepo)(nil)

func NewEcosystemRepo(data *Data) biz.EcosystemRepo {
	return &ecosystemRepo{data: data}
}

func (r *ecosystemRepo) ListProducts(ctx context.Context, q biz.EcosystemQuery) (biz.EcosystemListResult, error) {
	where := []string{`deleted_at = ''`, `status = 'published'`}
	args := []any{}
	if t := strings.TrimSpace(q.Type); t != "" {
		where = append(where, `type = ?`)
		args = append(args, t)
	}
	if s := strings.TrimSpace(q.Search); s != "" {
		where = append(where, `(name LIKE ? OR display_name LIKE ? OR description LIKE ?)`)
		pattern := "%" + s + "%"
		args = append(args, pattern, pattern, pattern)
	}
	wsql := strings.Join(where, " AND ")
	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, `SELECT COUNT(*) FROM ecosystem_products WHERE `+wsql, args...).Scan(&total); err != nil {
		return biz.EcosystemListResult{}, err
	}
	limit := int(q.Limit)
	if limit <= 0 {
		limit = 50
	}
	offset := int(q.Offset)
	if offset < 0 {
		offset = 0
	}
	rows, err := r.data.RawDB().QueryContext(ctx, `
SELECT id, name, display_name, description, type, author_id, version, price_model, price_cents,
       rating, install_count, config_json, status, created_at, updated_at
FROM ecosystem_products WHERE `+wsql+` ORDER BY install_count DESC, created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return biz.EcosystemListResult{}, err
	}
	defer rows.Close()
	var items []biz.EcosystemProduct
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return biz.EcosystemListResult{}, err
		}
		p.Installed, _ = r.IsInstalled(ctx, p.ID)
		items = append(items, p)
	}
	return biz.EcosystemListResult{Items: items, Total: total}, rows.Err()
}

func (r *ecosystemRepo) GetProduct(ctx context.Context, id string) (biz.EcosystemProduct, error) {
	row := r.data.RawDB().QueryRowContext(ctx, `
SELECT id, name, display_name, description, type, author_id, version, price_model, price_cents,
       rating, install_count, config_json, status, created_at, updated_at
FROM ecosystem_products WHERE id = ? AND deleted_at = ''`, id)
	p, err := scanProductRow(row)
	if err == sql.ErrNoRows {
		return biz.EcosystemProduct{}, fmt.Errorf("product not found")
	}
	return p, err
}

func (r *ecosystemRepo) CreateProduct(ctx context.Context, p biz.EcosystemProduct) (biz.EcosystemProduct, error) {
	_, err := r.data.RawDB().ExecContext(ctx, `
INSERT INTO ecosystem_products (id, name, display_name, description, type, author_id, version, price_model,
  price_cents, rating, install_count, config_json, status, created_at, updated_at, deleted_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'')`,
		p.ID, p.Name, p.DisplayName, p.Description, p.Type, p.AuthorID, p.Version, p.PriceModel,
		p.PriceCents, p.Rating, p.InstallCount, defaultJSON(p.ConfigJSON), p.Status, p.CreatedAt, p.UpdatedAt,
	)
	return p, err
}

func (r *ecosystemRepo) RecordInstall(ctx context.Context, productID, refID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := r.data.RawDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO ecosystem_installs (id, product_id, installed_ref_id, created_at, deleted_at)
VALUES (?,?,?,?,'')`, refID, productID, refID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ecosystem_products SET install_count = install_count + 1, updated_at = ? WHERE id = ?`, now, productID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ecosystemRepo) RemoveInstall(ctx context.Context, productID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.RawDB().ExecContext(ctx, `
UPDATE ecosystem_installs SET deleted_at = ? WHERE product_id = ? AND deleted_at = ''`, now, productID)
	return err
}

func (r *ecosystemRepo) IsInstalled(ctx context.Context, productID string) (bool, error) {
	var n int
	err := r.data.RawDB().QueryRowContext(ctx, `
SELECT COUNT(*) FROM ecosystem_installs WHERE product_id = ? AND deleted_at = ''`, productID).Scan(&n)
	return n > 0, err
}

func scanProduct(rows *sql.Rows) (biz.EcosystemProduct, error) {
	var p biz.EcosystemProduct
	err := rows.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Description, &p.Type, &p.AuthorID, &p.Version,
		&p.PriceModel, &p.PriceCents, &p.Rating, &p.InstallCount, &p.ConfigJSON, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func scanProductRow(row *sql.Row) (biz.EcosystemProduct, error) {
	var p biz.EcosystemProduct
	err := row.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Description, &p.Type, &p.AuthorID, &p.Version,
		&p.PriceModel, &p.PriceCents, &p.Rating, &p.InstallCount, &p.ConfigJSON, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}
