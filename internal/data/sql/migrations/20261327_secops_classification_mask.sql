-- 20261327 secops_classification_mask (E 阶段 W2b D6，拍板 L-1/L-2/L-4)
-- D6 密级体系 + 脱敏五闸数据面（自 workbench schema.sql 语义平移，需求 §7.3）：
--   classification_labels：L1-L5 密级标签（L3=强制本地），种子同 wb；
--   model_policy：单行路由策略（local_threshold 默认 3；水位线 >= 阈值强制本地），
--     sysadmin 可改（secops.policy 码位）+ 审计；
--   mask_rules/mask_proposals/mask_samples：脱敏五闸（提案→样本试跑→静态检查→
--     人工审批→shadow→activate 灰度），wb 词表列 TEXT[] 收敛为 wordlist_json
--     （双方言可移植）；布尔统一 INTEGER 0/1（memory_facts pii 同例）；
--   sessions/session_runs 增 ceiling 水位线列（0=未定级；GREATEST 只升不降由
--     biz/data 层保证，Dialect.Greatest 可用）。
-- 全部幂等（IF NOT EXISTS / ON CONFLICT DO NOTHING），重跑安全。

-- ── 密级标签（L1-L5；L3=秘密 强制本地模型通道）─────────────────────────────
CREATE TABLE IF NOT EXISTS classification_labels (
    level       INTEGER PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL DEFAULT ''
);
INSERT INTO classification_labels (level, name, description) VALUES
  (1, '公开', '可全员访问'),
  (2, '内部', '内部员工可见'),
  (3, '秘密', '强制本地模型通道'),
  (4, '机密', '严格授权访问'),
  (5, '绝密', '最高管控')
ON CONFLICT (level) DO NOTHING;

-- ── 模型路由策略（单行 id=1；水位线 >= local_threshold → 强制本地通道）────────
CREATE TABLE IF NOT EXISTS model_policy (
    id              INTEGER PRIMARY KEY,
    local_threshold INTEGER NOT NULL DEFAULT 3,
    updated_by      TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL DEFAULT ''
);
INSERT INTO model_policy (id, local_threshold) VALUES (1, 3)
ON CONFLICT (id) DO NOTHING;

-- ── 脱敏规则（运行态；approve 只生成 shadow=1 规则，activate 按灰度放开）──────
CREATE TABLE IF NOT EXISTS mask_rules (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL DEFAULT '',
    entity       TEXT NOT NULL,
    pattern      TEXT NOT NULL DEFAULT '',
    wordlist_json TEXT NOT NULL DEFAULT '[]',
    operator     TEXT NOT NULL DEFAULT 'placeholder',
    enabled      INTEGER NOT NULL DEFAULT 1,
    shadow       INTEGER NOT NULL DEFAULT 0,
    rollout_pct  INTEGER NOT NULL DEFAULT 100,
    version      INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mask_rules_entity ON mask_rules(entity, enabled);

-- ── 脱敏样本（闸2 试跑：positive=1 应被命中 / 0 不应被命中；种子同 wb）────────
CREATE TABLE IF NOT EXISTS mask_samples (
    id       TEXT PRIMARY KEY,
    entity   TEXT NOT NULL,
    text     TEXT NOT NULL,
    positive INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_mask_samples_entity ON mask_samples(entity);
INSERT INTO mask_samples (id, entity, text, positive) VALUES
  ('ms_phone_p1', '手机号', '我的手机号是13812345678，下班打给我', 1),
  ('ms_phone_p2', '手机号', '紧急联系人电话13998765432', 1),
  ('ms_phone_n1', '手机号', '订单号20261103001已发货，共11位数字编号', 0),
  ('ms_idcard_p1', '身份证号', '身份证号110101199003070018，请录入', 1),
  ('ms_proj_p1', '内部项目代号', '本季度重点推进 phoenix-2026 与 PHOENIX 2077 两个项目', 1),
  ('ms_proj_p2', '内部项目代号', '项目代号 PHOENIX-0001 的预算已批复', 1),
  ('ms_proj_n1', '内部项目代号', '凤凰卫视的英文是 Phoenix Television，与内部项目无关', 0)
ON CONFLICT (id) DO NOTHING;

-- ── 脱敏提案（五闸主体；状态机 proposed|sample_failed|static_failed|
--    pending_approval|shadow|rejected|activated）────────────────────────────
CREATE TABLE IF NOT EXISTS mask_proposals (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL DEFAULT '',
    entity              TEXT NOT NULL,
    pattern             TEXT NOT NULL DEFAULT '',
    wordlist_json       TEXT NOT NULL DEFAULT '[]',
    operator            TEXT NOT NULL DEFAULT 'placeholder',
    source              TEXT NOT NULL DEFAULT 'manual',
    status              TEXT NOT NULL DEFAULT 'proposed',
    sample_recall       REAL,
    sample_detail_json  TEXT NOT NULL DEFAULT '',
    static_findings_json TEXT NOT NULL DEFAULT '',
    created_by          TEXT NOT NULL DEFAULT '',
    reviewed_by         TEXT NOT NULL DEFAULT '',
    reviewed_at         TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL DEFAULT '',
    updated_at          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_mask_proposals_status ON mask_proposals(status, created_at);

-- ── 会话/运行水位线列（0=未定级；只升不降由写路径 GREATEST 保证）──────────────
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS ceiling INTEGER NOT NULL DEFAULT 0;
ALTER TABLE session_runs ADD COLUMN IF NOT EXISTS ceiling INTEGER NOT NULL DEFAULT 0;
