// Package configgraph implements the biz/configgraph.Repo port with raw SQL
// against config_graph_nodes / config_graph_edges (DDL migration 20261260).
// Follows the event_delivery_outbox precedent: ent schemas document the
// contract only; runtime access is raw SQL here.
package configgraph

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	bizcg "aranea-agents/internal/biz/configgraph"

	"github.com/lib/pq"
)

// domain is the apierror domain for this repo (CONFIG_GRAPH.* error codes).
const domain = "CONFIG_GRAPH"

// upsertBatchSize caps rows per multi-row INSERT (design §4.1: 500/batch;
// 500×11 params stays far below Postgres' 65535 bind limit).
const upsertBatchSize = 500

const (
	defaultListLimit = 200
	maxListLimit     = 1000
)

// brokenLike matches the evidence_json broken marker (v1 LIKE probe per
// design §2.2; volume is small, jsonb migration is a later optimization).
const brokenLike = `%"broken":true%`

type repo struct {
	rw *data.ReadWriteDB
	d  data.Dialect
	lg loggateway.Logger
}

var _ bizcg.Repo = (*repo)(nil)

// NewRepo constructs the raw-SQL graph repo from the shared Data handle.
func NewRepo(d *data.Data, lg loggateway.Logger) bizcg.Repo {
	if d == nil || d.RWDB() == nil {
		return nil
	}
	return NewRepoFromRWDB(d.RWDB(), d.Dialect(), lg)
}

// NewRepoFromRWDB constructs the repo from explicit handles (tests, seed paths).
func NewRepoFromRWDB(rw *data.ReadWriteDB, dialect data.Dialect, lg loggateway.Logger) bizcg.Repo {
	if rw == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &repo{rw: rw, d: dialect, lg: lg.With(loggateway.Domain("config_graph"))}
}

// toBizErr translates raw-SQL/Postgres errors to apierror (mirrors
// data.entErrToBizErr minus the ent branches — this repo never touches ent).
func toBizErr(err error) error {
	if err == nil {
		return nil
	}
	if ae, ok := apierror.From(err); ok {
		return ae
	}
	if errors.Is(err, sql.ErrNoRows) {
		return apierror.Wrap(err, apierror.CodeNotFound, domain)
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code.Name() {
		case "unique_violation", "foreign_key_violation":
			return apierror.Wrap(err, apierror.CodeConflict, domain)
		case "not_null_violation", "check_violation":
			return apierror.Wrap(err, apierror.CodeBadRequest, domain)
		}
	}
	return apierror.Wrap(err, apierror.CodeInternal, domain)
}

func attrsJSON(attrs map[string]any) string {
	if len(attrs) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func evidenceJSON(ev map[string]any) string { return attrsJSON(ev) }

func formatTS(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTS(raw sql.NullString) time.Time {
	if !raw.Valid || raw.String == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.String); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw.String); err == nil {
		return t
	}
	return time.Time{}
}

func (r *repo) UpsertNodes(ctx context.Context, nodes []bizcg.Node) error {
	if r == nil || len(nodes) == 0 {
		return nil
	}
	for start := 0; start < len(nodes); start += upsertBatchSize {
		batch := nodes[start:min(start+upsertBatchSize, len(nodes))]
		if err := r.upsertNodeBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

func (r *repo) upsertNodeBatch(ctx context.Context, batch []bizcg.Node) error {
	const cols = 11
	var sb strings.Builder
	sb.WriteString(`INSERT INTO config_graph_nodes (id, node_type, ref_id, node_key, display_name, workspace_id, status, attrs_json, generation, created_at, updated_at) VALUES `)
	args := make([]any, 0, len(batch)*cols)
	wrote := 0
	for _, n := range batch {
		if n.ID == "" || n.NodeType == "" || n.RefID == "" {
			r.lg.Warn("configgraph: skip invalid node row", loggateway.StepID("config_graph.repo"))
			continue
		}
		if wrote > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('(')
		for i := 0; i < cols; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('?')
		}
		sb.WriteByte(')')
		status := n.Status
		if status == "" {
			status = bizcg.NodeStatusActive
		}
		args = append(args,
			n.ID, n.NodeType, n.RefID, n.NodeKey, n.DisplayName, n.WorkspaceID,
			status, attrsJSON(n.Attrs), n.Generation, formatTS(n.CreatedAt), formatTS(n.UpdatedAt),
		)
		wrote++
	}
	if wrote == 0 {
		return nil
	}
	// created_at intentionally not updated on conflict (first write wins).
	sb.WriteString(` ON CONFLICT (node_type, ref_id, generation) DO UPDATE SET node_key=excluded.node_key, display_name=excluded.display_name, workspace_id=excluded.workspace_id, status=excluded.status, attrs_json=excluded.attrs_json, updated_at=excluded.updated_at`)
	q := r.d.RenumberPlaceholders(sb.String())
	if _, err := r.rw.WriteDB(ctx).ExecContext(ctx, q, args...); err != nil {
		return toBizErr(err)
	}
	return nil
}

func (r *repo) UpsertEdges(ctx context.Context, edges []bizcg.StoredEdge) error {
	if r == nil || len(edges) == 0 {
		return nil
	}
	for start := 0; start < len(edges); start += upsertBatchSize {
		batch := edges[start:min(start+upsertBatchSize, len(edges))]
		if err := r.upsertEdgeBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

func (r *repo) upsertEdgeBatch(ctx context.Context, batch []bizcg.StoredEdge) error {
	const cols = 8
	var sb strings.Builder
	sb.WriteString(`INSERT INTO config_graph_edges (id, src_id, dst_id, edge_type, evidence_json, workspace_id, generation, created_at) VALUES `)
	args := make([]any, 0, len(batch)*cols)
	wrote := 0
	for _, e := range batch {
		if e.ID == "" || e.SrcID == "" || e.Type == "" {
			r.lg.Warn("configgraph: skip invalid edge row", loggateway.StepID("config_graph.repo"))
			continue
		}
		if wrote > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('(')
		for i := 0; i < cols; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('?')
		}
		sb.WriteByte(')')
		args = append(args,
			e.ID, e.SrcID, e.DstID, e.Type, evidenceJSON(e.Evidence), e.WorkspaceID, e.Generation, formatTS(e.CreatedAt),
		)
		wrote++
	}
	if wrote == 0 {
		return nil
	}
	sb.WriteString(` ON CONFLICT (src_id, dst_id, edge_type, generation) DO UPDATE SET evidence_json=excluded.evidence_json, workspace_id=excluded.workspace_id`)
	q := r.d.RenumberPlaceholders(sb.String())
	if _, err := r.rw.WriteDB(ctx).ExecContext(ctx, q, args...); err != nil {
		return toBizErr(err)
	}
	return nil
}

func (r *repo) MaxGeneration(ctx context.Context) (int64, error) {
	if r == nil {
		return 0, nil
	}
	var gen int64
	q := r.d.RenumberPlaceholders(`SELECT COALESCE(MAX(generation),0) FROM config_graph_nodes`)
	if err := data.QueryRowScan(ctx, r.rw.ReadDB(ctx), q, nil, &gen); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return 0, nil
		}
		return 0, toBizErr(err)
	}
	return gen, nil
}

func (r *repo) DeleteGenerationBelow(ctx context.Context, belowGen int64) (int64, error) {
	if r == nil || belowGen <= 0 {
		return 0, nil
	}
	var total int64
	for _, table := range []string{"config_graph_edges", "config_graph_nodes"} {
		q := r.d.RenumberPlaceholders(fmt.Sprintf(`DELETE FROM %s WHERE generation < ?`, table))
		res, err := r.rw.WriteDB(ctx).ExecContext(ctx, q, belowGen)
		if err != nil {
			return total, toBizErr(err)
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}

func (r *repo) DeleteOutEdges(ctx context.Context, srcID string, generation int64) error {
	if r == nil || srcID == "" {
		return nil
	}
	q := r.d.RenumberPlaceholders(`DELETE FROM config_graph_edges WHERE src_id=? AND generation=?`)
	if _, err := r.rw.WriteDB(ctx).ExecContext(ctx, q, srcID, generation); err != nil {
		return toBizErr(err)
	}
	return nil
}

// escapeLike escapes LIKE wildcards for substring filters (ESCAPE '\').
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *repo) ListNodes(ctx context.Context, filter bizcg.NodeFilter) ([]bizcg.Node, error) {
	if r == nil {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	var sb strings.Builder
	sb.WriteString(`SELECT id, node_type, ref_id, node_key, display_name, workspace_id, status, attrs_json, generation, created_at, updated_at FROM config_graph_nodes WHERE generation=?`)
	args := []any{filter.Generation}
	if t := strings.TrimSpace(filter.NodeType); t != "" {
		sb.WriteString(` AND node_type=?`)
		args = append(args, t)
	}
	if ws := strings.TrimSpace(filter.WorkspaceID); ws != "" {
		sb.WriteString(` AND workspace_id=?`)
		args = append(args, ws)
	}
	if k := strings.TrimSpace(filter.KeyContains); k != "" {
		sb.WriteString(` AND node_key LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(k)+"%")
	}
	sb.WriteString(` ORDER BY node_type, node_key, id LIMIT ?`)
	args = append(args, limit)
	q := r.d.RenumberPlaceholders(sb.String())
	rows, err := r.rw.ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, toBizErr(err)
	}
	defer rows.Close()
	return scanNodeRows(rows, min(limit, 256))
}

// scanNodeRows scans node rows (column order = nodeSelectCols).
func scanNodeRows(rows *sql.Rows, capHint int) ([]bizcg.Node, error) {
	out := make([]bizcg.Node, 0, capHint)
	for rows.Next() {
		var n bizcg.Node
		var attrs string
		var created, updated sql.NullString
		if err := rows.Scan(
			&n.ID, &n.NodeType, &n.RefID, &n.NodeKey, &n.DisplayName, &n.WorkspaceID,
			&n.Status, &attrs, &n.Generation, &created, &updated,
		); err != nil {
			return nil, toBizErr(err)
		}
		if attrs != "" {
			_ = json.Unmarshal([]byte(attrs), &n.Attrs) // tolerate bad JSON: Attrs stays nil
		}
		n.CreatedAt = parseTS(created)
		n.UpdatedAt = parseTS(updated)
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, toBizErr(err)
	}
	return out, nil
}

func (r *repo) Counts(ctx context.Context, generation int64) (bizcg.Counts, error) {
	if r == nil {
		return bizcg.Counts{}, nil
	}
	var c bizcg.Counts
	q := r.d.RenumberPlaceholders(
		`SELECT (SELECT COUNT(*) FROM config_graph_nodes WHERE generation=?),
		        (SELECT COUNT(*) FROM config_graph_edges WHERE generation=?),
		        (SELECT COUNT(*) FROM config_graph_edges WHERE generation=? AND evidence_json LIKE '` + brokenLike + `')`)
	if err := data.QueryRowScan(ctx, r.rw.ReadDB(ctx), q, []any{generation, generation, generation}, &c.Nodes, &c.Edges, &c.Broken); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return bizcg.Counts{}, nil
		}
		return bizcg.Counts{}, toBizErr(err)
	}
	return c, nil
}
