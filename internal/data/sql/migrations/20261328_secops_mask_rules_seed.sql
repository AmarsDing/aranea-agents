-- 20261328 secops_mask_rules_seed（E 阶段 W2b D6 L-2 脱敏闸基线规则）
--
-- 为什么需要种子：V3 BeforeModel 脱敏钩子的 secondCheck 是**独立于规则配置**
-- 的内置识别器（防规则漏配），命中即 fail-closed 拒绝上云。若 mask_rules
-- 全空，任何含手机号/身份证/银行卡/邮箱的云通道文本都会硬失败——基线路由
-- 不可用。workbench 原系统未带种子（由管理员经五闸流程逐步配置），aranea
-- 作为新产品给出厂默认覆盖四类高频 PII；后续变更仍走五闸
--（提案→样本试跑→静态检查→人工审批→shadow→activate 灰度，V4 纵列）。
--
-- 顺序语义：ListEnabledMaskRules 按 id ASC 返回，规则依序应用。
--   mr_01 身份证（18 位含校验位构造）必须先于 mr_03 银行卡（\d{16,19}），
--   否则 18 位身份证会被更宽的银行卡正则抢先替换（实体标签失真，虽仍脱敏）。
--   mr_02 手机号 11 位与两者不重叠。
-- 实体名与 mask_samples 种子（20261327）对齐，V4 闸2 样本试跑按实体取规则。
--
-- 全部幂等（ON CONFLICT DO NOTHING），重跑安全；双方言一致
-- （standard_conforming_strings 下反斜杠为字面量，PG/SQLite 同义）。

INSERT INTO mask_rules (id, name, entity, pattern, wordlist_json, operator, enabled, shadow, rollout_pct, version)
VALUES
  ('mr_01_idcard',  '身份证号（出厂默认）', '身份证号',  '\b\d{17}[\dXx]\b', '[]', 'placeholder', 1, 0, 100, 1),
  ('mr_02_phone',   '手机号（出厂默认）',   '手机号',    '\b1[3-9]\d{9}\b', '[]', 'placeholder', 1, 0, 100, 1),
  ('mr_03_bankcard','银行卡号（出厂默认）', '银行卡号',  '\b\d{16,19}\b', '[]', 'placeholder', 1, 0, 100, 1),
  ('mr_04_email',   '邮箱（出厂默认）',     '邮箱',      '\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b', '[]', 'placeholder', 1, 0, 100, 1)
ON CONFLICT (id) DO NOTHING;
