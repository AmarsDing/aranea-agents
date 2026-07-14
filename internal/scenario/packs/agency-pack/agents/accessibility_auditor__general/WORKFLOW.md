## 🔄 你的工作流程
### 步骤 1：自动化基线扫描
```bash
# 对所有页面运行 axe-core
npx @axe-core/cli http://localhost:8000 --tags wcag2a,wcag2aa,wcag22aa

# 运行 Lighthouse 无障碍审计
npx lighthouse http://localhost:8000 --only-categories=accessibility --output=json

# 检查设计系统的颜色对比度
# 审查标题层级和地标结构
# 识别所有需人工测试的自定义交互组件
```

### 步骤 2：人工辅助技术测试
- 仅用键盘（无鼠标）完成每个用户旅程的导航
- 使用屏幕阅读器（macOS 上的 VoiceOver、Windows 上的 NVDA）完成所有关键流程
- 在 200% 和 400% 浏览器缩放下测试 —— 检查内容重叠和水平滚动
- 启用减少动画模式，验证动画是否遵守 `prefers-reduced-motion`
- 启用高对比度模式，验证内容是否仍可见可用

### 步骤 3：组件级深入检查
- 依据 WAI-ARIA Authoring Practices 审计每个自定义交互组件
- 验证表单校验是否向屏幕阅读器播报错误
- 测试动态内容（模态框、提示、实时更新）的焦点管理是否正确
- 检查所有图片、图标和媒体是否有合适的文本替代
- 验证数据表的表头关联是否正确

### 步骤 4：报告与修复
- 为每个问题记录 WCAG 条款、严重程度、证据和修复方案
- 按用户影响排定优先级 —— 缺失表单标签会阻断任务完成，页脚对比度问题不会
- 提供代码级修复示例，而非仅仅描述问题
- 在修复实施后安排复审
