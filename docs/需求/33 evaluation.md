# M17: Evaluation 评估 — 详细需求

> 对标 `pkg/trpc-agent-go/evaluation` 包，实现 Agent 自动化评估能力。
>
> **2026-05-17 现状对齐**：
> - ✅ `internal/biz/evaluation.go` / `internal/data/evaluation.go` / `internal/service/evaluation.go` / `internal/evaluation/runner.go` 已落地；`api/kratos/evaluation/v1/*` proto/gRPC/HTTP 已生成。
> - ❌ **`cmd/admin/wire.go` 在装配 EvaluationService 时 Runner 仍传 `nil`**，导致评估 Run 入口不可用；前端 store 未对接。
> - ❌ trajectory 多维度评估、LLM-Judge、报告导出 尚未实现。
>
> 后续以 `guides/execution-plan.md` §3 EP-BIZ-04 + 运维要点 `guides/evaluation.md` 为准。

---

## 1. 现状分析（已过期，保留参考）

项目无 Evaluation 评估能力。Agent 质量仅靠人工判断，无法自动化评估。

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/evaluation/
├── evaluation.go          # AgentEvaluator 接口：Evaluate/Close
├── options.go             # 评估选项
├── pass.go                # 评估通过判定
├── evalset/               # 评估集管理
│   ├── evalset.go         # EvalSet 接口
│   ├── locator.go         # 定位器
│   └── options.go         # 选项
├── metric/                # 指标管理
│   ├── metric.go          # Metric 接口
│   ├── locator.go         # 定位器
│   ├── criterion/         # 评估准则
│   │   ├── llm/           # LLM-as-Judge
│   │   └── ...
│   └── registry/          # 指标注册表
├── service/               # 评估服务
│   ├── service.go         # 评估执行服务
│   ├── local/             # 本地评估服务
│   └── options.go
├── status/                # 评估状态
│   └── status.go
├── evalresult/            # 评估结果
│   └── ...
├── evaluator/             # 评估器
│   └── registry/          # 评估器注册表
└── usersimulation/        # 用户模拟
    └── ...
```

### AgentEvaluator 接口

```go
type AgentEvaluator interface {
    Evaluate(ctx context.Context, evalSetID string, opt ...Option) (*EvaluationResult, error)
    Close() error
}

func New(appName string, runner runner.Runner, opt ...Option) (AgentEvaluator, error)
```

### 核心概念

1. **EvalSet**：评估用例集合，每个用例包含输入和期望输出
2. **Metric**：评估指标，如准确率、相关性、一致性
3. **LLM-as-Judge**：使用 LLM 评估 Agent 输出质量
4. **UserSimulation**：模拟用户与 Agent 交互
5. **MultiRun**：多次运行取平均，减少随机性
6. **EvalResult**：评估结果，包含每个用例的分数和详情

---

## 3. 需求清单

### 3.1 AgentEvaluator 集成

**需求**：支持对 Agent 进行自动化评估

**实现要点**：
- 新建 `internal/evaluation/trpc/evaluator.go`
- 包装 trpc `evaluation.New` 为项目可用组件
- 传入 Runner 和配置选项

**验收标准**：可对指定 Agent 运行评估

### 3.2 EvalSet 管理

**需求**：管理评估用例集

**实现要点**：
- 新建 `internal/evaluation/evalset/`
- 支持从文件/数据库加载评估用例
- 每个用例包含：输入、期望输出、评估准则

**验收标准**：可创建/编辑/删除评估用例集

### 3.3 LLM-as-Judge

**需求**：使用 LLM 评估 Agent 输出质量

**实现要点**：
- 集成 trpc `evaluation/metric/criterion/llm` 包
- 配置 Judge 模型
- 支持自定义评估 prompt

**验收标准**：LLM 可自动评估 Agent 输出质量

### 3.4 UserSimulation

**需求**：模拟用户与 Agent 交互

**实现要点**：
- 集成 trpc `evaluation/usersimulation` 包
- 配置模拟用户行为
- 支持多轮对话模拟

**验收标准**：模拟用户可自动与 Agent 交互

### 3.5 MultiRun

**需求**：多次运行取平均

**实现要点**：
- 配置 `numRuns` 参数
- 并行运行多次评估
- 聚合结果取平均

**验收标准**：评估结果为多次运行的平均值

### 3.6 可视化评估平台（超越层）

**需求**：前端可视化评估结果

**实现要点**：
- 评估结果仪表盘：分数分布、趋势图
- 用例详情：输入/输出/评分/理由
- A/B 测试：对比不同 Agent 版本
- 回归检测：新版本是否退化

**验收标准**：前端可查看评估结果和趋势

### 3.7 API 端点

**需求**：通过 API 管理评估

**实现要点**：
- `POST /evaluation/runs` — 启动评估
- `GET /evaluation/runs/:id` — 查询评估结果
- `GET /evaluation/evalsets` — 列出评估集
- `POST /evaluation/evalsets` — 创建评估集
- `GET /evaluation/metrics` — 列出指标

**验收标准**：通过 API 可管理评估的完整生命周期

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/evaluation/trpc/evaluator.go` | 新建 | AgentEvaluator 适配器 |
| `internal/evaluation/evalset/manager.go` | 新建 | EvalSet 管理 |
| `internal/evaluation/evalset/store.go` | 新建 | EvalSet 存储 |
| `internal/service/evaluation.go` | 新建 | Evaluation 服务层 |
| `internal/server/register_evaluation.go` | 新建 | Evaluation HTTP 端点 |
| `web/src/features/evaluation/` | 新建 | 前端评估平台 |

---

## 5. 验收标准总览

1. 可对指定 Agent 运行自动化评估
2. 可管理评估用例集
3. LLM 可自动评估 Agent 输出质量
4. 模拟用户可自动与 Agent 交互
5. 评估结果为多次运行的平均值
6. 前端可查看评估结果和趋势（超越层）
