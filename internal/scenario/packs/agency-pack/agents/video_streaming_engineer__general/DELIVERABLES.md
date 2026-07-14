## 📋 你的技术交付物
### ffmpeg 转码阶梯 → CMAF（打包一次）

```bash
# 编码多阶梯梯子，关键帧（GOP）对齐，以便 ABR 能在分片边界干净切换。
# 关键帧间隔 = 分片时长 * fps。
ffmpeg -i source.mov \
  -filter_complex "[0:v]split=4[v1][v2][v3][v4]; \
    [v1]scale=w=640:h=360[v360]; [v2]scale=w=1280:h=720[v720]; \
    [v3]scale=w=1920:h=1080[v1080]; [v4]scale=w=2560:h=1440[v1440]" \
  -map "[v360]"  -c:v:0 libx264 -b:v:0 800k   -maxrate:0 856k   -bufsize:0 1200k \
  -map "[v720]"  -c:v:1 libx264 -b:v:1 2800k  -maxrate:1 2996k  -bufsize:1 4200k \
  -map "[v1080]" -c:v:2 libx264 -b:v:2 5000k  -maxrate:2 5350k  -bufsize:2 7500k \
  -map "[v1440]" -c:v:3 libx264 -b:v:3 8000k  -maxrate:3 8560k  -bufsize:3 12000k \
  -x264-params "keyint=48:min-keyint=48:scenecut=0" \  # 闭合 GOP，2s @ 24fps，跨阶梯对齐
  -map a:0 -c:a aac -b:a 128k \
  -f null -   # （真实管道传输到 CMAF 打包器；关键帧对齐是这里的要点）

# 将编码后的版本一次性打包为 CMAF，同时输出 HLS + DASH 清单：
packager \
  in=v360.mp4,stream=video,init_segment=v360/init.mp4,segment_template='v360/$Number$.m4s' \
  in=v720.mp4,stream=video,init_segment=v720/init.mp4,segment_template='v720/$Number$.m4s' \
  in=audio.mp4,stream=audio,init_segment=a/init.mp4,segment_template='a/$Number$.m4s' \
  --hls_master_playlist_output master.m3u8 \
  --mpd_output manifest.mpd \
  --segment_duration 2
```

### 码率阶梯设计（per-title 优于一刀切）

| 阶梯 | 分辨率 | 码率 | 角色 |
|------|-----------|---------|------|
| 1 | 640×360 | ~0.8 Mbps | 启动阶梯 + 拥挤网络底线（快速首帧）|
| 2 | 1280×720 | ~2.8 Mbps | 主力——大多数会话在移动/Wi-Fi 上位于此 |
| 3 | 1920×1080 | ~5.0 Mbps | 良好宽带的默认选择 |
| 4 | 2560×1440 | ~8.0 Mbps | 强连接上的大屏幕 |

规则：阶梯间隔约 1.5–2 倍（太近浪费存储并混淆 ABR；太远导致突兀的质量跳变）。per-title 分析会移动这些值——卡通或幻灯片需要的比特远少于充满雪的滑雪场景，以获得相同的感知质量。仅在受众设备和网络能够使用时添加阶梯。

### 延迟等级决策表

| 用例 | 分片/分块 | 协议 | 目标延迟 | 接受的权衡 |
|----------|--------------|----------|----------------|-------------------|
| VOD | 4–6s 分片 | HLS/DASH | 启动优化，延迟无关 | 最佳缓存效率，最低成本交付 |
| 标准直播 | 2–4s 分片 | HLS/DASH | 15–30s 端到端 | 简单、稳健、缓存友好 |
| 低延迟直播 | CMAF 分块（~0.2–0.5s）在 2s 分片中 | LL-HLS / LL-DASH | 2–6s | 更多请求、更严格调优、更高成本 |
| 实时/互动 | 亚秒级 | WebRTC | < 1s | 完全不同的技术栈；ABR + 规模更难 |

### 真正重要的 QoE 指标

```text
按会话逐分片追踪——这些预测参与度，而非分辨率：
  · 首帧时间（启动延迟）   → 目标 < 1s；这是门口的流失
  · 卡顿比率（停滞时间 / 观看时间） → 目标 < 0.5%；#1 流失驱动因素
  · 播放失败率（从未开始）     → 通常是 DRM、清单或编解码器支持 bug
  · 平均交付码率 + 切换频率 → 质量不过度振荡
  · 视频开始前退出率          → 启动路径太慢或损坏
按最差网络群体告警，而非平均值——平均值隐藏了你正在失去的用户。
```
