## 🚨 你必须遵守的关键规则
### Jira 关卡
- 没有 Jira 任务 ID，绝不生成分支名称、提交信息或 Git 工作流建议
- 完全按提供的方式使用 Jira ID；不得发明、规范化或猜测缺失的工单引用
- 如果缺少 Jira 任务，询问：`请提供与此工作相关联的 Jira 任务 ID（例如 JIRA-123）。`
- 如果外部系统添加了包装前缀，保留其中的仓库模式而非替换它

### 分支策略与提交规范
- 工作分支必须遵循仓库意图：`feature/JIRA-ID-description`、`bugfix/JIRA-ID-description` 或 `hotfix/JIRA-ID-description`
- `main` 保持生产就绪；`develop` 是持续开发的集成分支
- `feature/*` 和 `bugfix/*` 从 `develop` 分出；`hotfix/*` 从 `main` 分出
- 发布准备使用 `release/version`；存在发布工单或变更控制项时，发布提交仍应引用它们
- 提交信息保持单行，遵循 `<gitmoji> JIRA-ID: 简短描述` 格式
- 优先从官方目录选择 Gitmoji：[gitmoji.dev](https://gitmoji.dev/) 和源仓库 [carloscuesta/gitmoji](https://github.com/carloscuesta/gitmoji)
- 对于本仓库中的新 Agent，优先使用 `✨` 而非 `📚`，因为该变更新增了目录能力而非仅更新现有文档
- 保持提交原子化、聚焦、易于回滚且无附带损害

### 安全与运营纪律
- 绝不在分支名称、提交信息、PR 标题或 PR 描述中放置密钥、凭证、令牌或客户数据
- 将安全审查视为强制要求，适用于认证、授权、基础设施、密钥和数据处理的变更
- 不得将未经验证的环境呈现为已测试；明确说明验证了什么以及在哪里验证的
- 合并到 `main`、合并到 `release/*`、大型重构和关键基础设施变更时，Pull Request 是强制要求
