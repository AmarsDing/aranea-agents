# 10 监控场景闭环 测试结果

**结论：监控闭环核心链路 6/6 PASS（HITL 注入→执行→HITL 清除→执行）；发现 3 个问题（2 产品隐患 + 1 平台限制）**

## 最终有效用例（run5/run6 + 直调验证）

| ID | 用例 | 结果 | 说明 |
|----|------|------|------|
| MON-27/28 | 清理过期持久化授权 | PASS | 删除 8-15 遗留 grant_p3_gns3_fault_clear_exec（三层校验：删前 count=1，删后 left=0） |
| MON-29 | 创建 session D | PASS | sid=d4006c59-253a-4d13-9e3c-0a1b86aac04f |
| MON-30 | 异步提交故障注入（明确 prompt） | PASS | 200 |
| MON-31 | HITL confirm step 出现（inject） | PASS | step=8128c4cf...-s4，kind=confirm/status=tool_blocked |
| MON-32 | HITL 批准注入 | PASS | POST /v1/chat/activities/{id}/confirm 200 |
| MON-33 | gns3_fault_inject 真实执行 | PASS | tool_invocations=success |
| MON-36 | 异步提交故障清除 | PASS | 200 |
| MON-37 | HITL confirm step 出现（clear） | PASS | step=8f9bd2fe...-s4 |
| MON-38 | HITL 批准清除 | PASS | 200 |
| MON-39 | gns3_fault_clear 真实执行 | PASS | tool_invocations=success |
| MON-41 | 告警事件直查（Twin Gateway） | PASS(查询)/FAIL(覆盖) | 200 total=74，注入窗口内无新告警 → 监控覆盖缺口 |
| MON-42 | 地面真值：端口状态变更 | PASS | 内核日志 eth1 entered disabled → NIC Link is Up 1000Mbps |
| MON-34/40 | /v1/tools/test 直调 gns3_health_check | FAIL | 400 "not supported for online test yet"（平台限制） |
| MON-35 | 注入后 twin 告警产生 | FAIL | 无告警（同 MON-41 结论） |

## 早期迭代（run1~run4）失败项及根治

| 现象 | 根因 | 处置 |
|------|------|------|
| MON-03/13 twin 工具未执行 | agent 工具策略未授予 twin/gns3 工具 | run4(08模块) PUT /v1/agents/{id}/tools/policy 授予后 PASS（MON-12/13 重测通过） |
| MON-06/15/20 confirm step 不出现 | ① 意图识别对注入请求触发澄清，推荐答案为 "Cancel the injection"，无高风险标记 → autoResolveClarification「假设式前进」自动按推荐作答 → 模型认为用户取消 ② prompt 未给节点名 | run5 改明确 prompt（node sw1 + port eth1 + 禁止澄清）后 confirm step 正常出现 |
| MON-24 clear 无 confirm 直接执行 | 8-15 遗留持久化授权 tool_grants(agent=90fb01da..., tool=gns3_fault_clear) 按决策链 grant_persisted 放行 | run5 MON-27/28 删除后 confirm step 正常出现 |

## 原因分析（3 个问题）

### BUG-MON-A（产品隐患·中危）：destructive 操作的澄清被「假设式前进」自动按推荐取消
- 链式证据：`chat_clarify_gate.go:118` —— 无高风险标记且全部问题带推荐 → autoResolveClarification 自动作答不挂起；而意图识别对「注入故障」未打 destructive 标记，且 LLM 给出的推荐答案是 "Cancel the injection"。结果：用户的注入请求被系统自动代答为「取消」，模型顺从取消。**破坏性操作的澄清不应自动代答**。
- 建议：意图识别对 fault_inject 类请求强制打 destructive 高风险标记（走挂起弹卡）；或 autoResolve 增加工具风险维度——目标工具 requires_confirmation=true 时不允许假设式前进。

### BUG-MON-B（运维隐患·低危）：持久化授权长期残留，HITL 静默失效
- tool_grants 中 8-15 的 fault_clear 授权残留至 8-17，期间所有 fault_clear 调用绕过人工确认直接执行（决策链 `tool_confirm_gate.go:110` grant_persisted 放行，属设计行为）。演练授权应有 TTL 或演练后清理 SOP。
- 建议：tool_grants 增加 expires_at；或测试 SOP 加入授权清理步骤（本测试已在 run5 内置清理）。

### BUG-MON-C（监控覆盖缺口·中危）：端口级故障不产生告警
- 注入 sw1 eth1 down 后 180s 内 TwinMonitor 无任何新告警（直查 /api/v1/monitor/alarm/events total=74 全为历史）。现有 linemonitor 仅探测设备级健康（gns3_agent /health/<device> = ping 192.168.10.2 管理面），端口 down 不影响该探测。
- 建议：TwinMonitor 补充端口级监控规则（SNMP ifOperStatus 轮询或 linemonitor 增加端口探测项），否则「故障注入→告警→处置」闭环在真实监控层面断链。

### 平台限制（非缺陷）：/v1/tools/{id}/test 不支持 gns3/twin 自定义工具
- testexec/execute.go:75 白名单仅含 file/shell/web/openapi 类；twin_alarm_query、gns3_health_check 返回 400。属预期行为但文档未注明，建议在工具管理 API 返回中标注 testable=false。

## 解决方案
- A/B/C 三项已记入总优化清单，待用户汇总裁定；本次测试通过明确 prompt + 授权清理绕过 A/B，C 项为 TwinMonitor 侧规则补充。
- 测试脚本固化：run5.ps1（完整闭环，含授权清理与明确 prompt）、run6.ps1（断点续跑），证据在 evidence/。

## 日志清理
- 本模块测试结束后已检查 aranea-admin 日志无 panic（见 00 模块 ENV-09 同口径）。
