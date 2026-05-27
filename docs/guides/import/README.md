# aranea import — 组织结构批量导入（PGO-4）

> **对应任务**：PGO-4-DOC-01  
> **二进制**：`bin/aranea`（`make cli` 构建）  
> **源码**：`internal/cli/cmd/import.go` + `internal/orgimport/`

---

## 1. 快速开始

```bash
# 构建 CLI
make cli

# 查看帮助
./bin/aranea import --help

# 预览导入计划（dry-run，默认）
./bin/aranea import org spec.yaml

# 实际导入
./bin/aranea import org spec.yaml --apply

# 从 Markdown 提取 YAML 规格并预览
./bin/aranea import org org.md --output-spec extracted.yaml

# 从 Markdown 提取并 AI 优化描述后导入
./bin/aranea import org org.md --refine --apply --correlation-id "my-run-001"
```

---

## 2. 配置

首次使用前配置后端地址和 Token：

```bash
./bin/aranea config set backend.base_url http://localhost:8000
./bin/aranea config set backend.token <your-jwt-token>

# 或通过环境变量
export ARANEA_BASE_URL=http://localhost:8000
export ARANEA_TOKEN=<your-jwt-token>
```

配置文件位于：
- **Linux/macOS**：`~/.config/aranea/config.toml`
- **Windows**：`%APPDATA%\aranea\config.toml`

---

## 3. 规格文件格式（YAML spec）

```yaml
meta:
  version: "1"
  author: "ops-team"

spec:
  industries:
    - key: "fin-001"
      name: "金融科技"
      description: "面向 C 端用户的数字金融服务，受《网络安全法》与银监会规定约束。"
      departments:
        - key: "fin-001-risk"
          name: "风控部"
          description: "负责贷款审批、欺诈检测与合规监控。"
          positions:
            - key: "fin-001-risk-analyst"
              name: "风控分析师"
              description: "利用数据模型评估借款人信用风险，输出评级报告。"
              agents:
                - key: "credit-risk-bot"
                  name: "信贷风险 Agent"
                  description: "基于申请数据和历史行为自动生成风险评分报告。"
                  prompt_files:
                    IDENTITY.md: |
                      # IDENTITY
                      ## Persona
                      专业、严谨的风控分析助理。
                    RULE.md: "# RULE\n禁止泄露客户隐私数据。"
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `meta.version` | ✅ | 规格版本，当前为 `"1"` |
| `spec.industries[].key` | ✅ | 全局唯一 key，导入幂等性依赖此字段 |
| `spec.industries[].name` | ✅ | 显示名称 |
| `spec.industries[].description` | 否 | 行业说明（建议 200 字以内） |
| `departments[].key` | ✅ | 部门唯一 key |
| `positions[].key` | ✅ | 岗位唯一 key |
| `agents[].key` | ✅ | Agent 唯一 key（等同于 `agent_key`） |
| `agents[].prompt_files` | 否 | `文件名: 内容` 映射，支持多文件 |

---

## 4. Markdown 输入（AI 提取模式）

当输入文件为 `.md` 或 `.markdown` 时，CLI 自动调用 `/v1/ai/refine` 的 `SPEC_EXTRACT` scope 将 Markdown 文档提取为合法 YAML。

```bash
# 查看提取结果（不导入）
./bin/aranea import org company-overview.md --output-spec extracted.yaml
cat extracted.yaml

# 提取后直接导入
./bin/aranea import org company-overview.md --apply
```

**注意**：Markdown 提取需要后端配置 `DefaultRefineLLM` 或有可用的 LLM Catalog 条目。

---

## 5. AI 优化模式（--refine）

`--refine` 标志对 spec 中每个 `description` 和 `agent_description` 字段调用 AI 优化。

```bash
./bin/aranea import org spec.yaml --refine --dry-run
# 先用 dry-run 预览优化结果，再加 --apply 执行
./bin/aranea import org spec.yaml --refine --apply
```

---

## 6. 幂等性保证

导入操作按 `key` 字段实现幂等：

- **首次导入**：对应 key 不存在时，创建新资源（HTTP `POST`）
- **重复导入**：对应 key 已存在时，更新字段（HTTP `PUT`）
- **未变更**：内容相同时，跳过（`Skipped` 计数加 1）

因此对同一 spec 多次运行 `--apply` 是安全的。

---

## 7. 审计

每次 `--apply` 调用都会在 HTTP 请求头中携带：

- `X-Correlation-Id`：默认自动生成（`cli-import-<timestamp>`），可通过 `--correlation-id` 指定
- `X-Source`：固定为 `cli_import`

后端 audit middleware 会将这些信息记录到 audit log，在 Datadog 中可按 `source:cli_import` 过滤。

---

## 8. 命令参数一览

```
aranea import org <spec-file> [flags]

Flags:
  --apply                  实际写入（覆盖 --dry-run）
  --dry-run                仅打印计划，不写入（默认 true）
  --refine                 对每个 description 字段调用 AI 优化
  --output-spec string     保存提取的 YAML 规格到此路径（仅 Markdown 输入时有效）
  -o, --output string      输出格式: text | json（默认 "text"）
  --timeout int            每次 HTTP 调用超时（秒，默认 120）
  --correlation-id string  审计追踪 ID（默认自动生成）
```

---

## 9. 常见问题（FAQ）

**Q: 导入后分类在页面上看不到？**  
A: 检查 `key` 是否符合规范（字母、数字、中划线，≤ 128 字符），以及 `enabled` 字段是否为 `true`（默认值）。

**Q: AI 提取失败，返回 REFINE_NO_LLM？**  
A: 请在管理后台「系统设置 → AI Refine LLM」中配置平台默认模型，或确保 LLM Catalog 中有可用条目。

**Q: 导入报 409 Conflict？**  
A: 该 key 已存在但 Applier 在 GET 查询时出现竞争。可安全重试，幂等机制会正确处理。

**Q: 如何只导入特定层级（如只导入 Agent，不动分类）？**  
A: 目前不支持部分导入，规格中填写完整结构并确保分类 key 已存在即可；Applier 会对已存在的条目执行 `Updated`（空更新）而不是报错。

---

## 10. 相关文件

| 文件 | 说明 |
|------|------|
| `cmd/aranea/main.go` | CLI 入口 |
| `internal/cli/cmd/import.go` | `aranea import` 命令实现 |
| `internal/orgimport/spec.go` | YAML 规格 struct |
| `internal/orgimport/validator.go` | 校验逻辑 |
| `internal/orgimport/planner.go` | Plan 生成 |
| `internal/orgimport/applier.go` | 幂等写入实现 |
| `internal/orgimport/loader.go` | YAML / Markdown 加载 |
| `docs/guides/ai-refine.md` | AI Refine 服务说明 |
