# cmd/admin 不变量档案（INVARIANTS）

> orca `daemon/AGENTS.md` 式档案：每条 = 不变量 / 为什么（事故史）/ 违反代价。
> 这是**摘要与指针**；红线与组织铁律的全文本仍在 `AGENTS.md` 与 docs/（gitignored）——全量搬迁为独立决策，见 docs/reports/2026-10-22-research-orca-orchestrator-analysis.md §5 H9。

## INV-ADMIN-1 wire_gen.go 自包含，禁止 plain regen

- **不变量**：`wire.go` 引用的 provider 副本必须**手工镜像**进 `wire_gen.go`；`wire` 工具直接重新生成的产物**不可编译**，禁止提交。
- **为什么**：本仓 wire_gen 为自包含快照（历史决策），与 `wire` 工具的 regen 输出结构性不兼容；多会话并行期已发生多起「regen 后编译红」事故。
- **违反代价**：dev 提交态编译破裂，全仓阻塞。
- **改 wire 前的核对**：确认 wire_gen 三注入存活（比看提交信息可靠）。

## INV-ADMIN-2 ent codegen 单写者 + 环境门槛

- **不变量**：ent codegen 必须 `DEPLOY_ENV=dev` + `KRATOS_HTTP_AUTH_DISABLED=1`；共享树下**一次只允许一个会话**执行 codegen；多会话环境用带外 staging（thin biz shim）+ 逐文件回写。
- **为什么**：2026-10-21 ent 双腐坏事故——两个会话并发 codegen + stash pop 复活腐坏备份。
- **违反代价**：ent 生成物腐坏扩散，回滚成本极高。

## INV-ADMIN-3 编译产物只进 bin/

- **不变量**：一切 `go build` 产物输出到 `bin/`；运行日志写 `bin/logs/`（`configs/*.yaml` output_dir）。
- **为什么**：根目录规范红线；历史 `_temp/`、`release/` 等散落目录已于 2026-08-09 清理。
- **违反代价**：发布/排查路径混乱，产物误入库。

## INV-ADMIN-4 日志构造注入

- **不变量**：struct 通过 `lg loggateway.Logger` 参数注入；禁止 `loggateway.Global()`（已 deprecated）；测试用 `loggateway.NewNoop()`。
- **为什么**：红线 #16；Global() 使日志归属不可追踪。
- **违反代价**：日志链路断签，审计失真。

## INV-ADMIN-5 提交纪律（多会话共树）

- **不变量**：commit 只 add 显式文件名（自己的文件）；推送前先算「远端变更 ∩ 脏文件」重叠；分叉 merge 不 rebase/force。
- **为什么**：并行会话 WIP 混杂，整树 add 会裹挟他人半成品（2026-10 多次双红教训）。
- **违反代价**：把他人未验证 WIP 推上共享分支。
