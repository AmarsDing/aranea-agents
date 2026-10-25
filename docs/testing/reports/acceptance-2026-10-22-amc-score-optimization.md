# AMC 文本评分优化本地验收记录

日期：2026-10-22。对应[分析报告](../../reports/2026-10-22-analysis-amc-textual-score-optimization.md)与[实现设计](../../scenarios/agent-memory-challenge/agent-memory-challenge.design.md#10-文本评分增量设计与-adr-amc-evidence-20261022)。本记录不包含比赛凭据，不代表新的平台成绩。

## 验证矩阵

| 检查 | 结果 | 证据 |
|---|---|---|
| HTTP 契约测试 | 21 个顶层测试通过 | `test/agent-memory-challenge/contract-results.jsonl` |
| 记忆领域测试 | 16 个顶层测试通过；已使用正常 `internal/biz` 包路径复核 | `domain-results.jsonl`、`biz-package-check.log` |
| 记忆存储/证据测试 | 39 个顶层测试通过 | `store-results.jsonl` |
| 并发、乱序、重放、回滚 race 检查 | 4 个测试通过；未报告竞态 | `race-results.jsonl` |
| 最后隐私修改定向复验 | 4 个顶层测试通过 | `privacy-results.jsonl` |
| Linux 实际 HTTP Add/Search | 21 条断言通过，其中 8 条为 top_k 上限 | `live-smoke-results.json` |
| Windows pgvector 构建 | 通过 | `bin/memoryeval-amc-score-20261022.exe` |
| Linux/amd64 pgvector 构建 | 通过 | `bin/memoryeval-amc-score-20261022-linux-amd64` |
| 补丁格式、凭据字符串、日志/协程规范定向检查 | 已检查本轮修改；无比赛凭据写入 | 修改文件与 `git diff --check` |

顶层目标回归合计 76 个，其中新增 20 个；子用例、race 重跑和定向重跑不重复计入 76。运行中遇到的其他工作区 Agent 工具装配编译问题，收尾时已用正常包路径复核恢复；本轮未改动其代码。

## 数据库与实际接口条件

- 使用本轮新建的 `pgvector/pgvector:pg16` 隔离容器，不读取原测试容器或开发数据库的口令。
- Go 存储测试只使用 `aranea_test` 下按测试随机创建/清理的 schema。
- Linux 最终实际接口复验连接本任务重新创建的临时实例里的全新 `aranea_eval_smoke` 数据库；服务只发布到本机回环地址，使用独立测试凭据。
- 实际请求使用自行构造的数据：颜色纠正、规则短样例、遥测标识、中文饮料偏好和联系方式。未使用公开/私有评测题及答案。
- HTTP 验证覆盖健康检查、401、Add 同步回显、即时读取、当前值、历史区间、选项辅助检索、规则样例、UTF-8、按需脱敏、跨用户隔离和整请求重放。
- 没有配置真实 embedding 服务；关键词路径为实际运行，向量纠错的正反边界通过 fake vector/embedding 集成用例验证。

## 复现入口

```powershell
# 自动建立并清理独立数据库；使用仓库要求的 F:\gocache。
.\test\agent-memory-challenge\run-score-regressions.ps1

# 如已有专用测试库，通过环境变量提供 DSN，脚本不打印凭据。
$env:ARANEA_TEST_PG_DSN = '<dedicated aranea_test DSN>'
.\test\agent-memory-challenge\run-score-regressions.ps1 -WithRace

# 仅允许回环地址的本地实际接口验证。
.\test\agent-memory-challenge\live-score-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>'
```

测试结果与本地产物不应替代部署版本声明。正式分数、真实模型供应商表现、现有参评库升级、云端稳定性和 Full 负载性能未由本记录验收。

## 第二轮业务逻辑优化验收

针对 62.12 分分项继续审查后，完成分析报告 Q1–Q12。新增 21 个顶层用例：领域 5 个、存储 16 个；目标套件累计 97 个，不将 race、HTTP 断言和重复运行叠加计入。

| 检查 | 结果 | 本地证据 |
|---|---|---|
| HTTP 契约 | 21 个通过 | `quality-contract-results.jsonl` |
| 领域规则 | 21 个通过 | `quality-domain-results.jsonl` |
| 存储与业务证据 | 55 个通过 | `quality-store-results.jsonl` |
| race | 8 个通过，未报告竞态 | `quality-race-results.jsonl` |
| 第二轮 Linux 实际请求 | 25 条断言通过，含 10 次 top_k 检查 | `quality-live-smoke-results.json` |
| 第一轮功能兼容实际请求 | 21 条断言通过，含 8 次 top_k 检查 | `quality-compatibility-smoke-results.json` |
| Windows / Linux pgvector 构建 | 均通过 | `quality-build-hashes.json` |

第二轮实际服务使用最新 Linux 二进制，连接一次性容器里的全新 `aranea_quality_smoke` 数据库。只向回环地址提供服务；全部数据、认证 Secret 和 Token 为专用合成测试值，不使用比赛题或比赛凭据。没有配置真实 embedding。启动中出现无关模块编译问题，经用户授权做一处数据库查询兼容修复后恢复。

第二轮产物：

- `bin/memoryeval-amc-quality-20261022.exe`，SHA256 `37E4326281CC2D6DFB15F19D7B62BA9E2BF4BB947BAA3BA21A607222635BCC49`。
- `bin/memoryeval-amc-quality-20261022-linux-amd64`，SHA256 `94834DDE4B4638E5C4ED96FC15C71B22903ABDF8AE7C8EE59095C37B089CEE6B`。

复现新增实际请求：

```powershell
.\test\agent-memory-challenge\live-quality-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>'
.\test\agent-memory-challenge\live-score-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>' `
  -ResultsFile 'quality-compatibility-smoke-results.json'
```

真实请求不等同于重新评测。未部署公网、未改绑正式版本、未提交平台 Smoke/Full。

收尾确认：构建哈希与记录一致；本轮代码格式、补丁空白与比赛凭据模式定向检查通过；专用运行容器、测试数据库容器和网络均已清理。文档与复现脚本保留在本地归档目录。

## 第三轮 r2 Full 短板优化验收

用户确认基线：`teval_e176051a414de4db` / 文本 / `amc-2026.09-r2` / `r2-full-1` / Full / Add16 / Search32 / top_k100。27 个子项中 17 个低于 50，8 个低于 35；这是历史参评版本，前两轮及本轮本地代码尚未提交该评测。仓库 r2 tag 的提交为 `b0e6063731e1ee510d1fbea8481a79476bbb8e97`。

第三轮按分析报告 R1–R7 完成生命周期、结构、关联、背景、指令范围、实际时间区间与综合类型覆盖。先复现 8 项业务失败和两项本地午夜用例失败，再实现修复；红测证据分别为 `fullfocus-before-results.jsonl`、`fullfocus-time-before-results.jsonl`。

| 检查 | 结果 | `test/agent-memory-challenge/` 下证据 |
|---|---|---|
| HTTP 契约 | 21 项通过，无跳过 | `fullfocus-contract-results.jsonl` |
| 领域规则 | 26 项通过，无跳过 | `fullfocus-domain-results.jsonl` |
| PostgreSQL 业务回归 | 73 项通过，无跳过 | `fullfocus-store-results.jsonl` |
| 并发与 race | 9 项通过，无竞态报告 | `fullfocus-race-results.jsonl` |
| 新 Linux 实际 HTTP | 50 条断言通过 | `fullfocus-live-smoke-results.json` |
| 第二轮功能兼容 HTTP | 25 条断言通过 | `fullfocus-quality-compatibility-results.json` |
| 首轮功能兼容 HTTP | 21 条断言通过 | `fullfocus-contract-compatibility-results.json` |
| 构建 | Windows/Linux amd64 `pgvector` 均成功 | `fullfocus-build-hashes.json`、`fullfocus-build-*.log` |
| 格式、空白与凭据模式 | 定向检查通过，未写入用户比赛凭据 | 本轮命令及源码清单 |
| 资源清理 | 两个专用容器及网络已清理 | `fullfocus-cleanup-results.json` |

顶层目标测试合计 **120**，比前两轮新增 **23**（领域 5，数据/证据 18）；不重复计入子用例、定向重跑和 race。实际 HTTP 合计 **96 条断言**，与 Go 顶层测试分开计数。汇总为 `fullfocus-test-counts.json`、`fullfocus-http-counts.json`。

16 个并发 Add 包含来源时间在中间的遗忘指令，后续 32 个并发 Search 都读取最新有效状态；此用例也运行 race。这个本地一致性用例不构成 Full 规模吞吐、限流或 P95 性能证明。

实际服务使用新 Linux 二进制、`pgvector/pgvector:pg16` 和全新 `aranea_fullfocus_smoke` 数据库，监听 `127.0.0.1:64002`（验证结束后已关闭）。未配置真实 embedding。全部测试用户、事实、认证 Secret 和 Token 为合成数据；未向正式比赛服务提交数据或调用评测任务。运行日志为 `bin/logs/amc-fullfocus-runtime-20261022.log`。

构建元信息：Go 1.26.3，`-tags=pgvector`，Git revision `34e38f9640009bc927f3d5358f24d8121a73a37a`，`vcs.modified=true`；Linux `CGO_ENABLED=0`、`GOOS=linux`、`GOARCH=amd64`。共享工作区不是冻结提交；源码清单保存在 `fullfocus-source-hashes.json`，正式打包应以源码与产物哈希共同核对。

- `bin/memoryeval-amc-fullfocus-20261022.exe`，SHA256 `E3286BD44399F2FD76565A7FEE2FCC59B786C936FB37B9A50F8978BE40CEE594`。
- `bin/memoryeval-amc-fullfocus-20261022-linux-amd64`，SHA256 `89DE1EB88D5916712BDF40E86F94126D27C550D7CE0F80E301E4DB62A4669BE7`。

复现命令（仅使用新的专用本地服务和测试凭据）：

```powershell
.\test\agent-memory-challenge\run-score-regressions.ps1
.\test\agent-memory-challenge\live-fullfocus-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>'
.\test\agent-memory-challenge\live-quality-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>' `
  -ResultsFile 'fullfocus-quality-compatibility-results.json'
.\test\agent-memory-challenge\live-score-smoke.ps1 `
  -BaseUrl 'http://127.0.0.1:<local-port>' -MemoryToken '<local test token>' `
  -ResultsFile 'fullfocus-contract-compatibility-results.json'
```

本轮构建曾因共享工作区新文件 `board_run_lease_repo.go` 的 `QueryRowContext` 调用失败，随后该文件自身更新为兼容调用，正常构建和回归恢复；本轮未编辑该模块。初始错误保存在 `fullfocus-unrelated-build-blocker.log`。先前审批策略的一处最小编译修复已获用户授权，仍按第二轮证据记录。

本地能力边界：逻辑删除不等同于物理擦除；指令控制仅限独立的明确用户消息；主体、实体、背景及范围是启发式；关联最多两跳，预算容不下完整父链时省略分支。大型结构可能摘录，未知日期不推造，旧正式库存没有回填。最终总结、归纳、流程执行和格式回答仍由平台 Answer 完成。真实 embedding、线上语义召回、Full 容量、P95、新正式得分均未验证；未部署公网、未改绑版本、未提交新的 Smoke/Full。

## 2026-10-23 主分支提交复核

仓库默认主分支为 `dev`（`origin/HEAD → origin/dev`）。从提交 `6b9258fea6f51a77cd0ebfac8a69398be8180cbd` 创建独立工作树，只复制本次交付文件，避免验证依赖共享工作区中其他任务的未提交改动。37 个记忆源码文件与第三轮清单哈希一致。

该工作树重新通过 120 项目标回归（接口 21、领域 26、存储 73）、9 项 race、96 条实际 HTTP 断言（50 + 25 + 21），均无失败或跳过；Windows/Linux amd64 的 `pgvector` 构建均成功。复核汇总与新产物哈希保存在 `test/agent-memory-challenge/commitcheck-results.json`。Go 为 1.26.3，本次使用独立的原生 PostgreSQL 18.3 临时实例；实际 HTTP 使用新 Windows 程序、全新数据库、回环地址和一次性合成鉴权配置。

临时 HTTP 服务和数据库进程均已停止；数据库数据目录作为忽略的本地测试产物保留，未纳入提交。此前第三轮 Linux/PG16 的实际接口证据继续保留。两次验收都未配置真实 embedding，也未触发新的正式 Smoke/Full 评测。
