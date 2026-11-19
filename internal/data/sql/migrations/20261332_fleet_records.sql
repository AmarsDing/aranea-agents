-- Version 20261332: Harness 多任务运行台（fleet，复刻 Orca 工作方式）的状态存储。
--
-- 单表 JSON 文档：kind ∈ workspace | note | event | run | task | message | automation，
-- data 为该记录的 JSON。运行台状态量小（几十到几百条）、结构随功能迭代，
-- 与 Orca 本身用 JSON 文件存状态同理；查询全部在 biz 层内存过滤。
-- 终端进程是运行态，不落库（服务重启后终端随之结束）。
--
-- 幂等：IF NOT EXISTS，重跑安全。

CREATE TABLE IF NOT EXISTS fleet_records (
  "kind"       TEXT NOT NULL,
  "id"         TEXT NOT NULL,
  "data"       TEXT NOT NULL,
  "created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  "updated_at" TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY ("kind", "id")
);
