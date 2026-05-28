# UI 执行规范（AI）

**系统**：奶油昼 · 玻璃夜。**约束**：下文数值与 token 为**唯一权威**；实现须逐字使用，禁止替换为「相近」色或省略 `-webkit-backdrop-filter`。  
**昼夜**：布局、间距、圆角、字号阶梯**不变**；只换色与材质。**Quasar**：`Dark.set()` → `body.body--dark`，样式分叉用 `body:not(.body--dark)` / `body.body--dark`。

---

## 1. 强制自检（实现前）

| 检查项 | 要求 |
|--------|------|
| 玻璃材质 | 所有浮层必须用半透明 + `backdrop-filter` / `-webkit-backdrop-filter`，blur 按层级 12–24px（移动端可降至 8–12px） |
| 边框 | 半透明色；**禁止**纯黑/纯白实线作玻璃边框 |
| 阴影 | 日间优先 **无常规 box-shadow**，用玻璃厚度与边框表达层级；夜间用光晕替代重阴影 |
| 双模 | 间距、圆角、字体阶梯、布局结构 **日夜一致**，仅换语义色与材质参数 |
| 日间交互增强 | 以 **金盏花锚点** `#E9A23B`（悬停 `#D48C1A`）贯连主按钮、文字链接、`:focus-visible` 环与表单聚焦边；玻璃层交互靠 **提高不透明度 + blur+2px**（如 `0.65→0.78`）与 **边框略提亮**，可叠 **极细暖色内高光**（如 `inset 0 1px 0 rgba(255,255,255,0.45)`），**禁用**青紫霓虹、强 `box-shadow` 与冷色 glow；按压可用 **轻微 `scale`**，次级控件悬停可用奶油衬底 `#FEF3E4` |
| 夜间霓虹 | `#00E5FF` / `#A855F7` **仅用于**交互焦点、强调边、动态渐变，避免铺满界面；**日间不得**使用该组色作默认强调（避免昼夜调性串味） |

### 0.1 日间交互增强（实现速查）

| 场景 | 推荐处理 | 避免 |
|------|----------|------|
| 主操作 | 实心或高对比填充 `#E9A23B`，字 `#FFFFFF`，悬停 `#D48C1A` | 大区域渐变抢镜 |
| 链接 / 次要强调 | 字色或下划线用 `#E9A23B`，悬停加深为 `#D48C1A` | 夜间霓虹青紫 |
| `:focus-visible` | `outline` / `ring` 使用 `#E9A23B`（或 `rgba(233,162,59,0.45)` 2px），与背景对比足够 | 仅依赖浏览器默认蓝环 |
| 可点击玻璃卡片 | 悬停：`rgba(255,253,245,0.78)` + `blur(20px)`，边框可向 `rgba(235,220,200,0.85)` 过渡 | 突然加深阴影代替材质变化 |
| 图标按钮 | 默认 `#B8A590`，悬停/激活 `#E9A23B` 或 `#3A322C`（按层级） | 彩虹或霓虹描边 |

---

## 2. CSS 变量（`:root` / `body.body--dark`）

**实现位置**：`web/src/css/theme/_css-vars-light.sass`（`:root`）、`_css-vars-dark.sass`（`body.body--dark`）；聚合入口 `app-theme.sass`。页面/组件用 **`var(--*)`**，勿硬编码 hex（除本文档明确要求处）。

### 2.1 日间（`:root`）

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#FEFBF4` | 主画布底 |
| `--glass-surface` | `rgba(255, 253, 245, 0.65)` | 标准玻璃 |
| `--glass-surface-hover` | `rgba(255, 253, 245, 0.78)` | 玻璃悬停 |
| `--glass-blur-default` | `18px` | 与 surface 配对 |
| `--glass-blur-hover` | `20px` | 悬停略增 |
| `--glass-border` | `rgba(235, 220, 200, 0.7)` | 玻璃边 |
| `--glass-elevated` | `rgba(255, 255, 255, 0.72)` | 弹层 |
| `--glass-blur-elevated` | `24px` | 抬高 blur |
| `--color-accent` | `#E9A23B` | 主操作、链接、焦点 |
| `--color-accent-hover` | `#D48C1A` | 主操作悬停 |
| `--focus-ring-light` | `2px solid rgba(233, 162, 59, 0.45)` | `:focus-visible` |
| `--interaction-surface-hover` | `#FEF3E4` | 次级悬停衬底 |
| `--glass-inner-highlight` | `inset 0 1px 0 rgba(255, 255, 255, 0.45)` | 可选顶缘高光 |
| `--color-text-primary` | `#3A322C` | 主文案 |
| `--color-text-secondary` | `#8B7A6B` | 辅文案 |
| `--color-icon-muted` | `#B8A590` | 图标/线 |
| `--color-success` | `#4CAF7C` | 成功 |
| `--color-warning` | `#F09B54` | 警告 |
| `--color-danger` | `#E55C5C` | 危险 |
| `--nav-bg-light` | `rgba(255, 249, 236, 0.85)` | 日间顶栏底（+ blur） |

### 2.2 夜间（`body.body--dark`）

| Token | 值 | 用途 |
|-------|-----|------|
| `--canvas-base` | `#090D14` | 主画布底 |
| `--glass-surface` | `rgba(18, 24, 34, 0.65)` | 标准玻璃 |
| `--glass-surface-hover` | `rgba(22, 28, 40, 0.75)` | 玻璃悬停 |
| `--glass-border` | `rgba(255, 255, 255, 0.08)` | 细边 |
| `--glass-border-hover` | `rgba(255, 255, 255, 0.16)` | 悬停边 |
| `--color-accent`（夜间语义） | `#00E5FF` | 与 §1「夜间霓虹」一致 |
| `--color-accent-hover` | `#5aebff` | 悬停 |
| `--color-neon-cyan` | `#00E5FF` | 焦点/链接 |
| `--color-neon-violet` | `#A855F7` | 二级强调 |
| `--gradient-flow-border` | `linear-gradient(120deg, #00E5FF, #A855F7, #00E5FF)` | 流动边 |
| `--color-text-primary` | `#EBEBF0` | 主文案 |
| `--color-text-secondary` | `#9CA0B0` | 辅文案 |
| `--color-success` | `#3FE0A0` | 成功 |
| `--color-warning` | `#FFAF4D` | 警告 |
| `--color-danger` | `#FF5E7A` | 危险 |
| `--nav-bg-dark` | `rgba(9, 13, 20, 0.7)` | 顶栏底（+ blur 20px） |
| `--nav-divider-dark` | `rgba(255, 255, 255, 0.06)` | 栏底分割线 |

### 2.3 玻璃表面最小片段

```css
background: var(--glass-surface);
backdrop-filter: blur(var(--glass-blur-default));
-webkit-backdrop-filter: blur(var(--glass-blur-default));
```

---

## 3. 样式工程（放哪里）

| 层级 | 路径 | 职责 |
|------|------|------|
| 构建期 | `web/src/css/quasar-variables.sass` | `$primary` 等（Vite `sassVariables`）；**不**随 Dark 重算 |
| Token | `web/src/css/app-theme.sass` → `web/src/css/theme/*` | `$cream-*`、`:root` / `body.body--dark` |
| 全局类 | `web/src/css/app-global.sass` | 字体、shell、页面级 class；昼夜用 `body:not(.body--dark)` / `body.body--dark` |
| Quasar 链（约定） | `web/src/style.sass` → `web/src/css/style.sass` | 配置里为 `css: ['style.sass']`（相对 `src/`）。**业务样式只改 `css/`**；根文件仅一行 `@import`。`client-entry` 固定 `import 'src/css/style.sass'`。 |

**规则**

1. 新 token → `theme/_css-vars-light.sass` / `_css-vars-dark.sass`（或新增 partial 并在 `app-theme.sass` `@import`）。  
2. 新布局/页面 class → `app-global.sass`；**禁止**第三套自定义主题类名与 Quasar 打架。  
3. 主强调、链接、焦点：**以 `--color-accent` 为准**；`$primary` 仅作组件默认兼容。  
4. **禁止**运行时用脚本改 `quasar-variables`（已编译）；昼夜仅用 Dark + 变量 + body 选择器。

**Token 膨胀**：仅在 `web/src/css/theme/` 增加 `_*.sass`，由 `app-theme.sass` 聚合；可按域拆文件，每文件可含并列的 `\:root` 与 `body.body--dark` 块；**禁止**与 `app-global` 平行的第二套全局 CSS 入口。

参考：[Quasar Dark 插件](https://quasar.dev/quasar-plugins/dark)。

---

## 4. 排版

| 项 | 值 |
|----|-----|
| 展示 | `SF Pro Display, Inter Tight, Helvetica Neue, sans-serif` |
| 正文 | `SF Pro Text, Inter, Helvetica Neue, sans-serif` |
| 字色 | 昼 `var(--color-text-primary)` / 夜同 token |
| 标题 | 负字距、偏紧行高 |
| 夜间标题（特殊模块可选） | `text-shadow: 0 0 12px rgba(0, 229, 255, 0.15)` |

字号阶梯（若项目另有全局阶梯文档）与之对齐；**切换昼夜不改阶梯与行高体系**。

---

## 5. 组件（数值）

### 5.1 按钮

| 模式 | 背景 | 字/边 | 悬停 | 圆角 / 其它 |
|------|------|--------|------|-------------|
| 昼·主 | `#E9A23B` | `#fff` | `#D48C1A` | 圆角 `10px`；内边距 `10px 20px`；可按 `scale(0.98)` |
| 昼·次 | 透明 | 字 `#3A322C`，边 `1px solid #D0C0A8` | 底 `#FEF3E4` | — |
| 昼·玻璃次 | `rgba(255,253,245,0.5)` + blur | 边 `#EDE3D3` | 按 §1 玻璃交互 | — |
| 夜·主（霓虹） | `rgba(0,229,255,0.15)` | 边 `1px solid #00E5FF`，字 `#00E5FF` | alpha→0.25；`box-shadow: 0 0 16px rgba(0,229,255,0.3)` | — |
| 夜·次 | 玻璃材质 | 边 `rgba(255,255,255,0.1)`，字 `#EBEBF0` | — | — |
| 胶囊 CTA | — | — | — | 圆角 `18px–980px` 按需 |

### 5.2 卡片

| 模式 | 规则 |
|------|------|
| 昼·玻璃（默认浮层） | `rgba(255,253,245,0.65)` + blur 18px；边 `rgba(235,220,200,0.7)`；**无**重投影 |
| 昼·实体（慎用） | 底 `#FFFDF5`；边 `#EDE3D3`；影 `0 2px 12px rgba(0,0,0,0.04)`；圆角 16–20px。**同一层级勿与玻璃混用** |
| 夜 | `rgba(18,24,34,0.65)` + blur 18px + webkit；边 `rgba(255,255,255,0.08)`；悬停边 → `--glass-border-hover`；选中可 `box-shadow: 0 0 20px rgba(0,229,255,0.15)` |

### 5.2a 对话框（`q-dialog` 内主卡片）

| 规则 |
|------|
| 背景 **`var(--glass-elevated)`**，**`backdrop-filter` + `-webkit-backdrop-filter`** 使用 **`var(--glass-blur-elevated)`**；边框 **`var(--glass-border)`**；圆角 **20–24px**（与 §7 面板一致） |
| 主按钮：优先 **`var(--color-accent)`** / **`var(--color-accent-hover)`**（随昼夜 token 切换），**禁止**在日间把霓虹青紫当默认主色（见 §1） |

### 5.3 输入

| 模式 | 规则 |
|------|------|
| 昼·实体 | 底 `#fff`；边 `#D0C0A8`；聚焦边 `#E9A23B`；字 `#3A322C` |
| 昼·玻璃 | 底 `rgba(255,255,255,0.5)` blur 8px；边 `rgba(208,192,168,0.6)`；聚焦 `#E9A23B` |
| 夜 | 底 `rgba(18,24,34,0.45)` blur 8px；边 `rgba(255,255,255,0.1)`；聚焦 `#00E5FF` + 微光晕 |
| 圆角 | `12px–16px` |

### 5.4 导航 / 工具条

| 模式 | 规则 |
|------|------|
| 昼 | 底 `rgba(255,249,236,0.85)` + blur；字暖棕系；可无下边线，或 `1px solid rgba(235,220,200,0.6)`；奶霜顶栏可用 `rgba(255,253,245,0.72)` + blur 20px |
| 夜 | 底 `rgba(9,13,20,0.7)` blur 20px；底分割 `1px`、`rgba(255,255,255,0.06)`；链接 `#EBEBF0`，悬停点亮 `#00E5FF` |

### 5.5 媒体

昼：图贴在奶油底上。夜：可略压暗或加玻璃蒙层；产品图夜间仅**极弱**青辉，勿抢主体。

---

## 6. 夜间特效（可选）

| 用途 | 要点 |
|------|------|
| 流动边 | `border-image` 或背景渐变；色 `#00E5FF` ↔ `#A855F7` |
| 扫描光 | 大面积 Hero 伪元素慢速带；**不挡阅读** |
| 霓虹 | `filter: drop-shadow(0 0 8px #00E5FF)` 小面积 |
| 移动降级 | 流光改静态渐变；blur 见 §1 |

---

## 7. 布局

| 项 | 规则 |
|----|------|
| 间距刻度 | `4, 8, 12, 16, 20, 24, 32, 48, 64` px；**昼夜同一套** |
| 圆角 | 控件 5–8px；卡片/面板 16–20px；大模块 28–36px；胶囊 56–980px；圆 `50%` |
| 密度 | 营销宽留白；数据页可更密 |
| 表单区宽度 | 单列表单 `max-width: 960px`（`.app-form-shell` / `.settings-grid`）；双栏能力卡 `1200px`（`.app-form-wide`） |
| 字段宽度 | 短字段 `320px`（`.app-field-sm`）；名称/下拉 `480px`（`.app-field-md`）；描述/Prompt `72ch`（`.app-field-long`） |
| 字段网格 | 多列配置用 `.app-form-field-grid`（`auto-fill, minmax(200px, 280px)`），禁止裸 `col-12` 拉满超宽屏 |
| 按钮 | 主操作 **auto width**，`.app-actions-bar` 右对齐；**禁止**配置页 `full-width` 主按钮（移动 `.app-btn-block-mobile` 除外） |
| Chat composer | `.chat-composer-inner` 限宽 `900px` 居中，与消息区对齐 |
| Dialog | `.app-dialog-card` + `--glass-elevated`；宽 `640–860px` |
| Z / 层级 | L0=`--canvas-base`；L1/L2 用玻璃不透明度与 blur 差、边框亮度区分；焦点昼 `#E9A23B`、夜霓虹边。**摒弃**靠重投影分层层级 |

---

## 8. Do / Don't

**Do**：全昼夜浮层磨砂玻璃；昼奶油 rgba(255,253,245,…)；夜深半透明 + 微光；强调仅交互锚点；层级靠模糊与边框。

**Don't**：昼默认大白实心不透明大块；默认重阴影堆层级；同层混实体与玻璃；玻璃上大色块糊满；移动端忽视性能（须降 blur / 简化光效）。

### 8.1 交互安全规范

| 场景 | 要求 |
|------|------|
| 破坏性操作 | 删除、回滚、终止、清除等不可逆操作**必须**有 `$q.dialog` 二次确认 |
| 表单提交 | 必填字段**必须**有 `:rules` 前端校验；提交按钮**必须**有 `:disable` 绑定 |
| 编辑器关闭 | 有未保存变更时关闭**必须**弹出确认；使用 `persistent` + dirty 检测 |
| IME 输入 | Chat 发送须同时检查 `event.isComposing` 和 `event.keyCode === 229` |
| 收藏/置顶 | 单字段切换只 patch 该字段，不提交完整 form |
| 加载失败 | 不使用 `router.back()` 强制跳转；显示错误页 + 重试按钮 |
| 删除按钮 | 列表项删除按钮应 hover 时才显示（`opacity: 0 → 1`），不始终暴露 |

---

## 9. 响应式

断点跟随项目全局配置。移动：blur **8–12px**；§6 动效降级静态。

---

## 10. AI 提示片段（短句）

- **昼卡片**：底 `#FEFBF4`；卡片 `rgba(255,253,245,0.65)` blur 18px；边 `rgba(235,220,200,0.7)`；主按钮 `#E9A23B`；无重投影。  
- **夜面板**：底 `#090D14`；面板 `rgba(18,24,34,0.65)` blur；边 `rgba(255,255,255,0.08)`；强调 `#00E5FF`。  
- **对话框**：内容卡 `--glass-elevated` + `blur(--glass-blur-elevated)` **双前缀**；边 `--glass-border`；主 CTA `--color-accent`。
