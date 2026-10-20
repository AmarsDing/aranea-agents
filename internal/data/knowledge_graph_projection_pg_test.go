package data

import (
	"context"
	"sort"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// G-33（ADR-KN-20261020-H）宿主投影可见性谓词的真实 PG 行为验证——最小自建
// 表（不依赖 pgvector，本地可绿；全 schema + ANN 集成见 knowledge_graph_test.go
// 系 CI 腿）。谓词红线：卡片表无 visibility 列，可见性一律经宿主文档投影
// 下推同一中央扩点 visibleDocIDsSubquery。行级矩阵：
//   - entity 卡：至少一个宿主文档可见 → 且只出题可见宿主；
//   - edge 卡：doc_id 与 target_doc_id 双端各至少一条可见（沿用 links 双端点
//     纪律）——一端私密整条 edge 卡对外部不可见；
//   - 词法降级：同一谓词（name_norm 精确/前缀 ∪ alias_norm 精确）。
func setupGraphCardPG(t *testing.T) *knowledgeRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	d := &Data{rawDB: db, readDB: db, pg: db, pgRead: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}
	repo := &knowledgeRepo{data: d, lg: loggateway.NewNoop()}
	ctx := context.Background()
	stmts := []string{
		`CREATE TABLE knowledge_collections (id TEXT PRIMARY KEY, embedding_model TEXT NOT NULL DEFAULT '', vault_backend TEXT NOT NULL DEFAULT 'local')`,
		`CREATE TABLE knowledge_documents (
			id            TEXT PRIMARY KEY,
			collection_id TEXT NOT NULL,
			title         TEXT NOT NULL DEFAULT '',
			rel_path      TEXT NOT NULL DEFAULT '',
			visibility    TEXT,
			owner_user_id TEXT   NOT NULL DEFAULT '',
			shared_with   TEXT[] NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE knowledge_entities (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			name          TEXT NOT NULL,
			name_norm     TEXT NOT NULL DEFAULT '',
			entity_type   TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE knowledge_entity_aliases (
			collection_id TEXT NOT NULL,
			entity_id     BIGINT NOT NULL,
			alias_norm    TEXT NOT NULL
		)`,
		`CREATE TABLE knowledge_doc_entities (
			collection_id TEXT NOT NULL,
			doc_id        TEXT NOT NULL,
			entity_id     BIGINT NOT NULL,
			mentions      INT NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE knowledge_links (
			id            BIGSERIAL PRIMARY KEY,
			collection_id TEXT NOT NULL,
			doc_id        TEXT NOT NULL,
			target_doc_id TEXT NOT NULL,
			link_type     TEXT NOT NULL,
			relation      TEXT NOT NULL DEFAULT '',
			context       TEXT NOT NULL DEFAULT '',
			valid_to      TIMESTAMPTZ
		)`,
		`CREATE TABLE knowledge_graph_embeddings (
			kind          TEXT NOT NULL,
			ref_id        BIGINT NOT NULL,
			collection_id TEXT NOT NULL,
			text          TEXT NOT NULL,
			source_hash   TEXT NOT NULL DEFAULT '',
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (kind, ref_id)
		)`,
		`INSERT INTO knowledge_collections (id) VALUES ('c1'), ('lex')`,
		// 文档：pub 集合可见；mine 属主 42 私密；shrA 对 deptA 共享。
		`INSERT INTO knowledge_documents (id, collection_id, title, rel_path, visibility, owner_user_id, shared_with) VALUES
			('pub',  'c1', '公共手册', '',       'collection', '',   '{}'),
			('mine', 'c1', '私密笔记', 'me/a.md', 'private',    '42', '{}'),
			('shrA', 'c1', '部门指南', '',       'shared',     '',   '{deptA}'),
			('t1',   'c1', '目标文档', 'docs/t1.md', 'collection', '', '{}')`,
		// 实体 1=pgvector（pub 提及），实体 2=内部代号（mine 提及），实体 3=灰度平台（pub+shrA 提及）。
		`INSERT INTO knowledge_entities (id, collection_id, name, name_norm, entity_type) VALUES
			(1, 'c1', 'pgvector',  'pgvector', '扩展'),
			(2, 'c1', '内部代号X', '内部代号x', '项目'),
			(3, 'c1', '灰度平台',  '灰度平台', '系统')`,
		`INSERT INTO knowledge_entity_aliases (collection_id, entity_id, alias_norm) VALUES
			('c1', 1, 'vector ext'),
			('c1', 2, 'codex')`,
		`INSERT INTO knowledge_doc_entities (collection_id, doc_id, entity_id, mentions) VALUES
			('c1', 'pub',  1, 3),
			('c1', 'mine', 2, 1),
			('c1', 'pub',  3, 5),
			('c1', 'shrA', 3, 2)`,
		// 边 11=pub→t1（双端集合可见）；边 12=mine→t1（源端私密）。
		`INSERT INTO knowledge_links (id, collection_id, doc_id, target_doc_id, link_type, relation, context, valid_to) VALUES
			(11, 'c1', 'pub',  't1', 'semantic', 'applies-to', '灰度平台 — 先 5% 流量', NULL),
			(12, 'c1', 'mine', 't1', 'semantic', 'depends-on', '内部代号X — 依赖链',   NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return repo
}

func TestGraphCardHostDocs_VisibilityProjectionPG(t *testing.T) {
	repo := setupGraphCardPG(t)
	ctx := context.Background()
	uid42 := auth.NewContext(ctx, &auth.Auth{UserID: 42})
	deptA := bizknowledge.WithOrgScope(ctx, []string{"deptA"})

	assertIDs := func(t *testing.T, name string, c context.Context, kind string, refID int64, want []string) {
		t.Helper()
		limit := bizknowledge.GraphCardEntityHostLimit
		if kind == bizknowledge.GraphCardKindEdge {
			limit = bizknowledge.GraphCardEdgeHostLimit
		}
		got, err := repo.ListGraphCardHostDocs(c, "c1", kind, refID, limit)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sort.Strings(got)
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
		}
	}

	t.Run("entity card only emits visible hosts for stranger", func(t *testing.T) {
		assertIDs(t, "stranger entity/2", ctx, "entity", 2, nil) // 宿主 mine 私密 → 无可见宿主
		assertIDs(t, "owner entity/2", uid42, "entity", 2, []string{"mine"})
		assertIDs(t, "stranger entity/3", ctx, "entity", 3, []string{"pub"})
		assertIDs(t, "deptA entity/3", deptA, "entity", 3, []string{"pub", "shrA"})
	})

	t.Run("edge card requires both endpoints visible", func(t *testing.T) {
		assertIDs(t, "stranger edge/11", ctx, "edge", 11, []string{"pub", "t1"})
		// 边 12 源端 mine 私密 → 对外部整条不可见（双端点纪律）。
		assertIDs(t, "stranger edge/12", ctx, "edge", 12, []string{"t1"})
		assertIDs(t, "owner edge/12", uid42, "edge", 12, []string{"mine", "t1"})
	})
}

func TestGraphCardLexical_VisibilityAndScoringPG(t *testing.T) {
	repo := setupGraphCardPG(t)
	ctx := context.Background()
	uid42 := auth.NewContext(ctx, &auth.Auth{UserID: 42})

	t.Run("exact match beats alias match beats prefix, strangers miss private hosts", func(t *testing.T) {
		hits, err := repo.SearchEntityCardsLexical(ctx, "c1", "pgvector", 8)
		if err != nil {
			t.Fatal(err)
		}
		if len(hits) != 1 || hits[0].RefID != 1 || hits[0].Score != 1.0 {
			t.Fatalf("exact hits = %+v", hits)
		}
		// 卡文与向量档同一构造器（含别名行）。
		if hits[0].Text != "pgvector（扩展）\n别名：vector ext" {
			t.Fatalf("lexical card text = %q", hits[0].Text)
		}
		// 外部代号X宿主私密：陌生人 exact/prefix 均不可见；属主可见。
		if hits, err = repo.SearchEntityCardsLexical(ctx, "c1", "内部代号x", 8); err != nil || len(hits) != 0 {
			t.Fatalf("stranger private-host exact must be invisible: %v %+v", err, hits)
		}
		hits, err = repo.SearchEntityCardsLexical(uid42, "c1", "内部代号x", 8)
		if err != nil || len(hits) != 1 || hits[0].RefID != 2 {
			t.Fatalf("owner exact: %v %+v", err, hits)
		}
	})

	t.Run("alias exact joins the leg", func(t *testing.T) {
		// alias_norm=codex 精确命中实体 2——外部不可见（宿主私密），属主得分 0.9。
		hits, err := repo.SearchEntityCardsLexical(uid42, "c1", "codex", 8)
		if err != nil || len(hits) != 1 || hits[0].RefID != 2 || hits[0].Score != 0.9 {
			t.Fatalf("alias exact: %v %+v", err, hits)
		}
		if hits, err = repo.SearchEntityCardsLexical(ctx, "c1", "codex", 8); err != nil || len(hits) != 0 {
			t.Fatalf("stranger alias exact must be invisible: %v %+v", err, hits)
		}
	})

	t.Run("prefix hit with shared host", func(t *testing.T) {
		deptA := bizknowledge.WithOrgScope(ctx, []string{"deptA"})
		hits, err := repo.SearchEntityCardsLexical(deptA, "c1", "灰度", 8)
		if err != nil || len(hits) != 1 || hits[0].RefID != 3 || hits[0].Score != 0.5 {
			t.Fatalf("prefix: %v %+v", err, hits)
		}
	})
}

// TestGraphCardANN_VisibilityProjectionPG ANN 谓词分支（pgvector 依赖——本地
// 测试容器无扩展则跳过；CI pgvector 全 schema 集成复验）。
func TestGraphCardANN_VisibilityProjectionPG(t *testing.T) {
	repo := setupGraphCardPG(t)
	ctx := context.Background()
	// 同一 schema 钉住的连接上 CREATE EXTENSION（SetupTestPGRaw 每次随机新
	// schema，二次取连接会把扩展建到别的 schema 里）。
	db := repo.data.rawDB
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Skipf("pgvector unavailable (local env debt; CI covers): %v", err)
	}
	// 词法集合不物化卡片；为卡片行手工补 embedding 列（迁移轨道本就由
	// EnsureKnowledgeSchema ALTER 补列，见 20261286 + 双轨纪律）。
	stmts := []string{
		`ALTER TABLE knowledge_graph_embeddings ADD COLUMN embedding vector(3)`,
		`INSERT INTO knowledge_graph_embeddings (kind, ref_id, collection_id, text, source_hash, embedding) VALUES
			('entity', 1, 'c1', 'pgvector（扩展）\n别名：vector ext', 'h1', '[1,0,0]'),
			('entity', 2, 'c1', '内部代号X（项目）\n别名：codex', 'h2', '[0.98,0.2,0]'),
			('edge',   11, 'c1', '灰度平台 applies-to 目标文档 — 先 5% 流量', 'h3', '[0.9,0.3,0]'),
			('edge',   12, 'c1', '内部代号X depends-on 目标文档 — 依赖链', 'h4', '[0.85,0.5,0]')`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	vec := []float32{1, 0, 0}
	// 陌生人：实体 2（宿主私密）与边 12（源端私密）整条不可见。
	hits, err := repo.SearchGraphCards(ctx, "c1", "entity", vec, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].RefID != 1 {
		t.Fatalf("stranger entity ANN = %+v, want 仅实体 1", hits)
	}
	if hits, err = repo.SearchGraphCards(ctx, "c1", "edge", vec, 8); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].RefID != 11 {
		t.Fatalf("stranger edge ANN = %+v, want 仅边 11", hits)
	}
	// 属主：实体 2 命中可见。
	uid42 := auth.NewContext(ctx, &auth.Auth{UserID: 42})
	hits, err = repo.SearchGraphCards(uid42, "c1", "edge", vec, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("owner edge ANN = %+v, want 双端可见 2 条", hits)
	}
}
