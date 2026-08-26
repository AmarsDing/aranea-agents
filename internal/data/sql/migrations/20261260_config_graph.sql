-- Version 20261260: config_graph schema（M81 配置资产图谱与变更影响面，P0-0.1）
-- 配置资产依赖图两张表：
--   config_graph_nodes  12 类资产节点（agent/team/skill/tool/prompt_file/cron_task/
--                       channel/organization/graph/knowledge_collection/mcp_server/hook），
--                       (node_type, ref_id, generation) 为真实唯一键，node_key 仅展示别名；
--   config_graph_edges  27 类引用边，evidence_json 含抽取证据（table/field/path）+
--                       broken 标记（抽取失败/目标从未存在）+ grant_origin 等边属性；
--                       断边 dst_id=''，目标键存 evidence_json.dst_key。
-- generation 双代切换：全量重建写新代，查询恒读当前代（当前代存 system_setting
-- key=config_graph_generation），无清表窗口。
-- 双方言通用（SQLite 风格 DDL，PG 经 translateSQLiteDDLToPostgres 翻译）。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS config_graph_nodes (
  id TEXT PRIMARY KEY,
  node_type TEXT NOT NULL,
  ref_id TEXT NOT NULL DEFAULT '',
  node_key TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  attrs_json TEXT NOT NULL DEFAULT '{}',
  generation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_config_graph_nodes_ref ON config_graph_nodes(node_type, ref_id, generation);
CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_type_status ON config_graph_nodes(node_type, status, generation);
CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_key ON config_graph_nodes(node_key);
CREATE INDEX IF NOT EXISTS idx_config_graph_nodes_ws ON config_graph_nodes(workspace_id);

CREATE TABLE IF NOT EXISTS config_graph_edges (
  id TEXT PRIMARY KEY,
  src_id TEXT NOT NULL DEFAULT '',
  dst_id TEXT NOT NULL DEFAULT '',
  edge_type TEXT NOT NULL,
  evidence_json TEXT NOT NULL DEFAULT '{}',
  workspace_id TEXT NOT NULL DEFAULT '',
  generation INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_config_graph_edges ON config_graph_edges(src_id, dst_id, edge_type, generation);
CREATE INDEX IF NOT EXISTS idx_config_graph_edges_src ON config_graph_edges(src_id, generation);
CREATE INDEX IF NOT EXISTS idx_config_graph_edges_dst ON config_graph_edges(dst_id, generation);
