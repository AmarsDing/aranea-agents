-- Version 20261311: ceoboard_webhook_deliveries 投递幂等维度化 (source, key)
-- (评审 2026-11-09 MEDIUM-7)。
--
-- 背景:原存储主键为全局幂等键,无 source 维度。adapter 侧幂等键取自优先级链
-- (强制键 > Idempotency-Key 头 > body.idempotency_key > body.event_id > 实体 id
-- > body 摘要),键本身不含 source——不同 webhook 源产生相同键(同 event_id /
-- 同实体 id / 同 body 摘要回退)时跨源撞键,后到的投递被静默去重(事件丢失)。
--
-- 新模型:存储主键 = source || ':' || 幂等键 复合串(拼接收口在 store 层
-- deliveryStorageID,与 ceoboard_events 的 event_uid = 平台:键 同构;读侧剥前缀
-- 还原,包外透明)。本迁移回填存量裸键行为复合键:
--   ① 复制为复合键行——INSERT OR IGNORE(pg 侧翻译为 ON CONFLICT DO NOTHING)
--      兜底病态撞名(如源恰名 "sha256" 与摘要回退键相撞),冲突行保留原文不覆盖;
--      长度 guard 限制复合串不超 key 列 MaxLen(512)(超长残留裸键就读,见下);
--   ② 仅删除"已成功复制"的旧裸键行(EXISTS 复合孪生才删)——复制失败的行
--      绝不误删。
-- 未回填的遗留裸键行:读取侧剥前缀逻辑对无前缀行恒等返回,语义不破;其重复投递
-- 会以复合键新建一行——升级为源内幂等前的最后一次性重复,不再跨源丢事件。
--
-- 幂等:已带自身复合前缀的行被 guard 排除(substr 前缀比对),重跑安全。
-- 双方言(pg/sqlite)语法一致(substr/length/|| 两库同名同义)。
INSERT OR IGNORE INTO ceoboard_webhook_deliveries
  ("key", "source", "event", "entity_id", "at", "digest", "fields", "status", "error")
  SELECT "source" || ':' || "key", "source", "event", "entity_id", "at", "digest", "fields", "status", "error"
    FROM ceoboard_webhook_deliveries
   WHERE substr("key", 1, length("source") + 1) <> "source" || ':'
     AND length("source") + 1 + length("key") <= 512;
DELETE FROM ceoboard_webhook_deliveries
 WHERE substr("key", 1, length("source") + 1) <> "source" || ':'
   AND EXISTS (
     SELECT 1 FROM ceoboard_webhook_deliveries AS n
      WHERE n."key" = ceoboard_webhook_deliveries."source" || ':' || ceoboard_webhook_deliveries."key"
   );
