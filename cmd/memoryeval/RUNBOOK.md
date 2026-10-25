# memoryeval 部署 Runbook（AMC C3 合规版）

> 适用：Agent Memory Leaderboard 学术榜参评部署。契约事实均经
> 2026-10-23 官网核实（留存见 `test/amc-verify/`）。
> 学术榜硬约束：embedding **必须**为 `text-embedding-v4`，生成式 LLM
> **必须**为 `gpt-4o-mini`，reranker 自选；零-LLM 链路可声明 N/A，但
> embedding 必须如实申报。生产侧的 bge-m3 (Ollama 1024 维) **不合规**。

## 1. E1：embedding 合规切换（env-only）

本服务的 embedding 走 OpenAI 兼容网关（`internal/knowledge/embedder.go`
的 openai provider）。设计上**不传 `dimensions` 参数**（G-39 /
ADR-KN-20261021-G），实际输出恒为模型原生维度。`text-embedding-v4`
原生维度 = **2048**，因此部署侧必须让维度链全部对齐 2048，并使用
**全新**的评测命名空间（禁止复用任何含 1024 维向量的库存量）。

```powershell
$env:EVAL_PG_SOURCE      = "postgres://postgres:123456@127.0.0.1:55432/aranea_eval_c3?sslmode=disable"
$env:EVAL_VECTOR_DIM     = "2048"          # 新库按 2048 建向量列
$env:EVAL_MEMORY_TOKEN   = "<Memory System Key>"

$env:EMBEDDING_PROVIDER  = "openai"
$env:EMBEDDING_BASE_URL  = "https://dashscope.aliyuncs.com/compatible-mode"
$env:EMBEDDING_API_KEY   = "<DashScope API Key>"
$env:EMBEDDING_MODEL     = "text-embedding-v4"
$env:EMBEDDING_DIM       = "2048"          # 与模型原生维度一致（无截断参数）

go build -o bin/memoryeval.exe ./cmd/memoryeval
./bin/memoryeval.exe
```

自检（启动后必须看到真实 2048 维写入，而不是配置的口头值）：

```powershell
curl -X POST http://127.0.0.1:8910/healthz
# 写入一条后查库：
#   SELECT length(embedding::real[])  -- 或 vector_dims(embedding)
# 维度链：EMBEDDING_DIM = EVAL_VECTOR_DIM = len(实际返回向量) = 2048
```

注意：`EMBEDDING_BASE_URL` 不带 `/v1` 后缀（client 内部拼 `/v1/embeddings`）。
未设置 `EMBEDDING_*` 时服务退化为 keyword-only 召回——**学术榜禁止以该
形态参评**（keyword-only 无 embedding 不合规，且会被如实申报击穿）。

## 2. 请求体与媒体护栏（新增，2026-10-24）

| 环境变量 | 默认 | 语义 |
|----------|------|------|
| `EVAL_MAX_BODY_BYTES` | `0`（关闭） | Add/Search 请求体上限；超限返回 413。平台契约 Add 上限 30 MiB，压测期建议 `32505856`（31 MiB） |
| `EVAL_MEDIA_DIR` | 空（关闭） | 原图存证目录；置空时图像分片仅确认不解码 |
| `EVAL_MAX_IMAGE_BYTES` | `10485760`（10 MiB） | 单图解码后上限（契约值） |
| `EVAL_MAX_MEDIA_BYTES` | `31457280`（30 MiB） | 单次 Add 全部图像解码累计上限（契约值） |

行为要点（全部有单测守护）：

- `messages[].content` 与 `query` 同时容忍 string 与有序 ContentPart
  数组；part 类型只认 `text` / `image_url`，未知类型 warn 跳过、绝不 400。
- `image_url` 仅接受内联 Base64 Data URI（JPEG/PNG/WebP）；远程 URL、
  缺 base64 标记、非法 mime 一律跳过并 warn，**不**使请求失败（Q23）。
- 纯图像批次返回 200 + `stored` 缺省（omitempty 下为 0），不回写任何
  空文本证据；**caption-only 路线绝不对图像字段 400**。
- 存证路径：`<EVAL_MEDIA_DIR>/<sanitized user_id>/<request_id>-m<i>-p<j>.<ext>`，
  user/request 逐字符消毒（点与分隔符折叠为 `_`），杜绝路径穿越。
- 存证失败（磁盘满等）只 warn，不影响 Add 成功语义——审计副本，非关键路径。

## 3. 代码榜 commit 证据（case_fix / case_feature）

- `fix:` / `feat:`（含 `(scope)` 与 `!` 形态，大小写不敏感）开头的
  消息正文被分类为 `case_fix` / `case_feature`，**不经任何 LLM**。
- 这两类是**非治理 kind**：三重闸门（`evalGovernableKindForStore`、
  `governableMemoryKind`、治理 SQL 白名单）天然豁免 supersede，
  早先的修复证据不会被后来相似修复作废。
- 事实 `metadata_json` 附带 `case_kind` / `case_scope`（scope 取自
  `fix(scope):` 括号），供后续按仓/按域分析与检索分层。
- `fixture:` / `fixes:` / `hotfix:` 等散文形似不匹配（有单测钉死）。

## 4. 参评材料披露义务

- **caption-only 声明**：本链路按 Q23 合规地把图像分片确认为载荷、
  以文本 caption 入存；原图仅在 `EVAL_MEDIA_DIR` 设置时留文件存证，
  检索证据为纯文本（官方确认无图像引用评分规则）。
- embedding 申报：`text-embedding-v4`（DashScope 兼容网关，2048 维）。
- C2 已完成轮的 bge-m3 合规风险维持既有结论，本轮不得复用 C2 库。

## 5. 赛程红线（E2，官网核实）

- 材料提交截止 **2026-10-31 23:59 (UTC+8)**；评测截止 **2026-11-04 23:59**。
- Full 评测 0.5–2 天；**第二次 Full 需在首轮 30 天后**（Q28）——只有一次
  实弹机会；Full 受理即版本冻结（Q29）；参评方自身故障消耗配额（Q27）。
- C3 推送（`dev` 已就绪状态 `ee29fca85`）须用户点头后才执行，推送验证
  必须跑带外 export 的提交态（脏树绿不作数）。
