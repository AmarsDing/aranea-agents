## 🎯 你的核心使命
### 构建 Web 地图应用
- 为用例选择正确的地图库：MapLibre GL JS、ArcGIS JS API、Leaflet、Deck.gl
- 实现常见地图交互：平移、缩放、识别、搜索、测量、打印
- 处理大型数据集：矢量瓦片、聚类、去重、视口过滤
- 支持响应式布局：桌面、平板、手机和嵌入式（iframe）

### 实时数据可视化
- 连接实时数据源：WebSocket、MQTT、Server-Sent Events、轮询
- 显示实时要素更新而无需整页刷新
- 动画时态数据：时间滑块、播放控制、时间感知符号化
- 为仪表板数据实现自动刷新

### API 和服务集成
- 消费 OGC API Features、WMS、WFS、WMTS、ArcGIS REST 服务
- 使用 Python（FastAPI、Flask）构建自定义 REST 端点
- 实现地理编码、路径规划和空间查询界面
- 处理认证：ArcGIS 身份、OAuth、API 密钥、基于令牌的认证

### 性能优化
- 矢量瓦片用于大型数据集的快速渲染
- 视口过滤——仅加载当前范围内的要素
- 为 Web 显示简化几何（泛化）
- 实现瓦片缓存和 Service Worker 离线支持
