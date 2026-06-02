# Web Research — P1–P3 修复（2026-05-23）

## 摘要

对 `web_research`（Tavily / SerpAPI）Review 项 P1–P3 落地：统一配置解析、目录/有效工具与在线测试一致、单 deadline 与补抓告警、代理贯穿 enrich、种子 schema 与存量库同步、测试与文档。

## P1 — 配置一致性

- 新增 `internal/tools/webresearch/resolve.go`：`ResolveConfig`、`MergePlatformConfig`、`CatalogReady`。
- 工具目录 `runtime_status` 读取系统设置中的 API Key（`HasAPIKey` / 明文 key）。
- `ToolUsecase` / `AgentUsecase` 注入 `SystemSettingRepo`，列表与有效工具矩阵共用平台配置。
- 有效工具：`web_research` 策略允许但无可用密钥时 `enabled=false`，`reason=missing_api_key`。
- 工具页在线测试 `testexec.Execute` 合并系统设置，与「测试连接」一致。

## P2 — 运行时质量

- `web_research.Call` 使用单次 `context.WithTimeout`（搜索 + 补抓共享）。
- 补抓失败或单 URL 错误写入 `partial` / `fetch_warnings`。
- `enrichHits` 经 `httpfetch.WithHTTPClient(buildHTTPClient(cfg))` 使用平台 `http_proxy`。
- `truncateUTF8` 按 rune 截断正文。
- 种子 `config_schema_json`；启动时 `syncBuiltinWebToolCatalogPatches` 更新 `duckduckgo_search`（默认关）、`web_research`、`web_fetch` schema。

## P3 — 测试与文档

- `resolve_test.go`、`integration_test.go`（httptest 模拟 Tavily）、`web_research_runtime_test.go`。
- `TestWebResearch` 错误改为 Kratos `WEB_RESEARCH` BadRequest（英文消息）。
- 系统设置 UI：`search_depth` 在 SerpAPI 时已禁用（既有逻辑保留）。

## 配置优先级（不变）

Agent `config_json` → 系统设置 `web_research_*` → `TAVILY_API_KEY` / `SERPAPI_API_KEY`。
