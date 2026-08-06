# Agent Memory Challenge 2026 — 开发计划

> **需求**：[agent-memory-challenge.md](./agent-memory-challenge.md) · **设计**：[agent-memory-challenge.design.md](./agent-memory-challenge.design.md)
> **提交截止**：2026-08-07 23:59（UTC+8）

---

## 1. 模块定位

本场景不是产品新功能，而是**参赛交付**：以最小改动在现有 L0–L4 记忆体系上桥接平台 Add/Search 契约，并完成学术披露与提交。适配层代码随仓库开源，参评版本打固定 tag。

## 2. 代码锚点

| 锚点 | 路径 | 说明 |
|------|------|------|
| 评测适配层（新增） | `api/kratos/memoryeval/v1/eval.proto`、`internal/service/memory_eval.go` | Add/Search 契约桥接 |
| 记忆写入 | `internal/biz/memory.go`（MemoryUsecase）、`internal/biz/memory_consolidator.go` | L3 事实提取/写入 |
| 记忆召回 | `internal/biz/memory_admin_store.go`（RecallL2Episodes/RecallL3Facts）、`internal/data/memory_shim_l2.go`、`memory_shim_l3.go` | 混合评分召回 |
| Embedding | `internal/service/memory_embedder_adapter.go`（MemoryEmbeddingAdapter） | OpenAI 兼容端点 |
| 部署 | `Dockerfile`（根）、`configs/` | 单容器 / compose 形态 |
| 现状梳理 | `docs/development/memory/memory.md`、`memory.design.md`、`memory-development.md` | L0–L4 完成功能真相源 |

## 3. 任务清单

| # | 任务 | 优先级 | 依赖 | 状态 |
|---|------|--------|------|------|
| T0 | 确认仓库 public 可匿名访问（https://github.com/AmarsDing/aranea-agents） | P0 阻塞 | 用户 | ✅ 已确认 public |
| T1 | 实现 Add/Search 适配层：`memoryeval/v1` proto + service + 鉴权中间件 + 窄接口 `EvalMemoryPort`；含 user_id 隔离单测 | P0 | 无 | ⏳ |
| T2 | Docker 双形态验证：①根 Dockerfile 干净构建 + SQLite 单容器跑通 Add/Search；②compose（pgvector）完整模式跑通 | P0 | T1 | ⏳ |
| T3 | 仓库 README 增加「Agent Memory Challenge」章节：构建/启动/健康检查/curl 示例/依赖与降级说明 | P0 | T2 | ⏳ |
| T4 | 仓库内发布学术披露页（引用 + 方法改动，引用 design.md §3/§4 内容） | P0 | 无 | ⏳ |
| T5 | Smoke 自测脚本：模拟平台 Add（多 user 多会话）→ Search 断言召回与隔离 → 输出报告 | P0 | T1 | ⏳ |
| T6 | 打固定版本 tag `amc-2026.08`，全局 grep 确认无 Key 残留 | P0 | T1–T5 | ⏳ |
| T7 | 填写申请表（需求文档 §3 口径，补联系人/机构）并提交 | P0 | T0、T6 | ⏳ |
| T8 | （可选）代码记忆榜复核：确认同一适配层覆盖工程任务记忆场景，必要时补说明 | P1 | T1 | ⏳ |
| T9 | （可选）社区贡献加分：准备 3 组挑战性测试样本提交审核 | P2 | T7 | ⏳ |

## 4. T1 实施要点（适配层）

1. 契约字段以官方 API Guide 最新版为准，先核对再写码（design.md §5.1 有 ⚠️ 标注）
2. Search 路径禁止任何 LLM 调用；返回库存记忆原文
3. user_id 为空一律 400；召回强制 user scope 过滤；单测必须含跨 user_id 污染用例
4. Add 同步确认 + 内部异步摄取；Embedding 不可用降级关键词索引，不阻断契约
5. 日志：适配层关键节点按 K1/K2 规范打进程日志（`loggateway`，构造注入），评测流量做计数限流
6. 遵守红线：新 proto 走 `make api`；wire 注入走 `make wire`；错误经 `apierror` 翻译

## 5. 验证命令

| 改动 | 最小验证 |
|------|----------|
| proto | `make api && go build ./...` |
| service/biz | `go test ./internal/service/... ./internal/biz/... -run MemoryEval -count=1` |
| wire | `make wire && go build ./cmd/admin` |
| Docker | `docker build -t aranea-memory:amc-2026.08 .` + 容器内 smoke 脚本全绿 |
| 提交前 | 后端全量 `make build && make test && make lint` |

## 6. 验收标准

继承需求文档 §4（AC1–AC6），另加：

| # | 标准 |
|---|------|
| AC7 | T1 单测覆盖：正常 Add/Search、user_id 缺失 400、跨 user_id 零泄漏、Search 无 LLM 调用断言 |
| AC8 | Smoke 自测报告（T5）显示召回率与隔离全部通过 |

## 7. 风险与应对

| 风险 | 等级 | 应对 |
|------|------|------|
| 仓库非 public / 开源合规未就绪 | 高 | T0 今天确认；若受阻则改走商业·API 路径（需公网稳定部署，成本更高）或放弃本期 |
| 平台 Docker 环境无法访问外部 Embedding API | 中 | 设计已内建关键词降级；T2 两种形态均验证 |
| SQLite 模式检索质量不足影响成绩 | 中 | compose（pgvector）作为推荐形态写进运行说明 |
| 契约字段与官方最新版偏差 | 中 | T1 动手前先复核官方 API Guide |
| 截止前时间不足 | 高 | 严格按 T0→T7 顺序，P0 之外全部可砍；材料（M1–M4）已完成不阻塞 |

## 8. 改动文件清单（预计）

新增：`api/kratos/memoryeval/v1/eval.proto`、`internal/service/memory_eval.go`、`internal/service/memory_eval_test.go`、`docker-compose.eval.yml`（形态 B）、`test/agent-memory-challenge/smoke.*`
修改：`README.md`（参赛章节）、`configs/`（eval 开关）、`internal/service/service.go`（wire 绑定）
文档：本目录四份（已完成）
