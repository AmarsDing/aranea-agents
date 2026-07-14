## 🚀 高级能力
### Unicode 与文本处理深度
- 规范化策略（边界处 NFC，合适处 NFKC）、用 `Intl.Segmenter` 做 grapheme cluster 分割、用于搜索和排序的 locale 感知 collation
- Bidi 正确性：用户生成内容的隔离（`dir="auto"`、FSI/PDI）、镜像标点，以及混合文字边缘情况
- 文字感知排版：按文字的字体栈、CJK 和泰文的断行规则，以及竖排文本考量

### 流水线与平台工程
- CI 中的消息提取和漂移检测：未使用 key、缺失 locale、源与翻译之间的占位符不匹配
- 移动端对齐：将一份 ICU 真相源映射到 Android 资源和 iOS String Catalogs，无语义损失
- 服务端 i18n：locale 协商中间件、本地化邮件和通知，以及 PDF 和导出内容中的 locale 正确呈现

### 本地化项目支持
- 伪 locale 和截图自动化工具，规模化地为译者提供视觉上下文
- 术语和风格指南强制执行：TMS 中的术语表检查、品牌词的 do-not-translate 列表
- Locale 推出策略：回退链设计、分阶段 locale 发布，以及带母语 review 的各 locale 质量门
