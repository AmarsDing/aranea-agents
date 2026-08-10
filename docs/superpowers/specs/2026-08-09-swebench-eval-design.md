# SWE-bench 自动化评测设计（方案 A：宿主机工作区 + 官方镜像评分）

> 状态：已批准（用户 2026-08-09 选择方案 A、Verified 500 题、HTTP API 端到端驱动、本机 Docker Desktop、运行时数据在 D 盘）

## 1. 目标

用 SWE-bench Verified（500 题）官方口径实测 Aranea 的编程任务能力，产出可与公开榜单直接对比的 resolved 率。分数必须代表真实用户路径（HTTP API 端到端，含编排/工具门控全栈）。

## 2. 关键决策

| 决策点 | 结论 |
|--------|------|
| 评测执行环境 | 本机 Docker Desktop（已装至 D:\DockerDesktop，数据根 D:\DockerData） |
| 数据集 | princeton-nlp/SWE-bench_Verified（500 题，可配置抽样 smoke） |
| Agent 驱动 | HTTP API 端到端：POST /v1/sessions → /v1/chat/messages:submit → 轮询 /v1/chat/run-status |
| Patch 提取 | 宿主机 workdir `git diff`（含新文件、剔除测试文件改动），不从 LLM 文本解析 |
| 评分 | 官方 `swebench.harness.run_evaluation`，resolved = F2P + P2P 全过 |
| 运行时数据 | 全部在 D 盘：D:\swebench\{runs,repos,predictions}；repo 缓存镜像克隆 |

## 3. 每题生命周期

1. git 镜像缓存克隆 repo → 检出 base_commit 到 `runs/<run_id>/<instance_id>/workdir`
2. 启动官方实例镜像容器（`sweb.eval.x86_64.<iid>`），workdir bind-mount 到 /testbed（agent 环境 == 评分环境）；agent 用 shell 工具 `docker exec` 在容器内跑测试
3. 建专用会话，提交渲染后的 issue prompt（agent_key=swe-bench，provider/model 走配置）
4. 轮询 run-status：completed/failed/timeout（软 25min/硬 30min）；awaiting_user 超 5min 判失败
5. workdir git diff → patch.diff（空 patch 记 empty_patch，计入分母）
6. 容器销毁，状态落盘可断点续跑

## 4. 状态机与容错

- 状态：pending → workspace_ready → solving → solved | failed | timeout | empty_patch；任意阶段 → infra_error
- infra_error（docker 故障/API 5xx/网络断）自动重试 ≤2 次，**不计入 Agent 能力失败分母**
- `solve --run-id <id>` 续跑非终态题目

## 5. 被测 Agent

- 专用 agent_key `swe-bench`，工具：shell + filesystem；工具确认门需自动通过（headless 前置条件）
- 系统提示词约束：自主工作不提问、最小修复、禁改测试文件
- 一次评测固定一个 provider/model

## 6. 产物与报告

- `predictions/<run_id>.jsonl`（官方格式）→ 官方 harness 评分
- `runs/<run_id>/report.md`：resolved 率、状态分布、失败分类、官方原始结果
- 报告归档 docs/reports/（命名 `YYYY-MM-DD-benchmark-swebench-verified.md`）

## 7. 前置条件

- [x] Docker Desktop 安装（D 盘，winget 4.85.0 已装）
- [ ] 重启系统使 WSL2 组件生效（WSL_E_WSL_OPTIONAL_COMPONENT_REQUIRED 待重启消除）
- [ ] Aranea admin server 以 `KRATOS_HTTP_AUTH_DISABLED=1` 启动（或配置 auth_cookie）
- [ ] 磁盘预留 ~100GB（实例镜像）

## 8. 验证策略

- pytest 覆盖纯函数：patch 测试文件过滤、状态机/断点续跑、prompt 渲染
- 端到端 smoke：Verified 抽 3 题全链路跑通，人工核对 patch 合理性 + 官方评分非 0，再放全量

## 9. 代码位置

`test/swebench/`（编排器 Python 包 `swebench_runner` + configs + tests），符合根目录规范（test/<test-name>/）。
