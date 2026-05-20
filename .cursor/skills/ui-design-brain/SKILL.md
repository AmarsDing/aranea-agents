---
name: ui-design-brain
description: >-
  Generate production-grade UI using component patterns and best practices.
  Use for web interfaces, dashboards, forms. In Aranea repo, always pair with
  aranea-frontend-ux skill — project UX.md overrides palette and glass rules.
license: See https://github.com/carmahhawwari/ui-design-brain/blob/main/LICENSE.txt
---

# UI Design Brain (Aranea 副本)

> **Aranea 项目**：先读 `.cursor/skills/aranea-frontend-ux/SKILL.md` 与 `docs/frontend/UX.md`，再应用下文通用原则。

完整组件库与 60+ 模式见上游仓库：[carmahhawwari/ui-design-brain](https://github.com/carmahhawwari/ui-design-brain)（`components.md`）。

## 核心原则

1. **克制装饰** — 留白是结构；避免粗描边抢内容
2. **排版承载层次** — 明确 h1→h3 阶梯，避免字号跳变过大
3. **一个强色点** — 中性底 + 单一 accent
4. **8px 网格** — 相关元素紧、区块间松
5. **可访问性** — 对比度、焦点环、语义 HTML
6. **避免 AI 俗套** — 紫白渐变、Inter 默认、彩虹 Badge

## 聊天 / 卡片类界面

- 卡片：**阴影或边框二选一**；玻璃项目用边框 + blur
- 长文：最大阅读宽度 ~72ch；列表项行高 ≥1.5
- 表格：表头 subtle 底，勿用饱和绿/紫整块
- 空/加载/错误：简短文案 + 主操作

## 工作流

1. 识别组件（导航、表单、表格、卡片…）
2. 应用最佳实践（标签在上、主按钮唯一、Modal 可 Esc 关闭）
3. 选定方向（SaaS 默认 / 极简 / 企业密 / 数据面板）— Aranea 固定为奶油+玻璃双模
4. 用项目 token 实现，勿硬编码颜色
