# 域 B 记忆召回评测结果（2026-08-17 16:02）

Pilot=False SkipPlant=False；植入消息数=60；植入会话=f1f65b10-679f-435b-8b21-e1f981982515

## 准确率（按类别）
| 类别 | PASS | FAIL | REVIEW | 准确率(不含REVIEW) |
|------|------|------|--------|------|
| abstention | 0 | 10 | 0 | 0% |
| multi_hop | 6 | 4 | 0 | 60% |
| single_hop | 8 | 2 | 0 | 80% |
| temporal | 3 | 7 | 0 | 30% |
| update | 8 | 2 | 0 | 80% |

## 召回段延迟（recall/debug，不经 LLM，n=10）
- min=23ms max=24ms P95=24ms
- 目标 <500ms；业界参考 Mem0 549ms / Zep ~200ms

## B07 prompt 体积
- 植入前 84 字符 → 植入后 84 字符