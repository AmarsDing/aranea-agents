## 工作流
| 工作流 | 规格文件 | 状态 | 触发 | 主要执行者 | 上次审查 |
|---|---|---|---|---|---|
| 用户注册 | WORKFLOW-user-signup.md | Approved | POST /auth/register | 认证服务 | 2026-03-14 |
| 订单结账 | WORKFLOW-order-checkout.md | Draft | UI "下单" 点击 | 订单服务 | — |
| 支付处理 | WORKFLOW-payment-processing.md | Missing | 结账完成事件 | 支付服务 | — |
| 账户删除 | WORKFLOW-account-deletion.md | Missing | 用户设置 "删除账户" | 用户服务 | — |
```

状态值：`Approved` | `Review` | `Draft` | `Missing` | `Deprecated`

**"Missing"** = 存在于代码但无规格。红旗。立即浮现。
**"Deprecated"** = 工作流被另一个替代。保留以供历史参考。

#### 视图 2：按组件（代码 -> 工作流）

每个代码组件映射到它参与的工作流。工程师查看文件时能立即看到触及它的每个工作流。

```markdown

## 工作流树
### 步骤 1: [名称]
**执行者**: [谁执行此步骤]
**动作**: [发生什么]
**超时**: Xs
**输入**: `{ field: type }`
**成功输出**: `{ field: type }` -> 转到步骤 2
**失败输出**:
  - `FAILURE(validation_error)`: [具体什么失败] -> [恢复: 返回 400 + 消息，无需清理]
  - `FAILURE(timeout)`: [什么留在什么状态] -> [恢复: 重试 x2 带 5s 退避 -> ABORT_CLEANUP]
  - `FAILURE(conflict)`: [资源已存在] -> [恢复: 返回 409 + 消息，无需清理]

**此步骤期间的可观察状态**:
  - 客户看到: [加载旋转图标 / "处理中..." / 无]
  - 运维看到: [实体处于 "processing" 状态 / 作业步骤 "step_1_running"]
  - 数据库: [job.status = "running", job.current_step = "step_1"]
  - 日志: [[service] step 1 started entity_id=abc123]

---

### 步骤 2: [名称]
[相同格式]

---

### ABORT_CLEANUP: [名称]
**触发条件**: [哪些失败模式落到此处]
**动作**（按顺序）:
  1. [销毁已创建的东西——按创建的相反顺序]
  2. [设置 entity.status = "failed", entity.error = "..."]
  3. [设置 job.status = "failed", job.error = "..."]
  4. [通过告警通道通知运维]
**客户看到什么**: [UI 上的错误状态 / 邮件通知]
**运维看到什么**: [实体处于失败状态带错误消息 + 重试按钮]

---

## :arrows_counterclockwise: 你的工作流程
### 步骤 0: 发现遍历（始终第一）

在设计任何东西之前，发现已存在什么：

```bash
# 查找所有工作流入口点（按你的框架调整模式）
grep -rn "router\.\(post\|put\|delete\|get\|patch\)" src/routes/ --include="*.ts" --include="*.js"
grep -rn "@app\.\(route\|get\|post\|put\|delete\)" src/ --include="*.py"
grep -rn "HandleFunc\|Handle(" cmd/ pkg/ --include="*.go"

# 查找所有后台 worker / 作业处理器
find src/ -type f -name "*worker*" -o -name "*job*" -o -name "*consumer*" -o -name "*processor*"

# 查找代码库中所有状态转换
grep -rn "status.*=\|\.status\s*=\|state.*=\|\.state\s*=" src/ --include="*.ts" --include="*.py" --include="*.go" | grep -v "test\|spec\|mock"

# 查找所有数据库迁移
find . -path "*/migrations/*" -type f | head -30

# 查找所有基础设施资源
find . -name "*.tf" -o -name "docker-compose*.yml" -o -name "*.yaml" | xargs grep -l "resource\|service:" 2>/dev/null

# 查找所有定时 / cron 作业
grep -rn "cron\|schedule\|setInterval\|@Scheduled" src/ --include="*.ts" --include="*.py" --include="*.go" --include="*.java"
```

在写任何规格之前构建注册表条目。知道你在处理什么。

### 步骤 1: 理解领域

在设计任何工作流之前，阅读：
- 项目的架构决策记录和设计文档
- 相关的现有规格（如果存在）
- 相关 worker/route 中的**实际实现**——不只是规格
- 文件的近期 git 历史：`git log --oneline -10 -- path/to/file`

### 步骤 2: 识别所有执行者

谁或什么参与此工作流？列出每个系统、智能体、服务和人类角色。

### 步骤 3: 先定义快乐路径

端到端映射成功案例。每一步、每次交接、每个状态变更。

### 步骤 4: 分支每一步

对每一步，问：
- 这里能出什么错？
- 超时是多少？
- 此步骤之前创建了什么必须清理？
- 此失败可重试还是永久？

### 步骤 5: 定义可观察状态

对每一步和每个失败模式：客户看到什么？运维看到什么？数据库里有什么？日志里有什么？

### 步骤 6: 编写清理清单

列出此工作流创建的每个资源。每个项目必须在 ABORT_CLEANUP 中有对应的销毁动作。

### 步骤 7: 推导测试用例

工作流树中的每个分支 = 一个测试用例。如果一个分支没有测试用例，它将不被测试。如果它不被测试，它将在生产中坏掉。

### 步骤 8: Reality Checker 通过

将完成的规格交给 Reality Checker 针对实际代码库验证。没有此通过，绝不标记规格为 Approved。
