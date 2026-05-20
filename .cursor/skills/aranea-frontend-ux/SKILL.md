---
name: aranea-frontend-ux
description: >-
  Aranea 前端 UI/UX 设计与实现。在用户要求美化界面、聊天卡片、玻璃材质、昼夜主题、
  或提及 UX/设计规范时使用。项目权威为 docs/frontend/UX.md，本 skill 在其上叠加
  通用产品设计原则（间距、层次、可读性、反模式）。
---

# Aranea 前端 UI/UX

## 权威来源（冲突时以此为准）

1. **`docs/frontend/UX.md`** — 奶油昼 / 玻璃夜 token、玻璃材质、禁止项
2. **`docs/guides/frontend-guide.md`** — 分层、样式落点（`theme/`、`app-global.sass`）
3. **`docs/README.md` §5** — 文档索引

**禁止**：为「好看」引入 UX.md 未允许的霓虹铺满、重投影、硬编码 hex（变量文件除外）、第二套全局 CSS。

## 设计前自检（来自 UI 设计最佳实践）

| 检查 | 要求 |
|------|------|
| 层次 | 用玻璃不透明度 + 边框亮度区分层级，不靠粗色描边 |
| 排版 | 标题阶梯克制（聊天 h1≤20px）；正文 14–15px；行高 1.55–1.75 |
| 色彩 | 中性底 + **一个**强调色（昼 `#E9A23B`，夜 `#00E5FF` 仅焦点/边） |
| 间距 | 8px 网格：气泡内边距 14–16px，段落间距 0.5–0.75em |
| 可扫读 | 长列表/工具表分组或缩进；表格用玻璃底而非饱和色块表头 |
| 状态 | 悬停：玻璃变亮 + 边略提亮；禁用重 box-shadow（昼） |
| 无障碍 | 对比度 WCAG AA；`:focus-visible` 用 `--color-accent` |

## 样式落点

| 内容 | 路径 |
|------|------|
| 新 token | `web/src/css/theme/_css-vars-*.sass` |
| 页面/聊天全局类 | `web/src/css/app-global.sass`（`.chat-page` 等） |
| 布局/动画（仅本组件） | `web/src/components/**` scoped sass |
| 展示组件 | 无 Store/API；props/emits only |

## 聊天消息卡片（专项）

- **结构**：头像外置 → 元信息行（名/时间）→ **玻璃 prose 卡片**（Markdown）
- **助手气泡**：`--glass-elevated`（昼）/ `--glass-surface`（夜）；`1px var(--glass-border)`；**禁止**-success/positive 绿描边
- **用户气泡**：昼实心 `--color-accent`；夜半透明青边（UX §5.1 夜主按钮）
- **Markdown**：`.chat-message-prose`；标题 `--color-text-heading`；链接 `--color-accent`（昼）/ `--color-link`（夜）
- **Quasar**：`q-chat-message` 设 `bg-color="transparent"`；全局覆盖 `.q-message-text` 背景
- **流式**：左侧 3px inset 强调条或轻脉动，**禁止**整卡绿色外框

## 反模式（不要做）

- 粗纯色边框（尤其 `#4CAF7C` / Quasar `positive` 当消息底）
- 标题 24px+ 压在 14px 正文上
- 靛紫渐变当默认主色（与项目金盏花/青霓虹冲突）
- 日间青紫霓虹 glow

## 参考 skill

通用组件模式见项目内 `.cursor/skills/ui-design-brain/`（上游：[ui-design-brain](https://github.com/carmahhawwari/ui-design-brain)）。

## 完成验证

```bash
cd web && pnpm run build
```
