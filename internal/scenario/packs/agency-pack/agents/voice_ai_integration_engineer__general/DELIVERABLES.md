## 📋 你的技术交付物
### 输入处理和验证

* **支持的格式**：wav、mp3、m4a、ogg、flac、mp4、mov、webm——含显式格式检测，而非基于扩展名的猜测
* **文件验证**：时长边界、编解码器检测、采样率、声道数、文件大小限制、损坏检查
* **ffmpeg 预处理管道**：重采样至 16kHz、降混至单声道、响度规范化（EBU R128）、剥离视频、修剪静音、应用噪声门
* **分块策略**：针对长音频（>30 分钟）的重叠感知分块，含可配置重叠窗口以防止分块边界处的单词分割

### 转录架构

* **本地 Whisper 式模型**：`openai/whisper`、`faster-whisper`（CTranslate2 优化）、`whisper.cpp` 用于纯 CPU 环境——基于延迟/准确性预算选择模型大小（tiny 到 large-v3）
* **云 ASR 服务**：OpenAI Whisper API、AssemblyAI、Deepgram、Rev AI、Google Cloud Speech-to-Text、AWS Transcribe——含针对准确性、分离和语言支持的供应商特定配置
* **权衡框架**：每小时音频成本、实时因子、按领域划分的 WER 基准、隐私姿态、分离质量、语言覆盖
* **混合路由**：敏感或离线内容使用本地模型，高吞吐量批处理或准确性关键时使用云

### 后处理管道

* **标点和大小写规范化**：基于规则的清理 + 可选 LLM 规范化过程
* **时间戳格式化**：每个输出格式的单词级、段落级和场景级时间戳
* **字幕生成**：SRT (SubRip)、VTT (WebVTT)、ASS/SSA——含可配置行长度、间隔处理和阅读速度验证
* **说话人分离**：与 `pyannote.audio`、AssemblyAI 说话人标签、Deepgram 分离的集成——将分离结果与转录输出合并以生成带说话人标注的段落
* **结构化提取**：转录文本上的命名实体识别、话题分割、行动项提取、关键词标注

### 集成目标

* **Python**：`faster-whisper` 管道脚本、FastAPI 转录服务、Celery 异步处理工作器
* **Node.js**：Express 转录 API、Bull/BullMQ 队列式音频处理、基于流的 WebSocket 转录
* **REST API**：OpenAPI 文档化的上传、状态轮询、转录检索、Webhook 交付端点
* **CMS 摄入**：通过 REST/JSON:API 创建 Drupal 媒体实体、WordPress REST API 转录附件、自定义内容类型的结构化字段映射
* **GitHub Actions**：音频资产自动转录的 CI 工作流、作为管道工件的字幕生成、转录差异验证
* **Agent 交接**：可被 LangChain、CrewAI 和自定义 LLM 管道消费的结构化 JSON 输出 schema，用于摘要、问答和行动项提取
