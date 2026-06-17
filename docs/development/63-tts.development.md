# 63 TTS 语音 — 开发计划

> **版本**：2026-06-17（重组三件套边界，修正代码锚点） | **状态**：❌ 未实现（占位模块，无生产 SLA）
> **需求**：[`63-tts.md`](./63-tts.md) · **设计**：[`63-tts.design.md`](./63-tts.design.md)
> **进度真相**：本文件 §2 现状评估 | **EP**：—

---

## 1. 模块定位

TTS 语音合成：将 Agent 的文本回复转换为语音输出，支持多种语音模型和语言。

**代码锚点**：

| 文件 | 行 | 说明 | 状态 |
|------|----|------|------|
| `internal/data/builtin_tools_seed.go` | 78 | `tts` 工具种子注册：`displayName="文本转语音"`、`category="media"`、`riskLevel="medium"`、`enabled=false`、`paramsSchema={"properties":{"text":{"type":"string"},"voice":{"type":"string"}},"required":["text"]}` | ✅ 存在（停用态） |
| `internal/biz/agent_effective_tools.go` | 59 | `toolGroupsMedia = []string{"read_image", "read_document", "read_spreadsheet", "create_image", "tts"}` — `tts` 归入 media 工具组 | ✅ 存在 |
| `internal/biz/agent_effective_tools.go` | 172 | `registryOptInOnlyKeys["tts"] = true` — `tts` 为 opt-in 工具（默认停用，需 profile/allow 显式启用） | ✅ 存在 |
| `api/kratos/tts/v1/` | — | TTS 独立服务 proto | ❌ 不存在（未规划） |
| `internal/biz/tts*.go` | — | TTS Usecase | ❌ 不存在 |
| `internal/data/tts*.go` | — | TTS Repo | ❌ 不存在 |
| `internal/service/tts*.go` | — | TTS Service | ❌ 不存在 |
| 前端 TTS 播放器组件 | — | Chat 消息朗读按钮 + 音频播放 | ❌ 不存在 |

> **结论**：TTS 作为独立模块无任何后端/前端实现；仅存在一个**停用**的 `tts` 内置工具注册项（media 类，opt-in 策略，与 `create_image` 同策略）。工具注册项本身无实际执行逻辑（无 `internal/tools/tts/` 实现），仅作为 catalog 占位。

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| TTS 独立服务（proto/biz/data/service） | ❌ | 无代码实现 |
| `tts` 内置工具注册 | ✅ | `internal/data/builtin_tools_seed.go:78`（停用态） |
| `tts` 工具策略（opt-in） | ✅ | `internal/biz/agent_effective_tools.go:172`（`registryOptInOnlyKeys`） |
| `tts` 工具实际执行逻辑 | ❌ | 无 `internal/tools/tts/` 实现，仅 catalog 占位 |
| 语音模型集成（Provider API） | ❌ | 无 |
| 前端语音播放器 | ❌ | 无前端播放器组件 |
| Agent 设置 TTS 配置 UI | ❌ | [`5-agent-setting.md` §8](./5-agent-setting.md#8-tabagent-语音合成tts) 有交互规格，但未实现 |

---

## 3. 差距与优化

1. **P2**：TTS 独立服务完全未实现，Agent 回复无法以语音形式输出。
2. **P2**：`tts` 工具仅有 catalog 注册，无实际执行逻辑（`internal/tools/tts/` 不存在），即使 opt-in 启用也无法真正调用。
3. **P3**：Agent 设置 TTS 配置 UI 有交互规格（5-agent-setting §8）但未实现。

---

## 4. 开发阶段

- **Phase 1**：TTS 框架搭建 + OpenAI TTS 集成（独立服务 or 工具执行逻辑）
- **Phase 2**：前端语音播放器 + Agent 设置 TTS 配置 UI
- **Phase 3**：多语音模型支持

> 阶段划分待 TTS 功能启动时细化；架构方向见 [`63-tts.design.md` §1](./63-tts.design.md#1-架构设计待定)。

---

## 5. 任务清单

| # | 任务 | 优先级 | EP | 状态 |
|---|------|--------|-----|------|
| 1 | `api/kratos/tts/v1/` proto 定义（若选架构 B/C） | P2 | — | ❌ |
| 2 | TTS Service + Usecase + Repo（若选架构 B/C） | P2 | — | ❌ |
| 3 | `internal/tools/tts/` 工具执行逻辑实现（若选架构 A/C） | P2 | — | ❌ |
| 4 | OpenAI TTS API 集成 | P2 | — | ❌ |
| 5 | 前端语音播放器组件（Chat 朗读按钮） | P2 | — | ❌ |
| 6 | Agent 设置 TTS 配置 UI（5-agent-setting §8） | P2 | — | ❌ |

---

## 6. 验收标准

> 进度状态跟踪于此。状态标记必须与代码真实状态一致（DOC-SYNC-5）。

- [ ] Agent 回复可转换为语音（FR-1）
- [ ] 前端可播放语音回复（FR-2）
- [ ] Agent 设置中可配置 TTS Provider / 音色 / 密钥（FR-3）
- [ ] `tts` 工具启用后可被 Agent 调用并返回语音文件（FR-4）

---

## 7. 依赖与风险

- TTS API 调用产生额外费用（需配额控制，见需求 NFR-2）
- 语音文件存储需考虑空间与清理策略（见需求 NFR-3）
- `tts` 工具当前为 catalog 占位，无执行逻辑；启用前必须先实现 `internal/tools/tts/`

---

*文档版本：2026-06-17 — 重组三件套边界，修正代码锚点（补全 `tts` 工具注册的实际代码位置）；与需求 [`63-tts.md`](./63-tts.md)、设计 [`63-tts.design.md`](./63-tts.design.md) 同步。*
