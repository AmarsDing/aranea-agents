-- Version 20261298: knowledge_source_connectors（G-38 SourceSync W1，连接器主表）
-- 一表承载连接器身份/调度/退避/预算全状态（37-knowledge.design.md §G-38 ③-1）：
--   - kind 白名单 CHECK（首发 shared_dir/notion，web connector 为二期欠账）；
--   - config_json 仅非密配置（红线：checkpoint 禁放 metadata_json 先例沿承为
--     cursor 独立列；凭据一律走 20261300 行表，绝不入 config_json）；
--   - 退避分级（⑤）：failure 翻 2^consecutive_failures 分钟 cap 60，
--     consecutive_failures/error_code（字节稳定枚举，biz/sourcesync 纯函数钉）/
--     error_message（≤512 截断）原位落库；
--   - due 拾取部分索引（20261293 先例）只覆盖 enabled ∧ 未软删谓词，
--     运行期过滤 sync_state（running 防双跑 / auth_paused 防烧配额）在残余层；
--   - 一库一连接器活性唯一（collection_id 软删感知部分唯一，MCP 20261209
--     ''::text 谓词规范先例）——软删重建天然放行；
--   - run_token 为当前轮认领 CAS（W2 ClaimRun），崩溃回收=lease_ttl 过期置 idle；
--   - 预算三列默认 500/200/512MiB（biz/sourcesync 常量同源，W1 可调参登记）。
-- 时间列沿用 channel_runtime_lease/board_run_leases 先例（TEXT + 定宽
-- '2006-01-02T15:04:05.000000000Z07:00'，字典序==时序，禁 RFC3339Nano）。
-- 编号重锚 20261297（skill D1 占用）+1，无撞号；双轨 EnsureSourceSyncSchema
-- 镜像同 commit。幂等，重跑安全。
CREATE TABLE IF NOT EXISTS knowledge_source_connectors (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('shared_dir', 'notion')),
  name TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NOT NULL DEFAULT '',
  collection_id TEXT NOT NULL DEFAULT '',
  config_json TEXT NOT NULL DEFAULT '{}',
  interval_seconds INTEGER NOT NULL DEFAULT 900,
  enabled BOOLEAN NOT NULL DEFAULT false,
  sync_state TEXT NOT NULL DEFAULT 'idle' CHECK (sync_state IN ('idle', 'running', 'auth_paused', 'error')),
  cursor TEXT NOT NULL DEFAULT '',
  next_run_at TEXT NOT NULL DEFAULT '',
  last_run_at TEXT NOT NULL DEFAULT '',
  last_success_at TEXT NOT NULL DEFAULT '',
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  max_items_per_run INTEGER NOT NULL DEFAULT 500,
  max_deletes_per_run INTEGER NOT NULL DEFAULT 200,
  max_fetch_bytes_per_run BIGINT NOT NULL DEFAULT 536870912,
  run_token TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);
-- due 拾取部分索引（谓词与查询侧字节对齐）。
CREATE INDEX IF NOT EXISTS idx_src_connectors_due
  ON knowledge_source_connectors (next_run_at)
  WHERE enabled AND deleted_at = ''::text;
CREATE INDEX IF NOT EXISTS idx_src_connectors_workspace
  ON knowledge_source_connectors (workspace_id);
-- 一库一连接器活性唯一（软删墓碑行豁免）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_src_connectors_collection_active
  ON knowledge_source_connectors (collection_id)
  WHERE deleted_at = ''::text;
