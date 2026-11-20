-- 20261337 knowledge_collection_space（工作台 › 资料 P1，设计
-- docs/aranea-agents/design/2026-11-17-三大模块重设计/10-资料.md §3.1、§4）
--
-- knowledge_collections.space：资料库归在哪个空间——mine（我的资料）/ company（公司资料）。
-- 空 = 未登记，按后端归：local → mine，team → company（biz.EffectiveSpace），存量行不回填。
-- 幂等：IF NOT EXISTS。

ALTER TABLE knowledge_collections ADD COLUMN IF NOT EXISTS space TEXT NOT NULL DEFAULT '';
