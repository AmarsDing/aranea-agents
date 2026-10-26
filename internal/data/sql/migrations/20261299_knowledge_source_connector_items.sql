-- Version 20261299: knowledge_source_connector_items（G-38 SourceSync W1，
-- 指纹表——身份寻址唯一真相，P3 拍板 a）
-- (connector_id, external_id)→document_id 是更新/删除的唯一寻址与管辖权边界
--（37-knowledge.design.md §G-38 ①/③-2）：
--   - own_lifecycle=false 标记 adopt 档（红线 3 同库同 hash 不重复建档契约
--     完整保留＝登记 item 但不接管生命周期/不 tombstone）；
--   - content_hash 提交成功才回填=checkpoint 推进点（对齐 vault 落库即
--     checkpoint 契约，失败不推进）；
--   - Upsert 语义钉：出席即 status='active'、miss_count=0、error_message 清空
--     ——flap 边界（缺席→回归→再缺席）不触发删除；
--   - error_message 列：③-2 Upsert 钉明文要求清 error_message，而设计列清单
--     未列该列——按语义钉落列（W1 登记此设计文本内不一致处置）；
--   - 删除候选拾取谓词=partial index WHERE status='missing'；
--   - miss 轮轮计数/last_seen_run_id 支撑 ⑥-3 候选圈定与 miss_unknown
--     连 3 轮>0 告警。
-- 时间列 TEXT 定宽布局先例（见 20261298 主表头注）。
CREATE TABLE IF NOT EXISTS knowledge_source_connector_items (
  connector_id TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  document_id TEXT NOT NULL DEFAULT '',
  fingerprint TEXT NOT NULL DEFAULT '',
  content_hash TEXT NOT NULL DEFAULT '',
  own_lifecycle BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'missing', 'deleted')),
  miss_count INTEGER NOT NULL DEFAULT 0,
  last_seen_run_id TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  first_seen_at TEXT NOT NULL DEFAULT '',
  last_seen_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (connector_id, external_id)
);
-- 删除候选拾取部分索引。
CREATE INDEX IF NOT EXISTS idx_src_items_missing
  ON knowledge_source_connector_items (connector_id)
  WHERE status = 'missing';
-- 文档倒查（UI 徽章判定/级联处置径）。
CREATE INDEX IF NOT EXISTS idx_src_items_document
  ON knowledge_source_connector_items (document_id);
