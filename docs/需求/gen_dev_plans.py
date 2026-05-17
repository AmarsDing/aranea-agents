# -*- coding: utf-8 -*-
"""Generate *-development.md under docs/需求/. Run: python docs/需求/gen_dev_plans.py"""
from pathlib import Path

ROOT = Path(__file__).resolve().parent

PLANS = [
    ("0-system-development.md", "系统架构", "✅ 基线稳定", "EP-DOC-01",
     ["维护系统框图与依赖矩阵", "补齐 channel-requirements-analysis 或移除 README 死链", "季度对照 execution-plan 附录 A"]),
    ("1-chat-development.md", "Chat 对话", "✅ 端到端", "—",
     ["HITL/await_user_reply 体验", "与 51 消息机制 WS 主通道对齐", "多模态附件（若需要）"]),
    ("2-agents-create-development.md", "Agent 创建", "✅", "—",
     ["创建向导校验", "默认 runtime_settings", "Tool/Skill 默认绑定"]),
    ("3-agent-list-development.md", "Agent 列表", "✅", "—",
     ["批量操作", "虚拟滚动性能", "workspace 筛选 M2"]),
    ("4-agent-type-development.md", "Agent 分类", "✅", "—",
     ["分类树排序", "列表筛选联动", "i18n"]),
    ("5-agent-setting-development.md", "Agent 设置", "✅", "EP-BIZ-06",
     ["RuntimeSettings 表单", "Tool 有效集预览", "Planner/Model 校验"]),
    ("6-agent-setting-file-development.md", "Agent 提示文件", "✅", "—",
     ["版本 diff", "大文件上传", "prompt 热更新"]),
    ("7-agent-evolution-development.md", "Agent 进化", "🟡 Scanner 无", "EP-BIZ-07",
     ["产品降级文案", "EvolutionScanner worker", "建议应用闭环"]),
    ("8-agent-title-development.md", "Agent 标题", "✅", "—",
     ["标题策略配置", "多语言", "与 Session 统一"]),
    ("9-provider-development.md", "Provider", "✅", "—",
     ["Failover 可视化", "密钥轮换", "趋势图性能"]),
    ("10-session-development.md", "Session", "✅", "—",
     ["Turns 回归矩阵", "压缩可观测", "workspace 隔离 M2"]),
    ("11-multi-agent-development.md", "Team", "✅", "—",
     ["五种模式 UI 标签", "transfer 调试", "成本归因"]),
    ("memory-development.md", "Memory L0-L4", "🟡 L4 未实现", "EP-BIZ-07",
     ["L4 与 Evolution 决策", "Memory tools 覆盖", "pgvector 配置"]),
    ("17-channel-development.md", "Channel", "🟡 飞书✅", "EP-BIZ-05",
     ["多渠道路由器", "未实现渠道禁用", "Webhook 安全"]),
    ("18-monitor-development.md", "Monitor", "✅", "EP-OBS-06",
     ["Grafana 对齐", "Trace 关联", "日志分页"]),
    ("19-mcp-development.md", "MCP", "🟡", "—",
     ["Broker 传输", "健康检查", "工具前缀冲突"]),
    ("20-skill-development.md", "Skill", "✅", "—",
     ["FS+DB 同步", "导入校验", "CodeExecutor 文档"]),
    ("21-cron-development.md", "Cron", "✅", "EP-RT-07 ✅",
     ["可观测面板", "DLQ 重放 UI", "时区测试"]),
    ("22-plugin-development.md", "Plugin", "✅ 注入", "EP-CB-01",
     ["Pre/Post 配对", "运行日志", "Agent 绑定"]),
    ("23-tools-development.md", "Tools", "✅ Override缺", "EP-BIZ-06",
     ["tool_override CRUD", "DeclarableTool UI", "P95 脱敏"]),
    ("24-telemetry-development.md", "Telemetry", "📄 占位", "EP-OBS-02 ✅",
     ["补 telemetry.md", "采样策略", "与 monitor 边界"]),
    ("25-cli-development.md", "CLI", "❌", "M5",
     ["cmd/aranea", "session ls", "配置共享"]),
    ("26-a2a-development.md", "A2A", "🟡", "EP-A2A-01/02",
     ["真派发", "远端鉴权", "EnsureA2ASchema"]),
    ("27-artifact-development.md", "Artifact", "🟡", "EP-RT-08",
     ["Runner 写回", "S3 Repo", "前端浏览器"]),
    ("28-callback-development.md", "Callback", "🟡", "EP-CB-01",
     ["Chain 挂 LLM", "用户 callback", "与 Plugin 合并"]),
    ("29-token-development.md", "Token", "✅", "—",
     ["成本模型", "分摊报表", "CSV 导出"]),
    ("30-ecosystem-development.md", "Ecosystem", "📄", "M5",
     ["市场协议", "Skill 签名", "开放 API"]),
    ("32-codeexecutor-development.md", "CodeExecutor", "✅ selector", "EP-BIZ-02 ✅",
     ["compose 文档", "资源限额", "审计"]),
    ("33-evaluation-development.md", "Evaluation", "🟡", "EP-DATA-01",
     ["EnsureEvalSchema", "前端 Run", "LLM-Judge"]),
    ("34-event-development.md", "Event", "✅", "EP-RT-06 ✅",
     ["可靠/lossy 文档", "背压告警", "跨进程总线"]),
    ("35-gateway-development.md", "Gateway", "🟡", "M3",
     ["并发闸门", "QueuedMessage", "Gateway facade"]),
    ("36-graph-development.md", "Graph", "✅", "—",
     ["模板库", "GC 配置", "HITL UX"]),
    ("37-knowledge-development.md", "Knowledge", "🟡", "EP-KN-01/02",
     ["EnsureKnowledgeSchema", "Embedder", "异步摄取"]),
    ("39-planner-development.md", "Planner", "✅", "—",
     ["a2ui 调试", "Graph 选型", "步骤可视化"]),
    ("40-runner-development.md", "Runner", "✅", "—",
     ["deps 文档", "取消传播", "Team 指标"]),
    ("50-avatar-development.md", "Avatar", "✅", "—",
     ["对象存储", "裁剪", "默认集"]),
    ("message-development.md", "消息机制 51", "🟡", "EP-FE-03",
     ["传输抽象", "event 全覆盖", "SSE 退役"]),
    ("tts-development.md", "TTS", "📄", "M5",
     ["补需求", "选型", "流式播放"]),
    ("admin-auth-development.md", "Admin/Auth", "✅", "EP-WS-01",
     ["JWT 轮换", "workspace 成员", "RBAC"]),
]

TEMPLATE = """# {title} — 开发计划

> **版本**：2026-05-17 | **状态**：{status}  
> **进度真相**：[`guides/execution-plan.md`](../guides/execution-plan.md) 附录 A · **关联 EP**：{ep}

---

## 1. 模块定位

见 [`0 系统框图.md`](./0%20系统框图.md) 与 `docs/README.md` §7 对应需求/设计文档。

---

## 2. 现状评估

| 维度 | 状态 |
|------|------|
| 整体 | {status} |

对照需求文档「2026-05-17 现状对齐」段与附录 A 各列（Proto/Biz/Data/Service/Server/Runtime/前端）。

---

## 3. 差距与优化

- 未完成验收项纳入 Phase。
- 遵守双框架边界：Kratos 传输 / trpc 运行时（AI-DEV-SPEC §1）。
- 复用既有 Usecase，避免平行实现。

---

## 4. 开发阶段

{phases}

---

## 5. 任务清单（可拆 PR）

| 序号 | 任务 | 优先级 | EP |
|------|------|--------|-----|
| 1 | Phase 1 首项 | P1 | {ep} |
| 2 | 单测 + make lint + 更新现状对齐 | P1 | §7 |
| 3 | 前端闭环（若适用） | P2 | EP-FE-* |

---

## 6. 验收标准

- [ ] `go build ./cmd/admin`
- [ ] 相关 `go test`；并发改动用 `-race`
- [ ] 附录 A + 需求「现状对齐」一致
- [ ] changelog 引用 EP

---

## 7. 依赖与风险

M2 多租户可能触及本模块写路径；按 admin→agent→session 分批合入。
"""


def main() -> None:
    for fn, title, status, ep, phase_list in PLANS:
        phases = "\n".join(f"- **Phase {i + 1}**：{p}" for i, p in enumerate(phase_list))
        body = TEMPLATE.format(title=title, status=status, ep=ep, phases=phases)
        (ROOT / fn).write_text(body, encoding="utf-8")
    print(f"Wrote {len(PLANS)} plans to {ROOT}")


if __name__ == "__main__":
    main()
