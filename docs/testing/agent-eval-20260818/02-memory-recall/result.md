# 域 B 记忆召回评测结果（2026-08-17 22:57）

Pilot= SkipPlant=；植入消息数=60；植入会话=a35a748b-ce1e-44ee-bb8a-3cc130732396

## 准确率（按类别）
| 类别 | PASS | FAIL | REVIEW | 准确率(不含REVIEW) |
|------|------|------|--------|------|
| abstention | 0 | 10 | 0 | 0% |
| multi_hop | 3 | 7 | 0 | 30% |
| single_hop | 8 | 2 | 0 | 80% |
| temporal | 0 | 10 | 0 | 0% |
| update | 0 | 10 | 0 | 0% |

## 召回段延迟（recall/debug，不经 LLM，n=10）
- min=26ms max=33ms P95=33ms
- 目标 <500ms；业界参考 Mem0 549ms / Zep ~200ms

## B07 prompt 体积
- 植入前 6372 字符 → 植入后 6372 字符