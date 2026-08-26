package configgraph

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/data"
	"aranea-agents/pkg/apierror"

	bizcg "aranea-agents/internal/biz/configgraph"

	"github.com/lib/pq"
)

// walkRowLimit caps recursive-CTE output（NFR-81-02 行数保护：稠密图多路径
// 行爆炸时截断，P95 < 200ms 靠 depth 默认 3 + 行上限双保险）。
const walkRowLimit = 2000

// nodeSelectCols 是 config_graph_nodes 的查询列（与 scanNode 扫描顺序一致）。
const nodeSelectCols = `id, node_type, ref_id, node_key, display_name, workspace_id, status, attrs_json, generation, created_at, updated_at`

// edgeSelectCols 是 config_graph_edges 的查询列（与 scanEdgeRows 扫描顺序一致）。
const edgeSelectCols = `id, src_id, dst_id, edge_type, evidence_json, workspace_id, generation, created_at`

func (r *repo) FindNode(ctx context.Context, generation int64, nodeType, ref string) (bizcg.Node, error) {
	if r == nil || nodeType == "" || ref == "" {
		return bizcg.Node{}, apierror.NotFound(domain, "node not found")
	}
	// 双解：ref_id 命中优先于 node_key（NodeIndex 语义：ref_id 是权威身份）。
	q := r.d.RenumberPlaceholders(
		`SELECT ` + nodeSelectCols + ` FROM config_graph_nodes
		 WHERE generation=? AND node_type=? AND (ref_id=? OR node_key=?)
		 ORDER BY CASE WHEN ref_id=? THEN 0 ELSE 1 END, node_key LIMIT 1`)
	var n bizcg.Node
	var attrs string
	var created, updated sql.NullString
	err := data.QueryRowScan(ctx, r.rw.ReadDB(ctx), q,
		[]any{generation, nodeType, ref, ref, ref},
		&n.ID, &n.NodeType, &n.RefID, &n.NodeKey, &n.DisplayName, &n.WorkspaceID,
		&n.Status, &attrs, &n.Generation, &created, &updated)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return bizcg.Node{}, apierror.NotFound(domain, "node not found: %s/%s", nodeType, ref)
		}
		return bizcg.Node{}, toBizErr(err)
	}
	if attrs != "" {
		_ = json.Unmarshal([]byte(attrs), &n.Attrs) // tolerate bad JSON: Attrs stays nil
	}
	n.CreatedAt = parseTS(created)
	n.UpdatedAt = parseTS(updated)
	return n, nil
}

// impactWalkSQL 反向闭包 CTE（design §5.1）：path 数组防环（起始节点入
// path，环回原点被截断）；broken 边（dst_id=”）不进 CTE。via 从目标向外
// 累积。参数序：startID(path)、startID(dst_id)、gen、gen、maxDepth、gen。
// var 而非 const：拼接 walkRowLimit（fmt.Sprint 非常量表达式）。
var impactWalkSQL = `WITH RECURSIVE reach AS (
  SELECT e.id, e.src_id, e.dst_id, e.edge_type, e.evidence_json, e.workspace_id, e.generation, e.created_at,
         1 AS depth, ARRAY[e.edge_type]::text[] AS via, ARRAY[?::text, e.src_id] AS path
  FROM config_graph_edges e
  WHERE e.dst_id = ? AND e.generation = ? AND e.dst_id <> ''
  UNION ALL
  SELECT e.id, e.src_id, e.dst_id, e.edge_type, e.evidence_json, e.workspace_id, e.generation, e.created_at,
         r.depth + 1, r.via || e.edge_type, r.path || e.src_id
  FROM config_graph_edges e
  JOIN reach r ON e.dst_id = r.src_id
  WHERE e.generation = ? AND e.dst_id <> '' AND r.depth < ?
    AND NOT (e.src_id = ANY(r.path))
)
SELECT r.id, r.src_id, r.dst_id, r.edge_type, r.evidence_json, r.workspace_id, r.generation, r.created_at,
       r.depth, r.via,
       n.id, n.node_type, n.ref_id, n.node_key, n.display_name, n.workspace_id, n.status, n.attrs_json, n.generation, n.created_at, n.updated_at
FROM reach r
JOIN config_graph_nodes n ON n.id = r.src_id AND n.generation = ?
LIMIT ` + fmt.Sprint(walkRowLimit)

// dependenciesWalkSQL 正向闭包 CTE（design §5.2）：同构反向，沿 src→dst 走，
// 到达节点为 dst 侧。参数序同 impactWalkSQL。
var dependenciesWalkSQL = `WITH RECURSIVE reach AS (
  SELECT e.id, e.src_id, e.dst_id, e.edge_type, e.evidence_json, e.workspace_id, e.generation, e.created_at,
         1 AS depth, ARRAY[e.edge_type]::text[] AS via, ARRAY[?::text, e.dst_id] AS path
  FROM config_graph_edges e
  WHERE e.src_id = ? AND e.generation = ? AND e.dst_id <> ''
  UNION ALL
  SELECT e.id, e.src_id, e.dst_id, e.edge_type, e.evidence_json, e.workspace_id, e.generation, e.created_at,
         r.depth + 1, r.via || e.edge_type, r.path || e.dst_id
  FROM config_graph_edges e
  JOIN reach r ON e.src_id = r.dst_id
  WHERE e.generation = ? AND e.dst_id <> '' AND r.depth < ?
    AND NOT (e.dst_id = ANY(r.path))
)
SELECT r.id, r.src_id, r.dst_id, r.edge_type, r.evidence_json, r.workspace_id, r.generation, r.created_at,
       r.depth, r.via,
       n.id, n.node_type, n.ref_id, n.node_key, n.display_name, n.workspace_id, n.status, n.attrs_json, n.generation, n.created_at, n.updated_at
FROM reach r
JOIN config_graph_nodes n ON n.id = r.dst_id AND n.generation = ?
LIMIT ` + fmt.Sprint(walkRowLimit)

func (r *repo) WalkGraph(ctx context.Context, generation int64, startID string, reverse bool, maxDepth int) ([]bizcg.WalkRow, error) {
	if r == nil || startID == "" || maxDepth <= 0 {
		return nil, nil
	}
	raw := dependenciesWalkSQL
	if reverse {
		raw = impactWalkSQL
	}
	q := r.d.RenumberPlaceholders(raw)
	rows, err := r.rw.ReadDB(ctx).QueryContext(ctx, q, startID, startID, generation, generation, maxDepth, generation)
	if err != nil {
		return nil, toBizErr(err)
	}
	defer rows.Close()
	out := make([]bizcg.WalkRow, 0, 128)
	for rows.Next() {
		var (
			e                bizcg.StoredEdge
			ev               string
			created          sql.NullString
			depth            int
			via              pq.StringArray
			n                bizcg.Node
			attrs            string
			ncreate, nupdate sql.NullString
		)
		if err := rows.Scan(
			&e.ID, &e.SrcID, &e.DstID, &e.Type, &ev, &e.WorkspaceID, &e.Generation, &created,
			&depth, &via,
			&n.ID, &n.NodeType, &n.RefID, &n.NodeKey, &n.DisplayName, &n.WorkspaceID,
			&n.Status, &attrs, &n.Generation, &ncreate, &nupdate,
		); err != nil {
			return nil, toBizErr(err)
		}
		if ev != "" {
			_ = json.Unmarshal([]byte(ev), &e.Evidence)
		}
		e.CreatedAt = parseTS(created)
		if attrs != "" {
			_ = json.Unmarshal([]byte(attrs), &n.Attrs)
		}
		n.CreatedAt = parseTS(ncreate)
		n.UpdatedAt = parseTS(nupdate)
		out = append(out, bizcg.WalkRow{Edge: e, Node: n, Depth: depth, Via: []string(via)})
	}
	if err := rows.Err(); err != nil {
		return nil, toBizErr(err)
	}
	return out, nil
}

// scanEdgeRows scans edge rows (column order = edgeSelectCols).
func scanEdgeRows(rows *sql.Rows) ([]bizcg.StoredEdge, error) {
	out := make([]bizcg.StoredEdge, 0, 64)
	for rows.Next() {
		var e bizcg.StoredEdge
		var ev string
		var created sql.NullString
		if err := rows.Scan(&e.ID, &e.SrcID, &e.DstID, &e.Type, &ev, &e.WorkspaceID, &e.Generation, &created); err != nil {
			return nil, toBizErr(err)
		}
		if ev != "" {
			_ = json.Unmarshal([]byte(ev), &e.Evidence)
		}
		e.CreatedAt = parseTS(created)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, toBizErr(err)
	}
	return out, nil
}

func (r *repo) queryEdges(ctx context.Context, where string, args ...any) ([]bizcg.StoredEdge, error) {
	q := r.d.RenumberPlaceholders(`SELECT ` + edgeSelectCols + ` FROM config_graph_edges WHERE ` + where)
	rows, err := r.rw.ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, toBizErr(err)
	}
	defer rows.Close()
	return scanEdgeRows(rows)
}

func (r *repo) ListNodeEdges(ctx context.Context, generation int64, nodeID string) (out, in, broken []bizcg.StoredEdge, err error) {
	if r == nil || nodeID == "" {
		return nil, nil, nil, nil
	}
	if out, err = r.queryEdges(ctx,
		`generation=? AND src_id=? AND dst_id<>'' ORDER BY edge_type, dst_id`, generation, nodeID); err != nil {
		return nil, nil, nil, err
	}
	if in, err = r.queryEdges(ctx,
		`generation=? AND dst_id=? ORDER BY edge_type, src_id`, generation, nodeID); err != nil {
		return nil, nil, nil, err
	}
	if broken, err = r.queryEdges(ctx,
		`generation=? AND src_id=? AND dst_id='' ORDER BY edge_type`, generation, nodeID); err != nil {
		return nil, nil, nil, err
	}
	return out, in, broken, nil
}

func (r *repo) ListBrokenEdgesTargeting(ctx context.Context, generation int64, keys []string) ([]bizcg.StoredEdge, error) {
	if r == nil || len(keys) == 0 {
		return nil, nil
	}
	// dst_key LIKE 探针（与 brokenLike 同先例：v1 文本匹配，jsonb 后续优化）。
	// key 经 LIKE 转义防通配符注入；key 含 JSON 转义字符（引号/反斜杠）时
	// 探针可能漏配——抽取侧 key 均为 slug/uuid，v1 接受并文档化。
	var sb strings.Builder
	sb.WriteString(`generation=? AND dst_id='' AND evidence_json LIKE '` + brokenLike + `' AND (`)
	args := []any{generation}
	for i, k := range keys {
		if k == "" {
			continue
		}
		if len(args) > 1 {
			sb.WriteString(` OR `)
		}
		_ = i
		sb.WriteString(`evidence_json LIKE ? ESCAPE '\'`)
		args = append(args, `%"dst_key":"`+escapeLike(k)+`"%`)
	}
	if len(args) == 1 {
		return nil, nil // 全部 key 为空
	}
	sb.WriteString(`) ORDER BY edge_type, src_id`)
	return r.queryEdges(ctx, sb.String(), args...)
}

// ListAllEdges 全量边扫描（health 分析用，design §5.4 千级规模内存计算；
// 与 WalkGraph 的 walkRowLimit 不同——此处故意不设行上限）。
func (r *repo) ListAllEdges(ctx context.Context, generation int64) ([]bizcg.StoredEdge, error) {
	if r == nil {
		return nil, nil
	}
	return r.queryEdges(ctx, `generation=? ORDER BY src_id, edge_type, dst_id`, generation)
}

// ListAllNodes 全量节点扫描（health 分析用；不设行上限，同 ListAllEdges）。
func (r *repo) ListAllNodes(ctx context.Context, generation int64) ([]bizcg.Node, error) {
	if r == nil {
		return nil, nil
	}
	q := r.d.RenumberPlaceholders(
		`SELECT ` + nodeSelectCols + ` FROM config_graph_nodes WHERE generation=? ORDER BY node_type, node_key, id`)
	rows, err := r.rw.ReadDB(ctx).QueryContext(ctx, q, generation)
	if err != nil {
		return nil, toBizErr(err)
	}
	defer rows.Close()
	return scanNodeRows(rows, 512)
}

func (r *repo) CountActiveSessions(ctx context.Context, agentIDs, teamIDs []string) (int64, error) {
	if r == nil || (len(agentIDs) == 0 && len(teamIDs) == 0) {
		return 0, nil
	}
	// design §5.1 signals：sessions WHERE agent_id/team_id IN (...) AND
	// deleted_at='' AND status NOT IN ('archived')。空数组 ANY 恒 false，
	// 无需动态拼 SQL。
	q := r.d.RenumberPlaceholders(
		`SELECT COUNT(*) FROM sessions
		 WHERE deleted_at='' AND status NOT IN ('archived')
		   AND (agent_id = ANY(?) OR team_id = ANY(?))`)
	var n int64
	if err := data.QueryRowScan(ctx, r.rw.ReadDB(ctx), q,
		[]any{pq.Array(agentIDs), pq.Array(teamIDs)}, &n); err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return 0, nil
		}
		return 0, toBizErr(err)
	}
	return n, nil
}
