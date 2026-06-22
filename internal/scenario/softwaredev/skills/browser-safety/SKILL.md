# Skill: 浏览器安全导航

## 定位
浏览器自动化工具（browser_navigate / browser_click / browser_snapshot 等）的安全使用规范。覆盖 SSRF 防护边界、URL 选择策略、隐私数据处理、结果体积控制。在使用任何 `browser_*` 工具前必须遵循本规范。

## 核心规则

### 1. SSRF 防护边界（强制）
- `browser_navigate` 受服务端 NavigationPolicy 强制校验，以下 URL 会被拒绝：
  - 回环地址：`localhost` / `127.0.0.0/8` / `::1`（除非 AllowLoopback=true）
  - 私网地址：`10.0.0.0/8` / `172.16.0.0/12` / `192.168.0.0/16`（除非 AllowPrivateNet=true）
  - `file://` 协议（除非 AllowFileURLs=true）
  - 非 http/https/about:blank 协议（ftp/javascript 等一律拒绝）
  - BlockedDomains 中的域名（含子域名）
  - AllowedDomains 非空时，仅允许列表内域名（含子域名）
- 被拒绝的导航会返回 `BadRequest` 错误，错误消息包含被拦截的主机名
- **不要尝试绕过**：编码、大小写混淆、DNS rebinding 均已在防护层覆盖

### 2. URL 选择策略
- 优先使用 HTTPS：HTTP URL 在传输中可被篡改，仅用于本地调试
- 不要导航到用户提供的原始 URL 而不校验：先确认域名可信
- `about:blank` 用于初始化页面或清空状态，是安全操作
- 短链接（t.co / bit.ly）需先解析目标：导航前确认最终域名
- 不要构造 URL 查询参数注入：用户输入必须 URL-encode
- 分页导航优先用 URL 参数（`?page=2`），不要依赖 JS 点击下一页

### 3. 工具选择决策树
- 仅需获取页面文本 → 优先 `web_fetch`（更轻量、无浏览器开销）
- 需要 JS 渲染后的内容 → `browser_navigate` + `browser_snapshot`
- 需要交互（点击、输入、提交） → `browser_navigate` + `browser_click` / `browser_type`
- 需要视觉信息（布局、颜色、截图） → `browser_take_screenshot`
- 需要提取结构化数据 → `browser_snapshot`（返回可访问性树）优于 `browser_take_screenshot`
- 截图仅在需要视觉判断时使用：截图体积大、无法被 LLM 文本解析

### 4. 隐私与敏感数据
- 截图可能包含用户敏感信息（邮箱、手机号、地址、凭证）：不要将截图内容回传到对话
- 导航到登录页时不要自动填充凭证：让用户提供或通过环境变量注入
- 不要导航到内部管理后台（如 `admin.example.com`）除非用户明确要求
- Cookie / localStorage 中的 token 不应在对话中暴露
- 截图前检查页面是否包含敏感面板（如支付信息、个人资料）：如有，改用 `browser_snapshot` 并只提取需要的字段

### 5. 结果体积控制
- `browser_snapshot` 返回可访问性树，复杂页面可达数十 KB：聚焦提取需要的节点
- `browser_take_screenshot` 返回 base64 图片，极易超过上下文预算：仅在必要时使用
- 不要连续多次截图：一次截图后分析，避免重复
- 长页面不要整页截图：用 `browser_click` 滚动到目标区域后再截图
- 导航后等待页面加载完成再 snapshot：避免拿到中间状态

### 6. 反爬与合规
- 遵守目标站点 `robots.txt`：导航前检查是否允许爬取
- 控制请求频率：连续导航间隔 ≥ 2 秒，避免触发限流或封禁
- 不要伪造 Referer / User-Agent 绕过访问控制
- 不要抓取需要付费墙的内容：尊重版权和订阅协议
- 公开数据可抓取，但不要大规模复制受版权保护的内容

## 检查清单
- [ ] 导航前确认目标 URL 使用 HTTPS
- [ ] 确认目标域名不在 BlockedDomains 内
- [ ] 确认目标域名在 AllowedDomains 内（若配置了白名单）
- [ ] 不导航到私网/回环地址（除非明确需要本地调试）
- [ ] 截图前检查页面无敏感信息
- [ ] 不在对话中回传 Cookie / token / 凭证
- [ ] 优先使用 web_fetch 而非 browser_navigate（当仅需文本时）
- [ ] 优先使用 browser_snapshot 而非 browser_take_screenshot（当仅需结构化数据时）
