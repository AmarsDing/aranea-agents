## 工作流程
### 阶段 1：从历史中学习
1. **获取分析数据**：通过 `check-analytics.sh` 调用 Upload-Post 分析端点获取主页指标和单帖表现
2. **提取洞察**：运行 `learn-from-analytics.js` 识别表现最佳的钩子、最优发布时间和互动模式
3. **更新经验**：将洞察累积到 `learnings.json` 持久知识库
4. **规划下一条轮播**：读取 `learnings.json`，从顶级表现中选择钩子风格，在最优时间调度，应用建议

### 阶段 2：研究与分析
1. **网站抓取**：运行 `analyze-web.js` 对目标 URL 进行完整的 Playwright 分析
2. **品牌提取**：色彩、字体、logo、favicon 用于视觉一致性
3. **内容挖掘**：从所有内页提取特性、证言、数据、定价、CTA
4. **细分检测**：分类业务类型并生成细分合适的叙事
5. **竞争对手映射**：识别网站内容中提及的竞争对手

### 阶段 3：生成与验证
1. **幻灯片生成**：运行 `generate-slides.sh`，通过 `uv` 调用 `generate_image.py`，用 Gemini（`gemini-3.1-flash-image-preview`）创建 6 张幻灯片
2. **视觉连贯**：第 1 张来自文本 prompt；第 2-6 张使用 Gemini 图生图，以 `slide-1.jpg` 作为 `--input-image`
3. **视觉验证**：Agent 使用自身的视觉模型检查每张幻灯片的文字清晰度、拼写、质量，以及底部 20% 无文字
4. **自动重新生成**：如果任何幻灯片失败，用 Gemini 仅重新生成该张（使用 `slide-1.jpg` 作为参照），重新验证直到全部 6 张通过

### 阶段 4：发布与追踪
1. **多平台发布**：运行 `publish-carousel.sh` 将 6 张幻灯片推送到 Upload-Post API（`POST /api/upload_photos`），参数为 `platform[]=tiktok&platform[]=instagram`
2. **热门音乐**：`auto_add_music=true` 在 TikTok 上添加热门音乐以获得算法加成
3. **元数据捕获**：将 API 响应中的 `request_id` 保存到 `post-info.json` 用于分析追踪
4. **用户通知**：仅在一切成功后才报告已发布的 TikTok + Instagram URL
5. **自我调度**：读取 `learnings.json` 的 bestTimes，在最优时段设置下次 cron 执行
