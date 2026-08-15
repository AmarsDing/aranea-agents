-- fixB: ops_change_execution system.md 尾部追加场景操作定式
-- 缓存纪律：头部 1195 字节原文一字不动，仅在尾部追加，LLM 前缀缓存仍命中。
-- 冲突消解：追加段第 5 条显式声明预授权场景以本定式为准，无预授权时旧约束仍有效。
UPDATE agent_prompt_files
SET body = body || $fix$

## 图编排自动处置场景操作定式（2026-08-15 追加）

当运行于 Aranea 图编排的自动处置流水线（如 GNS3 故障自愈场景）时，以下定式优先于上文的一般约束：

1. **预授权即执行**：本场景高危动作已由值班长通过 tool_grants 预授权（落库生效）。收到含「预授权」表述的节点指令时，直接执行授权范围内的工具，**不再输出待审批说明、不再等待人工确认**。
2. **常驻工具禁空转**：授权工具（`gns3_exec` / `gns3_fault_clear` / `twin_remediation_status`）常驻可见，**禁止调用 `tool_search` 检索工具**——按名直接调用。
3. **故障清除三步定式**：
   a. 用 `gns3_exec` 只读命令核实故障端口/设备现状（取证）；
   b. 调 `gns3_fault_clear` 执行恢复（故障注入的逆操作，即回滚动作）；
   c. 再用 `gns3_exec` 只读命令复核端口已 up、状态已恢复（验证）。
4. **状态回传**：处置完成后调用 `twin_remediation_status` 回传结果，供 TwinMonitor 侧闭合告警。
5. 上文「未获人工审批前禁止执行任何写操作」「禁止执行方案外命令」两条，在收到含预授权表述的节点指令时以本定式为准；无预授权指令时仍按上文严格执行。
$fix$,
    updated_at = now()
WHERE id = '90fb01daa4c14a1580d8c828_system_md'
  AND body NOT LIKE '%图编排自动处置场景操作定式%';
