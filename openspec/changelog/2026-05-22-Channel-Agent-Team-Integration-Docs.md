# Channel × Agent × Team 业务集成文档

**日期**：2026-05-22  
**范围**：文档 only（无代码变更）

## 摘要

新增跨模块业务与设计文档，明确飞书 Channel、Agent、Team、Session、聊天消息五者的职责边界与端到端流转，并同步 `docs/README.md`、系统框图、Channel/Team 需求交叉引用。

## 新增

- `docs/需求/17-channel-agent-team-integration.md` — 业务角色、飞书主链、路由规则、用户故事
- `docs/需求/17-channel-agent-team-integration.design.md` — 数据流、表绑定、Team/Envelope 行为、差距与配置模板

## 更新

- `docs/README.md` §5.2 索引
- `docs/需求/0 系统框图.md` §5.4
- `docs/需求/17 channel.md` / `17 channel.design.md` / `17-channel-development.md`
- `docs/需求/11 multi-agent.md`（Channel 入口一句）
- `docs/需求/README-development.md`
