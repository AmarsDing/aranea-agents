-- Version 20261310: ceoboard_dead_letters 库层幂等兜底部分唯一索引(评审 2026-11-09 M2)。
--
-- 背景:store.AddDeadLetter 原为"dlMu 进程内互斥 + 查重再插两步"——单实例写入者
-- 安全,但双实例/双跑并发时两个写者可同时穿过查重窗口,库层无兜约束会插重
-- (同一未重放业务键多行,重放路由与 Pending 计数失真)。ent schema 只声明了非唯一
-- 的 (kind, source, ref_key) 索引;部分唯一索引无法经 ent model 表达(prod 无
-- DropIndex 不会回收,DEV 模式 auto-migrate WithDropIndex 会抹掉——由启动期
-- reconcile EnsureCeoboardDeadLetterIdemSchema 每次幂等重建,见
-- internal/data/ceoboard_schema.go)。
--
-- 本迁移两步:
--   ① 清理存量重复:每组 (kind, source, ref_key) 未重放行保留 id 最大者——与
--      AddDeadLetter 查重 ORDER BY id DESC LIMIT 1 的既有"最新优先"语义一致;
--   ② 建部分唯一索引:ref_key <> '' 且未重放(replayed_at IS NULL)的行,
--      (kind, source, ref_key) 唯一。
-- ref_key='' 的行(receive 阶段无业务键可关联)不参与幂等,排除在索引外;
-- 重放(replayed_at 落时)后释放该键,允许产生新一轮死信(与 biz 语义一致)。
--
-- 幂等:IF NOT EXISTS;① 的谓词自幂等(已收敛的表重扫为零删除)。双方言(pg/sqlite)
-- 语法一致;storetest SQLite 钩子内置同语法 DDL。重跑安全。
DELETE FROM ceoboard_dead_letters
 WHERE "ref_key" <> '' AND "replayed_at" IS NULL
   AND "id" NOT IN (
     SELECT MAX("id") FROM ceoboard_dead_letters
      WHERE "ref_key" <> '' AND "replayed_at" IS NULL
      GROUP BY "kind", "source", "ref_key"
   );
CREATE UNIQUE INDEX IF NOT EXISTS ceoboard_dead_letters_unreplayed_ref_uidx
  ON ceoboard_dead_letters ("kind", "source", "ref_key")
  WHERE "ref_key" <> '' AND "replayed_at" IS NULL;
