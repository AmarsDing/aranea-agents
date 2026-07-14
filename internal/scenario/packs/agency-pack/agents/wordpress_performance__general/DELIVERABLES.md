## 📋 你的技术交付物
### 性能审计基线

```
WORDPRESS 性能审计基线
───────────────────────────────────────
环境
  WordPress / PHP:      [6.x / PHP 8.x — opcache 开启? JIT?]
  主机类型:            [共享 / VPS / 托管（Kinsta/WP Engine/Pressable）]
  对象缓存:         [无 / Redis / Memcached — 命中?]
  页面缓存:           [插件 / 主机级 / 无]
  CDN:                  [Cloudflare / Fastly / BunnyCDN / 无]

CORE WEB VITALS（移动端，节流 — 基线）
  LCP:                  [__ s]   （目标 < 2.5s）
  INP:                  [__ ms]  （目标 < 200ms）
  CLS:                  [__ ]    （目标 < 0.1）
  Lighthouse 性能:      [__ /100]

数据库（来自 Query Monitor）
  每请求查询数:  [__ count]   总查询时间: [__ ms]
  慢查询:         [前 5 — 来源插件/主题]
  Autoload 大小:        [__ KB/MB autoloaded options]
  无界查询:    [posts_per_page => -1 的元凶]

插件 / 主题开销（每请求）
  最重插件:     [按查询数 + PHP 时间排前]
  页面构建器加载:    [CSS/JS 发运 — KB]

前端
  渲染阻塞:      [阻塞 CSS/JS 计数]
  最大资源:       [按权重排前的脚本/样式/图片]
  图片:               [尺寸化? 懒加载? WebP/AVIF? LCP 图片已识别?]
```

### 缓存架构规范

```
WORDPRESS 缓存架构
───────────────────────────────────────
第 1 层 — 对象缓存（Redis / Memcached）:
  目的:             [在 RAM 中缓存重复 DB 查询 + 计算对象]
  后端:             [Redis / Memcached — 持久化]
  Drop-in:             [object-cache.php 已安装 + 验证命中]
  命中率目标:     [热缓存 > 90%]

第 2 层 — TRANSIENTS:
  用于:            [昂贵 API 调用、聚合、慢查询]
  过期:          [与数据波动性匹配 — 非"永远"]
  后端存储:       [对象缓存（不是负载下的 options 表）]

第 3 层 — 页面缓存（匿名 HTML）:
  后端:             [插件 / 主机 / Varnish]
  旁路规则:        [登录态、购物车、结账、账户 — 排除]
  TTL + 清理:         [发布/更新时 — tag/path 清理]

第 4 层 — CDN / 边缘:
  静态资源:       [长 TTL + 远期过期 + 版本化]
  边缘 HTML:           [仅匿名 — 动态页面旁路]

动态页面安全（在边缘验证）:
  □ 购物车 / 结账 / 账户 永不公开缓存
  □ 登录态响应 永不从匿名缓存提供
  □ Nonce/session 内容不在用户间泄露
```

### 查询与数据库优化计划

```
数据库优化计划
───────────────────────────────────────
慢 / 昂贵查询:   [从 Query Monitor / 慢日志捕获]
  来源:              [哪个插件 / 主题 / WP_Query]
  当前开销:        [__ ms, __ 行扫描]
  原因:               [无界 / 无索引 meta_query / N+1 / no_found_rows]

修复:
  □ 限定它（设置 posts_per_page；面向用户处绝不用 -1）
  □ 不分页时 no_found_rows => true
  □ 索引被过滤或排序的 meta/tax 列
  □ 不需要完整 post 对象时 fields => 'ids'
  □ 用一个查询替换每循环查询（杀 N+1）
  □ 把昂贵结果包裹在 transient 中（对象缓存支撑）

AUTOLOAD 卫生:
  Autoload 大小:        [前: __ KB → 后: __ KB]
  □ 大的未缓存 options 切到 autoload = no
  □ 孤儿/废弃插件 options 移除

验证:
  每请求查询数:  [前: __ → 后: __]
  查询时间:       [前: __ ms → 后: __ ms]   （测量）
```

### 前端与图片优化规范

```
前端交付优化
───────────────────────────────────────
资源优化:
  CSS:                 [压缩 + 合并; 关键 CSS 内联]
  JS:                  [压缩; 非关键延迟; 验证工作]
  Dequeuing:           [插件资源在页面未用处移除]
  字体:               [font-display: swap + 预加载关键字体]

渲染阻塞减少:
  □ 非关键 CSS 延迟 / 异步加载
  □ 非关键 JS 延迟（jQuery 依赖验证完好）
  □ 不使用页面构建器臃肿的页面 dequeue
  □ 第三方脚本门控（分析 / 聊天 / 像素）

图片（每张图片，无例外）:
  交付:            [正确尺寸派生 — srcset/sizes]
  格式:              [WebP / AVIF 带 fallback]
  尺寸:          [显式 width/height — 防 CLS]
  加载:             [首屏下 loading="lazy"]
  LCP 图片:           [预加载 + eager — 绝不懒加载]

验证（移动端，节流）:
  □ 页面渲染 + 压缩后每个交互元素工作
  □ CLS 不变或改善（无无尺寸图片）
  □ LCP 元素已识别并优先化
```

### 基础设施调优清单

```
基础设施性能调优
───────────────────────────────────────
PHP OPCACHE:
  opcache.enable:               [1]
  opcache.memory_consumption:   [128–256 MB 按代码库调整]
  opcache.max_accelerated_files:[提高以覆盖 WP 核心 + 插件]
  opcache.validate_timestamps:  [生产 0 — 部署时清理]
  opcache.jit:                  [已评估 — 测量，非假设]

PHP-FPM:
  pm:                           [dynamic / static — 按 RAM 调整]
  pm.max_children:              [RAM ÷ 平均进程大小]
  慢日志:                     [开启 — 捕获慢请求]

对象缓存后端:
  后端:                      [Redis / Memcached — 持久化]
  Drop-in 激活:               [object-cache.php — 验证命中]
  淘汰策略:              [allkeys-lru 或适当尺寸]

CDN / 边缘:
  静态资源缓存:         [长 TTL + 远期过期]
  动态旁路:               [购物车/结账/账户/登录态 — 验证]
  压缩:                  [边缘 Brotli / gzip]

验证:
  □ 对象缓存命中率已测量（不是假设已安装）
  □ 无私有/登录态响应在边缘被公开缓存
```

---
