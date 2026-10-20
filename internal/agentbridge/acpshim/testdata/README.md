# acpshim testdata — M4 P0-5 方言 fixture

> 用途：acp-shim-deepseek / acp-shim-grok（P2/P3）golden 测试的协议样本。
> Q3 拍板：**不引真实厂商 CI Key**——本目录样本为「协议形态忠实的人工合成数据」，
> 真实端点验证走人工 smoke（P4-4 交接清单）。凡与真实抓包冲突，以真实抓包为准
> 并修订本目录（修订注明采样日期与来源）。

## 文件清单

| 文件 | 方言 | 覆盖点 |
|------|------|--------|
| `deepseek_toolcall_stream.sse` | DeepSeek OpenAI 兼容流式 | **tool_calls 按 `index` 增量聚合**（首帧带 id/name，后续帧只带 arguments 片段）；`reasoning_content` 增量帧（deepseek-reasoner）shim 须容忍并转发为文本或丢弃（P2-1 决策：合入 assistant 文本前加 `「思考」` 前缀为可选项，默认丢弃帧但不断流）；`finish_reason:"tool_calls"` 终帧 |
| `deepseek_nonstream_toolcall.json` | DeepSeek 非流式回退 | `stream=false` 下 `message.tool_calls` 完整数组 + `usage` 计数透传 |
| `grok_toolcall_stream.sse` | xAI OpenAI 兼容流式 | role+tool_calls 同帧（xAI 首帧即带 tool_calls）；arguments 整段单帧（xAI 不切片时亦须正确聚合）；终帧 content 与 finish_reason 同帧 |
| `grok_nonstream_toolcall.json` | xAI 非流式回退 | 同 deepseek 结构；`usage` 字段名一致 |
| `codebuddy_meta_degradation.ndjson` | 原生 ACP（codebuddy） | `_meta` 双位置（params 根 + update 内）厂商扩展**透传不丢**（acp/types.go `Meta json.RawMessage`，设计 §F）；tool_call kind=edit 等未知 key 忽略不报错 |

## 关键假设（若真实抓包推翻，须更新 shim 与 fixture）

1. **tool_calls 增量按 `index` 归并**：同一 `index` 的后续 delta 只携带变动字段
   （`function.arguments` 为字符串片段追加，不重复 `id`/`name`）。
2. **id/name 只在首帧出现**：后续增量帧 `id` 可能缺席或为 null，聚合器不得覆盖已存 id。
3. **reasoning_content 与 content 相互独立**：不得把 reasoning 文本混入 fs_write 参数。
4. **`_meta` 可在任意 ACP 对象上**：全部原样透传；消费方按 key 查表，未知 key 忽略。

## claude_code npm 坐标核实（P0-5 结论，2026-10-21）

- 依据：`registry.npmjs.org` packument 缓存（`acp-npm.json`，采样 2026-10-21，
  正本存 `test/research-claude-acp/`）。
- **旧坐标 `@zed-industries/claude-code-acp@0.16.2` 已标记 deprecated**，
  官方迁移文案：*"This package has been renamed to `@agentclientprotocol/claude-agent-acp`"*。
- **权威新坐标：`@agentclientprotocol/claude-agent-acp`**（GitHub: agentclientprotocol/claude-agent-acp），
  全局安装后 bin 为 `claude-agent-acp`；旧 bin `claude-code-acp` 存量安装仍可用但不再收到更新。
- 行动项（ADR-88 P3-1）：`internal/biz/agentbridge/launch.go` 的 claude_code 案
  当前硬编码 `claude-code-acp`——P3-1 时按新坐标决断（兼容探测：先 `claude-agent-acp`，
  回退 `claude-code-acp`，均失败走分级探针 L1）。
