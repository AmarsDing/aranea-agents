## 📋 你的技术交付物
### 分支与提交决策矩阵
| 变更类型 | 分支模式 | 提交模式 | 使用时机 |
|-------------|----------------|----------------|-------------|
| 功能 | `feature/JIRA-214-add-sso-login` | `✨ JIRA-214: add SSO login flow` | 新产品或平台能力 |
| 缺陷修复 | `bugfix/JIRA-315-fix-token-refresh` | `🐛 JIRA-315: fix token refresh race` | 非生产关键缺陷工作 |
| 热修复 | `hotfix/JIRA-411-patch-auth-bypass` | `🐛 JIRA-411: patch auth bypass check` | 从 `main` 拉的生产关键修复 |
| 重构 | `feature/JIRA-522-refactor-audit-service` | `♻️ JIRA-522: refactor audit service boundaries` | 与已跟踪任务关联的结构性清理 |
| 文档 | `feature/JIRA-623-document-api-errors` | `📚 JIRA-623: document API error catalog` | 有 Jira 任务的文档工作 |
| 测试 | `bugfix/JIRA-724-cover-session-timeouts` | `🧪 JIRA-724: add session timeout regression tests` | 与已跟踪缺陷或功能关联的纯测试变更 |
| 配置 | `feature/JIRA-811-add-ci-policy-check` | `🔧 JIRA-811: add branch policy validation` | 配置或工作流策略变更 |
| 依赖 | `bugfix/JIRA-902-upgrade-actions` | `📦 JIRA-902: upgrade GitHub Actions versions` | 依赖或平台升级 |

如果更高优先级的工具需要外层前缀，保持仓库分支在其内部完整，例如：`codex/feature/JIRA-214-add-sso-login`。

### 官方 Gitmoji 参考
- 主参考：[gitmoji.dev](https://gitmoji.dev/) 获取当前 emoji 目录和预期含义
- 真相源：[github.com/carloscuesta/gitmoji](https://github.com/carloscuesta/gitmoji) 获取上游项目和使用模型
- 仓库专属默认：新增 brand-new agent 时使用 `✨`，因为 Gitmoji 将其定义为新功能；仅在变更限于现有 agent 或贡献文档的文档更新时使用 `📚`

### 提交与分支校验钩子
```bash
#!/usr/bin/env bash
set -euo pipefail

message_file="${1:?commit message file is required}"
branch="$(git rev-parse --abbrev-ref HEAD)"
subject="$(head -n 1 "$message_file")"

branch_regex='^(feature|bugfix|hotfix)/[A-Z]+-[0-9]+-[a-z0-9-]+$|^release/[0-9]+\.[0-9]+\.[0-9]+$'
commit_regex='^(🚀|✨|🐛|♻️|📚|🧪|💄|🔧|📦) [A-Z]+-[0-9]+: .+$'

if [[ ! "$branch" =~ $branch_regex ]]; then
  echo "Invalid branch name: $branch" >&2
  echo "Use feature/JIRA-ID-description, bugfix/JIRA-ID-description, hotfix/JIRA-ID-description, or release/version." >&2
  exit 1
fi

if [[ "$branch" != release/* && ! "$subject" =~ $commit_regex ]]; then
  echo "Invalid commit subject: $subject" >&2
  echo "Use: <gitmoji> JIRA-ID: short description" >&2
  exit 1
fi
```

### Pull Request 模板
```markdown
