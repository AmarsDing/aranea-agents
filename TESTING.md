# Go 测试合约（TESTING.md）

> 全仓 Go 测试的反模式基线（M9，仿 orca 入库测试合约条款化）。新增/修改测试时逐条自检；code review 按此清单打回。

## 禁止项

1. **禁真 sleep / 真等待**：测试里的 `time.Sleep` 等待事件一律改为注入时钟、chan 通知、或轮询超时断言。真 sleep 在 CI 慢机器上必然 flaky。
2. **禁 mock 数据层本身**：ent client / Repo 接口的持久化语义（唯一约束、事务、触发器、SQL 方言）只有真实 PG 能代表——用真实 PG（默认 DSN 已随代码内置：`postgres://postgres:123456@127.0.0.1:55432/aranea_test`，本机经 pg-proxy 55432 转发；CI/非标环境用 `ARANEA_TEST_PG_DSN` 覆盖，见 `internal/data/testhelper/pg.go`）。biz 层单测可用内存 fake，但 fake 必须实现**接口的完整语义**（含排序/过滤/分页），不是只返回 nil。
3. **禁只断言「mock 被调用」**：断言必须落在可观测结果上（返回值、落库状态、发布的事件、指标计数），「某方法被调了几次」不构成行为验证。
4. **禁 test-only export**：不得为测试开导出孔（`export_test.go` 例外，仅限同包白盒）。新增「仅测试使用」的导出符号视为架构口子，应被 review 打回并随后纳入 ratchet。
5. **禁在测试中重实现被测逻辑**：金样/期望值得自规格或真实观测，不得把被测实现抄一遍再 assert 自相等。
6. **禁共享可变单例未隔离**：包级可变状态（注册表、promauto 指标除外）在并行测试中必须隔离或显式串行（`t.Parallel` 开启前先确认无共享态）。

## 推荐项

- **构造注入**测试 logger：`loggateway.NewNoop()`，禁止 `loggateway.Global()`（红线 #16）。
- **表驱动**用例一行一个分支名（中文分支名优先，失败输出即文档）。
- **负向断言配对**：验证「正确输入通过」的同时验证「畸形输入被拒且报错文案稳定」——报错文案是折叠/匹配语义的载体，改动需同步测试。
- **并发路径**测试用 `-race` 验收：涉及锁/channel 的改动本地先跑 `go test -race` 再提。

## 与本仓其它规范的关系

本合约是 `aranea-review` SKILL 后端合规清单的测试域补充；与 AGENTS.md 红线冲突时以红线为准。
