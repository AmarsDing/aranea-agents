## 技术交付物
### 网站分析输出（`analysis.json`）
- 完整的品牌提取：名称、logo、色彩、字体、favicon
- 内容分析：标题、标语、特性、定价、证言、数据、CTA
- 内页导航：定价、特性、关于、证言页面
- 从网站内容检测竞争对手（20+ 已知 SaaS 竞争对手）
- 业务类型和细分分类
- 细分专属钩子和痛点
- 幻灯片生成的视觉上下文定义

### 轮播生成输出
- 6 张视觉连贯的 JPG 幻灯片（768x1376，9:16 比例），由 Gemini 生成
- 结构化的幻灯片 prompt 保存到 `slide-prompts.json`，用于分析关联
- 平台优化的文案（`caption.txt`），含细分相关的话题标签
- TikTok 标题（最多 90 字符），含策略性话题标签

### 发布输出（`post-info.json`）
- 通过 Upload-Post API 在 TikTok 和 Instagram 上同时直接发布到信息流
- TikTok 自动热门音乐（`auto_add_music=true`）以获得更高互动
- 公开可见（`privacy_level=PUBLIC_TO_EVERYONE`）以获得最大触达
- 保存 `request_id` 用于单帖分析追踪

### 分析与学习输出（`learnings.json`）
- 主页分析：粉丝、曝光、点赞、评论、分享
- 单帖分析：通过 `request_id` 获取特定轮播的浏览和互动率
- 累积经验：最佳钩子、最优发布时间、制胜风格
- 为下一条轮播提供的可操作建议
