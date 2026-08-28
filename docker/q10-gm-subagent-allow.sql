-- Q10-GM 组队工具载体（session-eval-20260827 / optimization-package.md，用户裁定 C+A 组合）
--
-- 根因（campaign-report P10 / S06 P1）：GM read_only profile 下无组队/派发工具载体，
-- 只能文字 Brief，S06/S08 两次「嘴派不发」（言行不一）。
--
-- 方案（C+A 组合）：
--   A) allow JSON 授 subagents 四件作分身执行兜底——治理钳制只改 profile 不动
--      allow（ClampSpecialistToolFace），profile_eff 仍 read_only 不触发
--      GOV_NOT_READONLY；subagents_* 非 R17 Spirit 保留件；种子 enabled=false
--      但属 registryOptInOnlyKeys，allow 命名即生效。auto-split 会把它们并入
--      deferred 目录 cue（GM tools_deferred_json 为空），tool_load 按需激活，
--      不污染 GM 6.6K 精简常驻面（P11 正面成果保持）。
--   C) prompt 言行一致：新增「能力边界」段——分身=自身克隆而非部门主管，禁止
--      虚构「已派发部门」，需部门执行时给路由建议（部门主管会话/精灵组队）。
--
-- 与种子代码同步：internal/biz/company_lead.go companyLeadSubagentAllowJSON +
-- internal/scenario/system/prompts/company_lead.md（新 GM 自动获得本配置）。
BEGIN;

-- 1) 3 个 GM 岗授 subagents 四件（幂等：仅在 allow 未含 subagents_spawn 时写）
UPDATE agent_runtime_settings s
SET tools_allow_json = '["subagents_spawn","subagents_list","subagents_get","subagents_cancel"]',
    updated_at = NOW()
FROM agents a
WHERE a.id = s.agent_id
  AND a.agent_key LIKE '__company_lead%__'
  AND s.tools_allow_json NOT LIKE '%subagents_spawn%';

-- 2) 存量 GM system.md 插入「能力边界」段（幂等：已含该段的行跳过）。
--    用单行锚点 + chr(10) 拼插，规避 Windows 管道/CRLF 检出对多行字面量的破坏
--    （2026-08-28 实测：Get-Content 管道会把 LF 变 CRLF，多行 replace 静默不命中）。
UPDATE agent_prompt_files f
SET body = replace(f.body,
    '## 禁止',
    '## 能力边界' || chr(10) || chr(10) ||
    '- 你有 subagents_spawn 分身工具：重型或可并行的起草/调研任务，可派自己的分身后台执行，subagents_get 取回结果' || chr(10) ||
    '- 分身是你自己的克隆，**不是部门主管**——禁止虚构「已派发到部门 / 已交办员工」' || chr(10) ||
    '- 需要部门真正执行时：产出公司级 Brief，并明确告知用户路由建议（切换到对应部门主管会话，或请精灵组队接手）' || chr(10) || chr(10) ||
    '## 禁止'),
    updated_at = NOW()
FROM agents a
WHERE a.id = f.agent_id
  AND a.agent_key LIKE '__company_lead%__'
  AND f.file_name = 'system.md'
  AND f.body LIKE '%不继承精灵工具箱%'
  AND f.body NOT LIKE '%能力边界%';

-- 3) 「精灵工具箱」→「精灵编排工具」措辞收紧（单行锚点，幂等）
UPDATE agent_prompt_files f
SET body = replace(f.body,
    '- 不继承精灵工具箱，不读员工过程全文',
    '- 不继承精灵编排工具（plan_and_execute 等），不读员工过程全文'),
    updated_at = NOW()
FROM agents a
WHERE a.id = f.agent_id
  AND a.agent_key LIKE '__company_lead%__'
  AND f.file_name = 'system.md'
  AND f.body LIKE '%不继承精灵工具箱%';

COMMIT;

-- 验证：3 岗 allow 已授 + prompt 已含能力边界段
SELECT a.agent_key, s.tools_allow_json,
       position('能力边界' in f.body) > 0 AS prompt_updated
FROM agents a
JOIN agent_runtime_settings s ON s.agent_id = a.id
JOIN agent_prompt_files f ON f.agent_id = a.id AND f.file_name = 'system.md'
WHERE a.agent_key LIKE '__company_lead%__'
ORDER BY a.agent_key;
