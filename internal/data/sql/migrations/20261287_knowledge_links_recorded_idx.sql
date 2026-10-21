-- Version 20261287: knowledge_links R 轴读索引（G-05-asof，ADR-KN-20261020-J）
-- ListCollectionLinksAsOf 的 R-only / 双轴象限谓词 recorded_at <= $R AND
-- (valid_to IS NULL OR valid_to > $R)：recorded_at 原本无任何索引，valid_to > $R
-- 分支与 GiST 表达式索引凑不起 BitmapOr → 全库 as-of 图读退化为全表 seq scan。
-- 双时态表历史行只增不删（ReplaceLinks 关版本不删历史），代价随历史线性膨胀。
-- (collection_id, recorded_at) 组合序：两个 AsOf 读者均以 collection_id = $1 前置
-- 等值约束、recorded_at <= $R 驱动范围——单索引同服务 R-only 与双轴象限。
-- Idempotent: "already exists" errors are treated as success by the migration runner (DB-N6).
CREATE INDEX IF NOT EXISTS knowledge_links_recorded_idx
  ON knowledge_links (collection_id, recorded_at);
