# 域 B：记忆召回与检索——指令驱动评测（agent-eval-20260818）

> 对应方案 [00-master-plan.md](../00-master-plan.md) §4 域 B。数据源：[sample-memory-qa.json](./sample-memory-qa.json)（50 条，5 类 x 10）。
> 判定等级：PASS / FAIL / REVIEW（自动判定存疑，人工复核证据）。

## 评测流程

1. **B-00 探针准备**：创建专用 agent `eval_memory_probe`（deepseek-v4-flash），测后连同其记忆按 agent_id 清理。
2. **B07-pre prompt 基线**：`GET /v1/agents/{key}/system-prompt/preview` 记录植入前 prompt 体积。
3. **植入阶段**：单会话串行发送全部 plant_messages（跨会话召回口径的前提）。每条记录响应与耗时（A01/A02 写路径佐证）。
4. **异步落库等待**：90s 后查 `/v1/memory/l3/facts?page_size=200`，统计植入关键词命中条数。
5. **B02 召回段延迟**：取 10 条代表性 question，调 `POST /v1/memory/recall/debug`（纯召回路径、不经 LLM）计时——此口径即「召回段 P95」，对标 Mem0 549ms / Zep ~200ms。
6. **提问阶段**：每条 case **新建独立会话**发送 question（跨会话召回），记录回答全文与端到端耗时。
7. **B07-post**：再次 preview 记录植入后 prompt 体积，差值即记忆注入增量。
8. **判定**：
   - `keywords_all`：回答含全部 expected_keywords → PASS，否则 FAIL；
   - `keywords_any`：含任一 → PASS；
   - `abstain`：含拒答词（没有记录/不知道/无法确认/…）且不含植入词表 → PASS；含植入词表 → FAIL；其余 → REVIEW。
9. **汇总**：按 category 统计准确率，产出延迟分布（召回段/端到端），写 result.md。

## 执行

```powershell
# 试跑（前 3 条，验证链路，约 2 分钟）
powershell -ExecutionPolicy Bypass -File run.ps1 -Pilot
# 全量（50 条，约 90 次 LLM 调用 + 90s 落库等待，预计 15-25 分钟）
powershell -ExecutionPolicy Bypass -File run.ps1
# 复跑提问（植入已做过）
powershell -ExecutionPolicy Bypass -File run.ps1 -SkipPlant
```

## 清理

评测完成后：`DELETE /v1/agents/eval_memory_probe`（级联会话）；L3 facts 按 agent_id=eval_memory_probe 过滤删除（写操作三层校验：先 SELECT COUNT 同条件确认 → 事务 → 核验 affected rows）。
