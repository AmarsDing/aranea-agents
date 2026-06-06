## MODIFIED Requirements

### Requirement: CI 失败自动检测与日志提取
CI 失败后，系统 SHALL 自动提取失败日志并解析为结构化 FailureReport JSON（而非原始文本），传给 Agent/pattern-fix 进行诊断。FailureReport MUST 包含 type/source/job/file/line/error_code/message/stack_trace/related_code 字段。

#### Scenario: CI 失败触发 Auto-Fix
- **WHEN** CI workflow 以 failure 结论完成
- **THEN** 系统 SHALL 提取失败日志，解析为 FailureReport JSON，按 type 路由到对应修复策略

#### Scenario: Lint 错误规则修复
- **WHEN** FailureReport.type 为 "lint_error"
- **THEN** 系统 SHALL 使用 araneactl lint --fix + eslint --fix + stylelint --fix 进行规则修复

#### Scenario: 测试/构建失败 Agent 诊断
- **WHEN** FailureReport.type 为 "test_failure" 或 "build_failure"
- **THEN** 系统 SHALL 优先使用自托管 Agent（ARANEA_API_URL）诊断，未配置时回退到 pattern-fix.sh

## ADDED Requirements

### Requirement: Critic Agent 语义检查步骤
Auto-Fix 验证通过后，系统 SHALL 新增 Critic Agent 步骤，用 LLM 对比 diff 检查语义偏差。

#### Scenario: Critic Agent 评估修复安全性
- **WHEN** Auto-Fix 生成的 patch 通过 go vet + pnpm build 验证
- **THEN** 系统 SHALL 调用 Critic Agent 评估修复的语义安全性，输出 CriticResult{is_safe, risk_level, concerns, suggestion}

### Requirement: 保护文件白名单细化
系统 SHALL 在保护文件列表中增加白名单机制，允许 auto-fix 修改 internal/biz/monitor/ 目录下的自愈模块代码。

#### Scenario: 自愈模块可被修复
- **WHEN** auto-fix 修复的文件位于 internal/biz/monitor/ 目录
- **THEN** 系统 SHALL 允许修复，不触发保护文件拒绝

#### Scenario: 关键文件仍受保护
- **WHEN** auto-fix 尝试修改 .github/workflows/、Makefile、go.mod/sum、proto 文件
- **THEN** 系统 SHALL 拒绝修复，记录保护文件命中
