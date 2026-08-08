package data

// knowledge 实体治理（G5-F B9~B12）：归一化回填、合并事务、别名路由。
// 合并核心 mergeKnowledgeEntityRows 同时服务于 20261129 迁移（冲突组自动合并）
// 与 MergeEntities repo 方法（手动/RPC 合并），避免逻辑漂移。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/lib/pq"
)

// knowledgeEntityRow 治理用的实体行快照。
type knowledgeEntityRow struct {
	ID       int64
	Name     string // 展示名（首见写法）
	NameNorm string // 归一化名（NormalizeEntityName）
}

// MergeEntities 手动/RPC 合并（G5-F B10）：mergeeIDs 并入 keeperID，与迁移冲突组
// 自动合并共享 mergeKnowledgeEntityRows / rewriteEntityLinkContexts（防逻辑漂移）。
// 幂等：不存在的 mergee 跳过（重跑零重写）；keeper 不存在返回 NotFound；
// keeper 出现在 mergeeIDs 中防御性剔除。
func (r *knowledgeRepo) MergeEntities(ctx context.Context, collectionID string, keeperID int64, mergeeIDs []int64) (bizknowledge.EntityMergeResult, error) {
	var result bizknowledge.EntityMergeResult
	err := r.data.PostgresExecInTx(ctx, func(ctx context.Context, tx *sql.Tx) error {
		keeper, mergees, err := loadKnowledgeEntityMergeRows(ctx, tx, collectionID, keeperID, mergeeIDs)
		if err != nil {
			return err
		}
		if len(mergees) == 0 {
			return nil
		}
		mentions, err := mergeKnowledgeEntityRows(ctx, tx, collectionID, keeper, mergees)
		if err != nil {
			return err
		}
		renames := map[string]string{}
		for _, m := range mergees {
			if m.Name != keeper.Name {
				renames[m.Name] = keeper.Name
			}
		}
		links, err := rewriteEntityLinkContexts(ctx, tx, collectionID, renames)
		if err != nil {
			return err
		}
		result = bizknowledge.EntityMergeResult{
			RewrittenMentions: mentions,
			RewrittenLinks:    links,
			MergedEntities:    len(mergees),
		}
		return nil
	})
	if err != nil {
		return bizknowledge.EntityMergeResult{}, err
	}
	return result, nil
}

// loadKnowledgeEntityMergeRows 装载 keeper + mergees（同 collection 校验；keeper 缺失
// 返回 NotFound；mergeeIDs 去重并剔除 keeper/不存在者）。
func loadKnowledgeEntityMergeRows(ctx context.Context, tx *sql.Tx, collectionID string, keeperID int64, mergeeIDs []int64) (knowledgeEntityRow, []knowledgeEntityRow, error) {
	var keeper knowledgeEntityRow
	err := tx.QueryRowContext(ctx,
		`SELECT id, name, name_norm FROM knowledge_entities WHERE collection_id = $1 AND id = $2`,
		collectionID, keeperID).Scan(&keeper.ID, &keeper.Name, &keeper.NameNorm)
	if errors.Is(err, sql.ErrNoRows) {
		return keeper, nil, apierror.NotFound(apierror.DomainKnowledge, "keeper entity not found")
	}
	if err != nil {
		return keeper, nil, fmt.Errorf("load keeper %d: %w", keeperID, err)
	}
	want := make(map[int64]bool, len(mergeeIDs))
	for _, id := range mergeeIDs {
		if id > 0 && id != keeperID {
			want[id] = true
		}
	}
	if len(want) == 0 {
		return keeper, nil, nil
	}
	ids := make([]int64, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, name, name_norm FROM knowledge_entities WHERE collection_id = $1 AND id = ANY($2) ORDER BY id`,
		collectionID, pq.Array(ids))
	if err != nil {
		return keeper, nil, fmt.Errorf("load mergees: %w", err)
	}
	defer rows.Close()
	var mergees []knowledgeEntityRow
	for rows.Next() {
		var m knowledgeEntityRow
		if err := rows.Scan(&m.ID, &m.Name, &m.NameNorm); err != nil {
			return keeper, nil, fmt.Errorf("scan mergee: %w", err)
		}
		mergees = append(mergees, m)
	}
	return keeper, mergees, rows.Err()
}

// ListEntities 列出库内全部实体字典条目（按 id 有序），供合并建议计算（B11）。
func (r *knowledgeRepo) ListEntities(ctx context.Context, collectionID string) ([]bizknowledge.Entity, error) {
	rows, err := r.data.PostgresRead().QueryContext(ctx,
		`SELECT id, name, name_norm, entity_type FROM knowledge_entities WHERE collection_id = $1 ORDER BY id`,
		collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bizknowledge.Entity
	for rows.Next() {
		var e bizknowledge.Entity
		if err := rows.Scan(&e.ID, &e.Name, &e.NameNorm, &e.EntityType); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// sqlEntityQuerier 抽象 *sql.DB / *sql.Tx 共有操作（迁移与 repo 合并共用）。
type sqlEntityQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// mergeKnowledgeEntityRows 把 mergees 并入 keeper（同一 collection 内）：
// 提及重写（(doc_id, entity_id) 冲突时 mentions 求和）→ mergee 既有别名过户
// （与 keeper 既有别名碰撞的删除，keeper 为准）→ mergee name_norm 落 keeper 别名
// （与 keeper norm 相同者跳过：精确命中已覆盖，别名冗余）→ 删除 mergee。
// 返回重写提及条数。entity 轨链接 context 重写由调用方批量进行
// （rewriteEntityLinkContexts，多次合并共享一遍扫描）。
func mergeKnowledgeEntityRows(ctx context.Context, q sqlEntityQuerier, collectionID string, keeper knowledgeEntityRow, mergees []knowledgeEntityRow) (int, error) {
	if len(mergees) == 0 {
		return 0, nil
	}
	ids := make([]int64, len(mergees))
	for i, m := range mergees {
		ids[i] = m.ID
	}
	res, err := q.ExecContext(ctx, `
INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions)
SELECT collection_id, doc_id, $1, mentions FROM knowledge_doc_entities
WHERE collection_id = $2 AND entity_id = ANY($3)
ON CONFLICT (doc_id, entity_id) DO UPDATE SET mentions = knowledge_doc_entities.mentions + EXCLUDED.mentions`,
		keeper.ID, collectionID, pq.Array(ids))
	if err != nil {
		return 0, fmt.Errorf("rewrite mentions: %w", err)
	}
	rewritten, _ := res.RowsAffected()
	if _, err := q.ExecContext(ctx,
		`DELETE FROM knowledge_doc_entities WHERE collection_id = $1 AND entity_id = ANY($2)`,
		collectionID, pq.Array(ids)); err != nil {
		return 0, fmt.Errorf("delete moved mentions: %w", err)
	}
	// 别名过户：mergee 名下与 keeper 既有别名同 norm 的行删除（keeper 为准），其余转指 keeper。
	if _, err := q.ExecContext(ctx, `
DELETE FROM knowledge_entity_aliases a
WHERE a.collection_id = $1 AND a.entity_id = ANY($2)
  AND EXISTS (SELECT 1 FROM knowledge_entity_aliases b
              WHERE b.collection_id = a.collection_id AND b.alias_norm = a.alias_norm AND b.entity_id = $3)`,
		collectionID, pq.Array(ids), keeper.ID); err != nil {
		return 0, fmt.Errorf("drop colliding aliases: %w", err)
	}
	if _, err := q.ExecContext(ctx, `
UPDATE knowledge_entity_aliases SET entity_id = $3
WHERE collection_id = $1 AND entity_id = ANY($2)`,
		collectionID, pq.Array(ids), keeper.ID); err != nil {
		return 0, fmt.Errorf("transfer aliases: %w", err)
	}
	// mergee 写法落 keeper 别名：后续抽取（别名命中）跨同步持久路由到 keeper。
	for _, m := range mergees {
		if m.NameNorm == "" || m.NameNorm == keeper.NameNorm {
			continue
		}
		if _, err := q.ExecContext(ctx, `
INSERT INTO knowledge_entity_aliases (collection_id, entity_id, alias_norm)
VALUES ($1,$2,$3)
ON CONFLICT (collection_id, alias_norm) DO UPDATE SET entity_id = EXCLUDED.entity_id`,
			collectionID, keeper.ID, m.NameNorm); err != nil {
			return 0, fmt.Errorf("insert alias %q: %w", m.NameNorm, err)
		}
	}
	if _, err := q.ExecContext(ctx,
		`DELETE FROM knowledge_entities WHERE collection_id = $1 AND id = ANY($2)`,
		collectionID, pq.Array(ids)); err != nil {
		return 0, fmt.Errorf("delete mergees: %w", err)
	}
	return int(rewritten), nil
}

// rewriteEntityLinkContexts 把 entity 轨链接 context（共享实体展示名逗号分隔）
// 中的旧名替换为 keeper 名（替换后去重保持原序）。返回重写链接条数。
func rewriteEntityLinkContexts(ctx context.Context, q sqlEntityQuerier, collectionID string, renames map[string]string) (int, error) {
	if len(renames) == 0 {
		return 0, nil
	}
	rows, err := q.QueryContext(ctx,
		`SELECT id, context FROM knowledge_links
		 WHERE collection_id = $1 AND link_type = 'entity' AND context <> ''`, collectionID)
	if err != nil {
		return 0, fmt.Errorf("scan entity links: %w", err)
	}
	type update struct {
		id      int64
		context string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var linkCtx string
		if err := rows.Scan(&id, &linkCtx); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan entity link: %w", err)
		}
		if renamed := renameEntityContext(linkCtx, renames); renamed != linkCtx {
			updates = append(updates, update{id, renamed})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate entity links: %w", err)
	}
	rows.Close()
	for _, u := range updates {
		if _, err := q.ExecContext(ctx,
			`UPDATE knowledge_links SET context = $2 WHERE id = $1`, u.id, u.context); err != nil {
			return 0, fmt.Errorf("rewrite link %d context: %w", u.id, err)
		}
	}
	return len(updates), nil
}

// renameEntityContext 纯函数：拆分 → 旧名映射 keeper 名 → 去重 → 重组。
func renameEntityContext(linkCtx string, renames map[string]string) string {
	parts := strings.Split(linkCtx, ",")
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if keeper, ok := renames[p]; ok {
			p = keeper
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, ",")
}

// backfillKnowledgeEntityNameNorms Go 侧回填 name_norm 并自动合并归一化冲突组
// （keeper = id 最小者；PG 无 NFKC，不可 DB 侧回填）。名归一化为空的垃圾行
// 连带提及删除（无治理价值）。返回每库展示名重写映射（旧名→keeper 名，
// 供链接 context 一遍扫描重写）与累计重写提及条数。
func backfillKnowledgeEntityNameNorms(ctx context.Context, tx *sql.Tx, lg loggateway.Logger) (map[string]map[string]string, int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, collection_id, name FROM knowledge_entities ORDER BY collection_id, id`)
	if err != nil {
		return nil, 0, fmt.Errorf("backfill name_norm: scan entities: %w", err)
	}
	type groupKey struct {
		collectionID string
		norm         string
	}
	groups := map[groupKey][]knowledgeEntityRow{}
	var order []groupKey
	var garbage []int64
	for rows.Next() {
		var r knowledgeEntityRow
		var collectionID string
		if err := rows.Scan(&r.ID, &collectionID, &r.Name); err != nil {
			rows.Close()
			return nil, 0, fmt.Errorf("backfill name_norm: scan row: %w", err)
		}
		r.NameNorm = bizknowledge.NormalizeEntityName(r.Name)
		if r.NameNorm == "" {
			garbage = append(garbage, r.ID)
			continue
		}
		k := groupKey{collectionID, r.NameNorm}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, 0, fmt.Errorf("backfill name_norm: iterate: %w", err)
	}
	rows.Close()

	renames := map[string]map[string]string{}
	totalMentions := 0
	for _, k := range order {
		members := groups[k]
		keeper := members[0] // ORDER BY id：组内首个即 id 最小者
		if _, err := tx.ExecContext(ctx,
			`UPDATE knowledge_entities SET name_norm = $2 WHERE id = $1`, keeper.ID, k.norm); err != nil {
			return nil, 0, fmt.Errorf("backfill name_norm for entity %d: %w", keeper.ID, err)
		}
		if len(members) == 1 {
			continue
		}
		n, err := mergeKnowledgeEntityRows(ctx, tx, k.collectionID, keeper, members[1:])
		if err != nil {
			return nil, 0, fmt.Errorf("auto-merge norm group %q: %w", k.norm, err)
		}
		totalMentions += n
		if renames[k.collectionID] == nil {
			renames[k.collectionID] = map[string]string{}
		}
		for _, m := range members[1:] {
			renames[k.collectionID][m.Name] = keeper.Name
		}
	}
	if len(garbage) > 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_doc_entities WHERE entity_id = ANY($1)`, pq.Array(garbage)); err != nil {
			return nil, 0, fmt.Errorf("delete garbage entity mentions: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM knowledge_entities WHERE id = ANY($1)`, pq.Array(garbage)); err != nil {
			return nil, 0, fmt.Errorf("delete garbage entities: %w", err)
		}
		lg.Warn("knowledge entity governance: dropped un-normalizable entity names",
			loggateway.StepID("data.ddl_migration.knowledge_entity_governance"),
			loggateway.Int("dropped", len(garbage)))
	}
	return renames, totalMentions, nil
}

// ddlKnowledgeEntityGovernance G5-F（B9/B12）迁移：
//  1. 结构补丁（SQL 文件）：name_norm 列 + knowledge_entity_aliases 表 + 废 name 唯一约束；
//  2. Go 回填 name_norm（PG 无 NFKC，不可 DB 侧回填）+ 冲突组自动合并（keeper=id 最小者）；
//  3. entity 轨链接 context 展示名重写；
//  4. 建 (collection_id, name_norm) 唯一索引（必须先于 ReplaceDocEntities 的
//     ON CONFLICT 路径上线；回填前建会因冲突组失败）。
//
// fresh 库整体跳过（knowledge_entities 由 EnsureKnowledgeSchema 以新形态创建）；
// Postgres-only（knowledge 依赖 pgvector）。幂等：IF NOT EXISTS + 合并确定性。
func ddlKnowledgeEntityGovernance(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		return nil
	}
	exists, err := d.TableExists(ctx, rawDB, "knowledge_entities")
	if err != nil {
		return fmt.Errorf("knowledge entity governance: check table: %w", err)
	}
	if !exists {
		lg.Info("knowledge entity governance skipped (fresh db)",
			loggateway.StepID("data.ddl_migration.knowledge_entity_governance"))
		return nil
	}
	if err := executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261129_knowledge_entity_governance.sql", d, lg); err != nil {
		return err
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("knowledge entity governance: begin tx: %w", err)
	}
	defer tx.Rollback()
	renames, mentions, err := backfillKnowledgeEntityNameNorms(ctx, tx, lg)
	if err != nil {
		return err
	}
	links := 0
	for collectionID, rm := range renames {
		n, err := rewriteEntityLinkContexts(ctx, tx, collectionID, rm)
		if err != nil {
			return err
		}
		links += n
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("knowledge entity governance: commit: %w", err)
	}
	if _, err := rawDB.ExecContext(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS knowledge_entities_name_norm_key
	ON knowledge_entities(collection_id, name_norm)`); err != nil {
		return fmt.Errorf("knowledge entity governance: unique index: %w", err)
	}
	lg.Info("knowledge entity governance applied",
		loggateway.StepID("data.ddl_migration.knowledge_entity_governance"),
		loggateway.Int("rewritten_mentions", mentions),
		loggateway.Int("rewritten_links", links))
	return nil
}
