# ADR：组织感知编排（ORG-FAST）— 快且准地获取/组建团队

> 日期：2026-08-22
> 类型：ADR（架构决策）
> 状态：已采纳，待实施（M78）
> 关联：[78-org-aware-orchestration.md](../development/78-org-aware-orchestration.md) · [设计](../development/78-org-aware-orchestration.design.md) · [开发计划](../development/78-org-aware-orchestration.development.md) · [M67](../development/67-organization-redesign.md) · [1-chat B.10.21](../development/1-chat.design.md)

---

## 背景

用户给出任务指令后，系统需要**又快又准**地找到承接组织、组建团队。最初设想：

> Agent 按任务类型找最合适的公司 → 找最合适的部门交给部门领导 → 部门领导再分解任务、把最合适的任务交给最合适的 Agent；没有合适的公司则自己创造 Team 或 Agent。

该设想对齐真实公司运作（M67 编制表隐喻），但若按「每一跳都走 LLM」落地，会与现有 `plan_and_execute`（Plan → Allocate → Orchestrate）叠加成**双分解、多跳冷启动**，既慢又易冲突。

## 决策

采用 **ORG-FAST**（Organization-Filtered Allocation with Specialist Teams）：

| 角色 | 热路径职责 | 不负责 |
|------|-----------|--------|
| **精灵 / 编排管家** | 复杂度门控、任务分解、DAG、合成 | 不代替部门主管审批 |
| **组织树** | **确定性剪枝**：把 200 人候选收到 5–15 人 | 不参与 LLM 推理 |
| **Allocator** | 在剪枝后的候选上做 L0 配方 / L1 使命 / L2 履历+语义 / L3 冷启动 | 不把部门主管当业务执行者 |
| **部门主管** | **治理**：借调审批、交付质量门、缺编建议；抽查文件走 memberfs | 不二次分解用户原任务；不把全部附件喂给主管 LLM |
| **Factory** | 无合适 Agent 时创建（用户确认）并挂到已有岗位 | 不创建公司/部门 |
| **交接** | Brief 进下游上下文；Bulk 只传指针并物化 inbox | 不把仓库灌进 prompt；不用监管目录当传递通道 |

**拒绝**把「公司 → 部门 → 主管 LLM 分解 → 派工」作为每条任务的串行调度链。

## 后果

- 快：高置信路径 0 次额外 LLM（配方/使命命中）；L3 只对剪枝集调用；主管 LLM 只在治理条件触发。
- 准：候选被组织子树约束，避免媒体部主管被派成软件任务 Lead（已有实测）。
- 兼容：不替换 B.10.21 匹配管线，只在其前增加 Org-Prune，并在 AssembleTeam 补上 `DepartmentID`。
- 创造策略收紧：任务路径禁止创建公司/部门；只允许确认后创建 Agent 与一次性 Team。
- 交接：下游默认只吃结论信封；大文件声明后落 inbox/制品，与 M71「传递与监管分离」一致。

## 替代方案（否决）

| 方案 | 否决原因 |
|------|----------|
| 每任务串行「找公司→找部门→主管分解」 | 每跳 3–10s LLM；与 Planner 双分解冲突；当前 workspace 单公司，找公司无收益 |
| 纯向量全库匹配、忽略组织 | 快但准度差，跨部门乱配；M67 编制表闲置 |
| 无匹配就创建公司 | 组织膨胀、权限/审批失锚；公司是管理员资产不是任务产物 |
| 跨团队把文件全文注入下游 prompt | 慢且不准；与已有 500 字摘要 + 按需读全文设计冲突 |
| 用 memberfs 给下游员工读上游目录 | 破坏「声明即交付」；监管通道被当成传递通道 |

## 后续

实施任务与代码锚点见 [78-org-aware-orchestration.development.md](../development/78-org-aware-orchestration.development.md)。
