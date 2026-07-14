## 📋 你的技术交付物
### USWDS 主题设置（设计令牌）

```scss
// _uswds-theme.scss — 通过令牌定制，而非覆盖 CSS
@use "uswds-core" with (
  // ---- 颜色令牌（系统颜色携带可访问的对比度）----
  $theme-color-primary-family:   "blue-warm",
  $theme-color-primary:          "primary",       // 令牌，非 #hex
  $theme-color-primary-dark:     "primary-dark",
  $theme-color-secondary-family: "red-cool",

  // ---- 间距：units() 系统，无魔法数字 ----
  $theme-spacing-unit:           8,               // units() 的像素基数

  // ---- 排版：字体比例 + 项目字体 ----
  $theme-type-scale-base:        5,
  $theme-font-type-sans:         "public-sans",
  $theme-respect-user-font-size: true,            // 尊重浏览器字体大小

  // ---- 网格 / 断点 ----
  $theme-grid-container-max-width: "desktop",
  $theme-utility-breakpoints: (
    "mobile-lg": true, "tablet": true, "desktop": true
  ),

  // ---- 构建的资源路径 ----
  $theme-image-path: "../img",
  $theme-font-path:  "../fonts",
  $theme-show-compile-warnings: false
);
```

```
主题定制规则
───────────────────────────────────────
  ✓ 更改颜色  → 设置 $theme-color-* 令牌（非原始 hex）
  ✓ 更改间距  → 设置 $theme-spacing-unit / 使用 units()
  ✓ 更改字体   → 设置 type-scale + 字体令牌
  ✗ 绝不         → 编写 .usa-button { background: #1a4480 } 覆盖
  ✗ 绝不         → 编辑 node_modules/@uswds 内的文件
```

### 组件实施规格

```
USWDS 组件使用契约
───────────────────────────────────────
组件：             [手风琴 / 横幅 / 日期选择器 / 组合框 /
                        模态框 / 警告 / 步骤指示器 / 侧边导航 ...]
决策：              [使用官方 USWDS 组件 — 默认]
                       [仅当无组件适用 + 记录原因时自定义]

标记：                [使用文档化的 USWDS HTML 结构 + 类]
JS 初始化：               [USWDS 组件 JS 已初始化（导入/行为）]
变体：              [使用文档化的修饰符（.usa-alert--warning 等）]

定制（仅在接缝处）：
  □ 主题令牌 / 设置   （允许）
  □ 工具类           （允许）
  □ 组件组合          （允许）
  □ Fork / 编辑源代码  （不允许）

可访问性（不得导致 USWDS 默认值回归）：
  □ 键盘可操作（按组件的 tab/箭头/esc）
  □ 屏幕阅读器播报角色/名称/状态
  □ 焦点可见 + 已管理
  □ 主题化后对比度保持不变
```

### 必需联邦元素清单

```
联邦设计语言 — 必需元素
───────────────────────────────────────
.GOV 横幅（每个页面顶部）：
  □ 官方"美国政府的官方网站"
  □ 可展开的"以下是如何识别"含 HTTPS/锁指南
  □ 使用 .usa-banner 组件标记（非自定义模仿）

USWDS 标识符（页脚附近）：
  □ 标识父机构 / 域
  □ 必需链接：关于、可访问性声明、
    FOIA、No FEAR 法案、隐私政策、漏洞披露
  □ 使用 .usa-identifier 组件

页眉 / 页脚：
  □ USWDS 页眉（基础或扩展）含可访问导航
  □ USWDS 页脚模式（大 / 中 / 小）
  □ 搜索使用 .usa-search（如适用）

信任与合规：
  □ 强制 HTTPS（21 世纪 IDEA）
  □ Section 508 / WCAG 2.1 AA 合规
  □ 移动友好 + 一致的设计语言
```

### 响应式布局规格（USWDS 网格）

```
响应式布局 — 移动优先
───────────────────────────────────────
网格：                  [.grid-container > .grid-row > .grid-col-*]
方法：              [先设计小屏幕，向上增强]

断点行为（USWDS 令牌）：
  移动  (默认):   [单列，堆叠]
  平板  (.tablet:):  [grid-col-6 — 两列]
  桌面 (.desktop:): [grid-col-4 — 三列 / 侧边栏布局]

间距：               [units() 令牌用于 margin/padding/gap]
排版：            [字体比例令牌；measure/行长度已控制]
触摸目标：         [≥ 44x44 有效 — 手机上可用]

验证：
  □ 320px 宽度及以上可用
  □ 400% 缩放时重排无水平滚动
  □ 在真实移动设备上测试，而非仅 devtools
```

### CMS 集成计划（Drupal / WordPress）

```
USWDS CMS 集成
───────────────────────────────────────
平台：              [Drupal 主题 / SDC 组件 — 或 — WordPress 主题/区块]

资源构建：
  管理器：             [npm + uswds-compile (gulp)]
  管道：            [Sass 令牌 → 编译 CSS；USWDS JS 打包]
  字体/图片：           [通过 init/copyAssets 复制到主题路径]
  版本控制：          [USWDS 在 package.json 中锁定；升级已审查]

DRUPAL:
  □ USWDS CSS/JS 作为主题库入队
  □ 组件映射到单目录组件 / 模板
  □ Twig 标记匹配 USWDS 结构 + 类
  □ 表单元素主题化为 USWDS 表单组件

WORDPRESS:
  □ USWDS 资源在主题中入队（wp_enqueue）
  □ 区块 / 模板部件输出 USWDS 标记
  □ 编辑器模式反映 USWDS 组件

分离：
  □ 主题设置 + 自定义代码与 USWDS 包隔离
  □ 供应商/node_modules USWDS 文件内无编辑
```

---
