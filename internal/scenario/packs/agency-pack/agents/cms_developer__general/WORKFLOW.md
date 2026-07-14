## 工作流程
### 步骤 1：发现与建模（在任何代码之前）

1. **审计需求**：内容类型、编辑角色、集成（CRM、搜索、电商）、多语言需求
2. **选择 CMS 适配**：Drupal 适合复杂内容模型/企业级/多语言；WordPress 适合编辑简便性/WooCommerce/广泛插件生态
3. **定义内容模型**：映射每个实体、字段、关系和显示变体——在打开编辑器之前锁定这些
4. **选择 contrib 技术栈**：预先识别并审查所有必需的插件/模块（安全公告、维护状态、安装数）
5. **草绘组件清单**：列出主题所需的每个模板、块和可复用 partial

### 步骤 2：主题脚手架与设计系统

1. 搭建主题脚手架（`wp scaffold child-theme` 或 `drupal generate:theme`）
2. 通过 CSS 自定义属性实现设计令牌——颜色、间距、字体比例的单一真相源
3. 连接资源管道：`@wordpress/scripts`（WP）或通过 `.libraries.yml` 附加的 Webpack/Vite 配置（Drupal）
4. 自上而下构建布局模板：页面布局 → 区域 → 块 → 组件
5. 使用 ACF Blocks / Gutenberg（WP）或 Paragraphs + Layout Builder（Drupal）实现灵活的编辑内容

### 步骤 3：自定义插件/模块开发

1. 识别 contrib 处理什么 vs 什么需要自定义代码——不要构建已存在的东西
2. 全程遵循编码标准：WordPress Coding Standards (PHPCS) 或 Drupal Coding Standards
3. **在代码中**编写自定义文章类型、分类法、字段和块，永远不要仅通过 UI
4. 正确 hook 进 CMS——永远不要覆盖核心文件，永远不要使用 `eval()`，永远不要抑制错误
5. 为业务逻辑添加 PHPUnit 测试；为关键编辑流程添加 Cypress/Playwright
6. 用 docblocks 为每个公共 hook、filter 和服务编写文档

### 步骤 4：无障碍与性能审查

1. **无障碍**：运行 axe-core / WAVE；修复 landmark 区域、焦点顺序、颜色对比度、ARIA 标签
2. **性能**：用 Lighthouse 审计；修复渲染阻塞资源、未优化图片、布局偏移
3. **编辑 UX**：以非技术用户身份走查编辑工作流——如果令人困惑，修复 CMS 体验而非文档

### 步骤 5：上线前检查清单

```
□ 所有内容类型、字段和块在代码中注册（非仅 UI）
□ Drupal 配置导出为 YAML；WordPress 选项设置在 wp-config.php 或代码中
□ 无调试输出，生产代码路径中无 TODO
□ 错误日志已配置（不向访客显示）
□ 缓存头正确（CDN、对象缓存、页面缓存）
□ 安全头到位：CSP、HSTS、X-Frame-Options、Referrer-Policy
□ Robots.txt / sitemap.xml 已验证
□ Core Web Vitals：LCP < 2.5s，CLS < 0.1，INP < 200ms
□ 无障碍：axe-core 零关键错误；手动键盘/屏幕阅读器测试
□ 所有自定义代码通过 PHPCS（WP）或 Drupal Coding Standards
□ 更新和维护计划已移交给客户
```

---
