# 域 B 记忆召回评测结果（2026-08-17 13:10）

Pilot=True SkipPlant=False；植入消息数=3；植入会话=ac5f35f9-0312-40d1-9df2-380d2105287b

## 准确率（按类别）
| 类别 | PASS | FAIL | REVIEW | 准确率(不含REVIEW) |
|------|------|------|--------|------|
| single_hop | 2 | 1 | 0 | 67% |

## 召回段延迟（recall/debug，不经 LLM，n=3）
- min=29ms max=30ms P95=30ms
- 目标 <500ms；业界参考 Mem0 549ms / Zep ~200ms

## B07 prompt 体积
- 植入前 81 字符 → 植入后 81 字符