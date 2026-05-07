# Channel 管理 — 设计文档

本文档描述 Channel 的**录入界面设计**（含各平台独立添加页）、**控件与参数清单**，以及按照 **aranea 平台资源架构**落地的**存储与运行时使用设计**。

本次设计目标：支持 **20+ Channel** 的统一配置、启停、凭据管理与运行时加载；前端沿用 ClawPanel 现有「通道配置页 + 插件 catalog + schema 渲染」思路，后端不再以散落的 `openclaw.json.channels.<id>` 作为唯一存储，而是采用 aranea 中 `PlatformResource` 风格的 `channel` 表、`config_json`、`metadata_json`、`channel_credential` 和 `channel_delivery` 组合。

---

## 1. 总体信息架构

| 页面 | 路由建议 | 说明 |
|------|----------|------|
| Channel 列表 | `/channels` | 见下「列表页」 |
| 新增 Channel（向导） | `/channels/new` | 两步：① 选择平台 ② 填写配置 |
| 编辑 Channel | `/channels/:id/edit` | 见下「编辑页」；不支持更换平台 `type` |
| 各平台专属表单 | 新增第二步 / 编辑页，按平台 `type` 动态渲染区块 | 同一套字段组件复用 |

**新增流程**：列表点击「新增 Channel」→ 步骤一选择平台 → 步骤二填写「通用信息 + 当前平台专属参数」→ 保存 / 保存并测试 → 返回列表或留在详情。

### 1.1 Channel 列表页

| 区域 | 内容 |
|------|------|
| 顶栏 | 标题「Channel 管理」；主按钮「新增 Channel」 |
| 筛选 | `Select`：平台类型（全部 / 各 `config_json.type`）；`Select`：状态（全部 / 启用 / 停用 / 连接异常等，与 `enabled`+`status` 映射一致，见第 6 节） |
| 搜索 | `Input.Search`：按 `name`、可选 `channel_key` 短片段 |
| 空态 | 插图 + 文案「尚未接入渠道」+ 引导「新增 Channel」 |

#### 1.1.1 添加完成后的列表展示（表格列与数据来源）

用户保存成功并返回列表后，每一行展示下列信息（**默认列顺序**；过宽时可隐藏次要列或收进「更多」）。

| 列名（表头） | 展示内容 | 数据来源 / 说明 |
|--------------|----------|-----------------|
| **名称** | 主文案为 `name`；副文案可选展示 `channel_key` 便于区分 | `channel.name`、`channel.channel_key` |
| **平台** | 平台图标 + 文案：如「飞书/Lark」「微信-公众号」等 | **图标**：有 `metadata_json.icon_url` 则 `<img src>`；否则内置资源映射 `config_json.type` + `variant`；文案同左 |
| **外部 ID**  | 简短展示 `metadata_json.external_id` 或 `config_json.config` 内主键（如 `app_id`、`page_id`）截断 | 便于运营对照第三方后台 |
| **启用** | `Switch` **或** 标签「启用 / 停用」 | `channel.enabled`；与第 6 节「`enabled` 与 `status` 分工」一致 |
| **连接状态** | 彩色标签：`正常` / `待配置` / `异常` 等 | 由 `channel.status`（及可选 `last_error_message` 是否有值）映射 |
| **最近错误** | 单行省略，`Tooltip` 显示全文 | `metadata_json.last_error_message` 截断（如 40 字）；无则显示「—」 |
| **最近更新** | 相对时间或 `YYYY-MM-DD HH:mm` | `updated_at` |
| **操作** | 见下「1.1.2」 | 固定右列，不换行 |

**交互**：点击**名称**（整格可点）跳转 `/channels/:id/edit`（与「编辑」等价）；若产品有详情页，可改为先进详情再点「编辑」。

#### 1.1.2 操作列：编辑、删除及其他

操作列建议使用 **文字按钮** 或 **图标按钮**（带 `Tooltip`），同一行内横向排列，顺序如下。

| 操作 | 控件 | 行为 |
|------|------|------|
| **编辑** | `Button` type="link" 或笔形图标 | 跳转 `/channels/:id/edit`；无权限时 `disabled` 或隐藏（见第 2.5 节） |
| **删除** | `Button` danger 或垃圾桶图标 | 点击弹出 `Modal.confirm`：标题「确认删除该 Channel？」；正文提示「删除后无法恢复，第三方 Webhook 需自行解绑」；确认后 `DELETE`，成功 Toast 并刷新列表 |
| **复制 Webhook** | `Button` link | 复制完整回调 URL 到剪贴板；无 Webhook 的平台可隐藏 |

**与「启用」的关系**：若表格中已有「启用」`Switch`，操作列**不必**再重复「停用」按钮，避免重复；若启用放在操作列，则操作列为：`启用/停用`（或下拉）｜`编辑`｜`删除`。

**权限**：无编辑权限的用户仅可查看列表，操作列不展示编辑/删除或整体 `disabled`；删除建议仅管理员可见。

### 1.2 编辑页（与新增页差异）

| 项 | 说明 |
|------|------|
| 路由 | `/channels/:id/edit`；无步骤一，直接进入表单 |
| 平台类型 | **只读展示**（图标 + 文案），不可从微信改为飞书等 |
| 密钥类字段 | `Input.Password` 留空表示**不修改**已有凭据；展示「已配置」+「重置密钥」可选 |
| 只读 URL | Webhook / 回调 URL 与新增一致，依赖已有 `id` 与 `config_json.webhook.path` |
| 并发 | 保存时若后端支持乐观锁，提交 `updated_at` 或 `version`（见第 6 节）不一致则提示「数据已被他人修改，请刷新」 |
| 底部操作 | 与第 5.7 节一致：取消、保存、保存并测试连接 |

---

## 2. 全局布局与通用 UI 规范

### 2.1 页面框架

- **顶栏**：面包屑 `Channel 管理 / 新增`；右侧可选「帮助文档」链接。
- **主体宽度**：建议最大宽度 `960px`～`1200px` 居中，表单单列为主，避免过宽导致视线跳跃。
- **步骤指示器（Steps）**：水平排列两步。
  - 步骤 1：`选择平台`（未完成 / 完成打勾）
  - 步骤 2：`填写配置`（当前步骤高亮）
- **底部操作栏**：吸底或表单末尾固定区域，左对齐次要按钮，右对齐主按钮。

### 2.2 通用组件行为

| 组件 | 行为说明 |
|------|----------|
| 文本输入 | 必填项标签带红色 `*`；失焦校验；错误信息置于输入框下方 |
| 密码类输入 | 默认掩码；右侧「眼睛」图标切换明文；保存成功后清空输入框，显示「已配置」状态 |
| 只读展示框 | 系统生成的 Webhook URL、路径等：只读 `Input` +「复制」按钮；复制成功 Toast |
| 开关 | 启用 Channel：`Switch`，默认开启 |
| 单选卡片 | 平台选择：大卡片 + 图标 + 标题 + 一行描述 |
| 折叠面板 | 「高级设置」「Webhook 订阅字段」等次要配置：`Collapse` 默认收起 |
| 表单分组 | 使用 `Card` 或 `Divider + 标题` 区分「通用信息」「平台凭证」「回调地址」 |

### 2.3 状态与反馈

- **加载**：提交、测试连接时主按钮 `loading`，表单整体 `disabled`。
- **成功**：Toast「保存成功」；若需用户在第三方后台粘贴 URL，用 `Alert` 提示下一步操作。
- **失败**：顶部 `Alert` 展示服务端错误摘要；字段级错误保留在对应项下方。

### 2.4 草稿与离开确认

| 场景 | 建议行为 |
|------|----------|
| 步骤二点击「上一步」 | 若已填写内容，`Modal.confirm`：「返回将清空当前平台已填内容」；确认后清空步骤二本地状态并回到步骤一 |
| 点击「取消」或路由离开 | 若有未提交变更，`Modal.confirm`：「未保存的更改将丢失」 |
| 刷新/关闭浏览器 | 依赖 `beforeunload` 提示（可选）；或本地草稿（`sessionStorage` 暂存平台 `type` + 非敏感字段，**密钥不落本地**） |

### 2.5 安全、权限与合规

| 项 | 说明 |
|------|------|
| 角色权限 | 建议至少区分：**管理员**可创建/编辑/删除、查看完整 Webhook URL；**运营**仅可查看 Channel 名称与状态、不可查看/复制密钥；实现以实际 RBAC 为准 |
| 密钥展示 | API 永不返回明文 Secret；仅 `******` 或「已配置」+ 可选重置流程 |
| 审计 | 对凭据变更、删除 Channel、启用/停用写入系统审计日志；若需要独立表，可扩展 `channel_audit_log` |
| Webhook 日志 | 若存 `channel_delivery` 或原始 body：**不存明文消息内容**或按合规要求脱敏与设置保留周期（如 7 天），可仅存 `event_id`、状态码、耗时 |

---

## 3. 步骤一：选择平台 — 界面设计（详细）

### 3.1 布局结构

```
┌─────────────────────────────────────────────────────────────┐
│  面包屑: Channel 管理 / 新增                                   │
│  标题: 新增 Channel                                            │
│  [ ● 选择平台 ] ─── [ 2 填写配置 ]                             │
├─────────────────────────────────────────────────────────────┤
│  搜索: [ 🔍 搜索平台名称________________ ]                     │
│  分组 Tab: [ 全部 ] [ 国内 ] [ 海外 ] [ 办公协作 ]              │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ [图标]   │ │ [图标]   │ │ [图标]   │ │ [图标]   │          │
│  │ 飞书/Lark│ │ 微信     │ │ QQ       │ │ Facebook │          │
│  │ 企业协作 │ │ 消息触达 │ │ 机器人   │ │ Messenger│          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
│  ... 更多卡片                                                 │
├─────────────────────────────────────────────────────────────┤
│                        [ 取消 ]  [ 下一步 → ]                 │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 控件清单

| 控件 | 类型 | 参数/绑定 | 说明 |
|------|------|-----------|------|
| 搜索框 | `Input.Search` | `v-model: keyword`，过滤卡片标题 | 过滤「飞书」「微信」等关键词 |
| 分组 Tab | `Tabs` | `filter: all \| domestic \| overseas \| office` | 国内：微信、QQ；海外：Facebook、Telegram、WhatsApp；办公：飞书 |
| 平台卡片 | 自定义卡片 | 每项：平台 `type`、`icon`、`title`、`description` | 单选；选中态：边框高亮 + 浅底 |
| 取消 | `Button` | `secondary` | 返回列表，若有未保存草稿可 `Modal.confirm` |
| 下一步 | `Button` | `primary`，`:disabled="!selectedType"` | 进入步骤二并带上平台 `type` |

### 3.3 平台卡片数据（展示用，20+）

卡片数据优先由后端 `GET /api/channels/catalog` 返回；前端只保留兜底图标和排序分组。ClawPanel 现有实现里 catalog 会从插件 manifest 的 `channels`、`channelEnvVars`、`configSchema`、`uiHints` 读取，并补充手工维护的内置/虚拟通道。本项目建议沿用该思路，但 catalog 只描述「可配置能力」，真实配置落到第 6 节的 `channel` 表。

| channel_key | 标题 | 分组 | 接入形态 | 副标题/描述 |
|-------------|------|------|----------|-------------|
| `qq` | QQ (NapCat) | 国内 | plugin / websocket | QQ 个人号，NapCat OneBot11 协议 |
| `qqbot` | QQ 官方机器人 | 国内 | plugin / webhook | QQ 开放平台机器人 |
| `feishu` | 飞书 / Lark | 办公协作 | plugin / webhook | 事件订阅、机器人消息、多账号 |
| `dingtalk` | 钉钉 | 办公协作 | plugin / webhook | 钉钉机器人与事件回调 |
| `wecom` | 企业微信智能机器人 | 办公协作 | plugin / webhook | 群机器人或智能机器人 |
| `wecom-app` | 企业微信自建应用 | 办公协作 | virtual / webhook | 写入 `channels.wecom.agent` 兼容配置 |
| `openclaw-weixin` | 微信（ClawBot） | 国内 | plugin / qrcode | 腾讯官方 WeChat ClawBot |
| `wechat` | 微信开放平台 | 国内 | builtin / webhook | 公众号、小程序、企业微信（步骤二再选子类型） |
| `telegram` | Telegram | 海外 | plugin / webhook 或 polling | Bot API 接入 |
| `whatsapp` | WhatsApp | 海外 | plugin / webhook | Meta Cloud API 或 BSP |
| `facebook` | Facebook Messenger | 海外 | plugin / webhook | Page Webhook 与 Messenger |
| `discord` | Discord | 海外 | plugin / gateway | Bot Gateway / Interaction |
| `slack` | Slack | 办公协作 | plugin / event | Slack App Events / Bot |
| `msteams` | Microsoft Teams | 办公协作 | plugin / webhook | Teams Bot / Graph |
| `googlechat` | Google Chat | 办公协作 | plugin / webhook | Google Chat App |
| `line` | LINE | 海外 | plugin / webhook | LINE Messaging API |
| `matrix` | Matrix | 海外 | plugin / sync | Matrix Bot |
| `mattermost` | Mattermost | 办公协作 | plugin / websocket | Mattermost Bot |
| `signal` | Signal | 海外 | plugin / daemon | Signal CLI/daemon |
| `zalo` | Zalo | 海外 | plugin / webhook | Zalo Official Account |
| `zalouser` | Zalo User | 海外 | builtin / polling | Zalo 用户侧通道 |
| `imessage` | iMessage | 海外 | plugin / bridge | BlueBubbles / macOS 桥接 |
| `bluebubbles` | BlueBubbles | 海外 | plugin / bridge | iMessage Android/Server 桥接 |
| `nextcloud-talk` | Nextcloud Talk | 办公协作 | plugin / polling | Nextcloud Talk Bot |
| `synology-chat` | Synology Chat | 办公协作 | plugin / webhook | Synology Chat Bot |
| `irc` | IRC | 海外 | plugin / socket | IRC Bot |
| `nostr` | Nostr | 海外 | plugin / relay | Nostr relay 订阅 |
| `twitch` | Twitch | 海外 | plugin / eventsub | Twitch Chat / EventSub |
| `tlon` | Tlon | 海外 | plugin | Tlon / Urbit 集成 |
| `voice-call` | Voice Call | 语音 | builtin / webhook | Twilio / Telnyx / Plivo / mock |
| `qa-channel` | QA Channel | 测试 | plugin | QA 与回归测试通道 |

> 若产品将微信拆为三步，也可在步骤一直接拆成三张卡片（公众号 / 小程序 / 企微），此时步骤二不再出现「微信子类型」选择。建议首版 UI 对 20+ 平台启用搜索、分组与「常用」置顶，避免首屏卡片过载。

---

## 4. 步骤二：填写配置 — 通用区块（所有平台）

### 4.1 布局顺序（自上而下）

1. **页面标题区**：显示已选平台图标 + 名称；`Button`「上一步」回到步骤一（可清空当前平台专属草稿，需确认）。
2. **Card：通用信息**
3. **Card：平台凭证**（各平台字段不同，见第 5 节）
4. **Card：回调与集成**（Webhook URL 等只读项，部分平台才有）
5. **底部**：`取消` | `保存` | `保存并测试连接`

### 4.2 通用信息 — 控件与参数

| 字段名（逻辑） | 控件 | 必填 | 校验规则 | 占位符示例 |
|----------------|------|------|----------|------------|
| `name` | `Input` | 是 | 1～128 字符，同租户内建议唯一提示 | `例如：官网客服-飞书` |
| `description` | `Input.TextArea` | 否 | 最大 512 字符 | `业务用途说明` |
| `icon` | `Upload` + 预览 或 URL `Input` | 否 | 写入 `metadata_json.icon_url`；未上传则使用内置平台图标，见 **§6.3** | — |
| `enabled` | `Switch` | 否 | 默认 `true` | 启用后才会接收消息 |
| `wechat_subtype` | `Radio.Group` 或 `Select` | 仅微信 | 三选一 | 见 5.2 节 |

---

## 5. 各 Channel 添加界面 — 专属控件与参数

以下均为**步骤二**中「平台凭证」「回调与集成」区块的内容；与第 4 节「通用信息」叠加。

### 5.1 飞书 / Lark（`feishu`）

**分组 A：应用凭证**

| 参数（逻辑名） | 控件 | 必填 | 说明 |
|----------------|------|------|------|
| `app_id` | `Input` | 是 | 飞书开放平台应用 App ID |
| `app_secret` | `Input.Password` | 是（新建） | 应用 Secret；编辑时可为空表示不修改 |
| `encrypt_key` | `Input.Password` | 视加密方式 | 事件订阅加密密钥，文档称 Encrypt Key |
| `verification_token` | `Input` | 视订阅方式 | 事件订阅 Verification Token |

**分组 B：回调与集成**

| 参数 | 控件 | 说明 |
|------|------|------|
| `webhook_url` | `Input` 只读 + `Button` 复制 | 后端根据 `channel_id` 生成完整 HTTPS URL，供飞书后台「请求地址」填写 |
| `event_subscribe_hint` | `Alert` info | 文案提示需在开放平台勾选事件列表、订阅方式 |

**校验**：`app_id` 非空；新建时 `app_secret` 必填。

---

### 5.2 微信（`wechat`）

**子类型选择（若步骤一未拆分）**：`Radio` — `公众号` | `小程序` | `企业微信`，对应 `wechat_subtype`。

#### 5.2.1 公众号（`official`）

| 参数 | 控件 | 必填 |
|------|------|------|
| `app_id` | `Input` | 是 |
| `app_secret` | `Input.Password` | 是（新建） |
| `token` | `Input` | 是 | 服务器配置中的 Token |
| `encoding_aes_key` | `Input.Password` | 安全模式下必填 | 43 位 EncodingAESKey |

| 参数 | 控件 | 说明 |
|------|------|------|
| `server_url` | 只读 + 复制 | 微信公众平台服务器 URL |
| `server_path_hint` | `Text` | 若路径带 `channel_id` 需说明 |

#### 5.2.2 小程序（`miniprogram`）

| 参数 | 控件 | 必填 |
|------|------|------|
| `app_id` | `Input` | 是 |
| `app_secret` | `Input.Password` | 是（新建） |

可选：`message_push_url` 只读（若走云开发/客服消息，按实际产品补字段）。

#### 5.2.3 企业微信（`wechat_work`）

| 参数 | 控件 | 必填 |
|------|------|------|
| `corp_id` | `Input` | 是 |
| `agent_id` | `Input` | 是 |
| `secret` | `Input.Password` | 是（新建） |

可选：`token`、`encoding_aes_key`（接收消息回调时与公众号类似）。

---

### 5.3 QQ（`qq`）

**分组：机器人凭证**

| 参数 | 控件 | 必填 | 说明 |
|------|------|------|------|
| `app_id` | `Input` | 是 | QQ 开放平台应用/机器人 AppID |
| `client_secret` 或 `bot_token` | `Input.Password` | 是（新建） | 以实际 QQ 机器人文档字段名为准 |

**分组：回调**

| 参数 | 控件 | 说明 |
|------|------|------|
| `callback_url` | 只读 + 复制 | 供 QQ 后台配置的回调地址 |

---

### 5.4 Facebook（`facebook`）

**分组：应用与主页**

| 参数 | 控件 | 必填 |
|------|------|------|
| `page_id` | `Input` | 是 |
| `page_access_token` | `Input.Password` | 是（新建） |
| `app_secret` | `Input.Password` | 是（新建） | 用于 Webhook 签名校验 |

**分组：Webhook**

| 参数 | 控件 | 必填 |
|------|------|------|
| `verify_token` | `Input` | 是 | 与 Meta 后台 Webhook 校验一致 |

**高级（Collapse）**

| 参数 | 控件 | 说明 |
|------|------|------|
| `subscribed_fields` | `Checkbox.Group` | 可选：`messages`、`messaging_postbacks`、`message_deliveries` 等 |

| 参数 | 控件 | 说明 |
|------|------|------|
| `callback_url` | 只读 + 复制 | Meta 开发者后台 Webhook 回调 URL |

---

### 5.5 Telegram（`telegram`）

**分组：Bot**

| 参数 | 控件 | 必填 |
|------|------|------|
| `bot_token` | `Input.Password` | 是（新建） | 由 @BotFather 获取 |

**分组：接入方式**

| 参数 | 控件 | 说明 |
|------|------|------|
| `receive_mode` | `Radio` | `webhook` \| `long_polling` | 二选一 |

**当 `receive_mode === webhook`**

| 参数 | 控件 | 说明 |
|------|------|------|
| `webhook_url` | 只读 + 复制 | `setWebhook` 填写的 HTTPS URL，由系统生成 |

**当 `long_polling`**

| 参数 | 控件 | 说明 |
|------|------|------|
| `polling_hint` | `Alert` | 说明由服务端进程拉取，无需填 URL |

---

### 5.6 WhatsApp（`whatsapp`）

**分组：Meta Cloud API（默认）**

| 参数 | 控件 | 必填 |
|------|------|------|
| `waba_id` | `Input` | 是 | WhatsApp Business Account ID |
| `phone_number_id` | `Input` | 是 | 发送号码 ID |
| `access_token` | `Input.Password` | 是（新建） | 长期或短期 Token，产品内需说明轮换方式 |

| 参数 | 控件 | 说明 |
|------|------|------|
| `verify_token` | `Input` | Webhook 验证用，与 Meta 配置一致 |

| 参数 | 控件 | 说明 |
|------|------|------|
| `callback_url` | 只读 + 复制 | Meta Webhook 回调地址 |

**可选分组：BSP / 第三方**

| 参数 | 控件 | 说明 |
|------|------|------|
| `provider` | `Select` | `meta_cloud` \| `other` |
| `provider_api_key` | `Input.Password` | 选第三方时显示 |

---

### 5.7 底部操作（所有平台）

| 控件 | 类型 | 行为 |
|------|------|------|
| 取消 | `Button` | 返回列表或步骤一，`Modal` 确认未保存 |
| 保存 | `Button` primary | 校验 → POST/PUT API → Toast |
| 保存并测试连接 | `Button` | 先保存再调「测试连接」接口，结果用 `Alert` 或 `Modal` 展示 |

---

## 6. 存储设计（按 aranea 架构）

### 6.1 设计原则

Channel 配置采用 aranea 的平台资源模型：主数据是 `PlatformResource`，用统一的 `channel` 表承载；不同平台差异不扩列，而进入 `config_json` 和 `metadata_json`；敏感凭据不进 `config_json`，只在 `channel_credential.secret_ref` 中保存密钥引用。这样 20+ Channel 不需要 20+ 张表，也不需要为每个平台不断加列。

| 设计点 | 说明 |
|--------|------|
| 主表统一 | `channel` 与 aranea 的 `PlatformResource` 字段对齐：`id`、`channel_key`、`name`、`status`、`enabled`、`sort_order`、`config_json`、`metadata_json` |
| 差异 JSON 化 | 平台参数、Webhook 路径、账号列表、订阅字段、运行模式放 `config_json` |
| 展示/诊断元数据 | 图标、分组、catalog 来源、最近错误、最近连接时间放 `metadata_json` |
| 密钥引用化 | `app_secret`、`bot_token`、`access_token`、`encodingAESKey` 等写入 `channel_credential.secret_ref` |
| 运行时只读视图 | Runtime 通过 repository/service 读取 enabled channels，合并 config 与 credential ref 后注入 channel adapter/plugin |
| 软删除 | 使用 `deleted_at`，删除后保留审计与历史 delivery |

### 6.2 表：`channel`

对齐 aranea 现有迁移中的 `channel` 表，建议作为本项目首选落库结构：

```sql
CREATE TABLE IF NOT EXISTS channel (
  id TEXT PRIMARY KEY,
  channel_key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT ''
);
```

| 字段 | UI / 业务含义 |
|------|---------------|
| `id` | 内部 ID，建议 `ch_` 前缀或 UUID；前端路由可使用该值 |
| `channel_key` | 平台/实例键，单账号可为 `telegram`，多实例建议 `telegram_support`、`feishu_sales` |
| `name` | 展示名称，如「官网客服 Telegram」 |
| `description` | 备注 |
| `status` | 授权/连接状态：`draft`、`pending_auth`、`active`、`error`、`archived` |
| `enabled` | 人为启停开关；关闭后不再接收或投递消息 |
| `sort_order` | 列表和常用平台排序 |
| `config_json` | 非敏感运行配置，必须是合法 JSON 字符串 |
| `metadata_json` | UI、诊断和 catalog 元信息，必须是合法 JSON 字符串 |
| `deleted_at` | 软删除；空字符串表示未删除 |

`enabled` 与 `status` 分工保持不变：`enabled=false` 表示人为停用；`status=error` 表示凭据失效、Webhook 验签失败、运行时连接失败等。列表筛选时「停用」来自 `enabled`，「异常」来自 `status`。

### 6.3 `config_json` 结构约定

所有 Channel 使用同一外层结构，平台差异放在 `config` 内。`channel_key` 是实例键，`type` 是平台类型；这样可以支持同平台多实例，也能兼容 ClawPanel 当前以 `channels.<id>` 作为单实例配置的写法。

```json
{
  "type": "telegram",
  "receive_mode": "webhook",
  "webhook": {
    "path": "/webhooks/ch_telegram_support",
    "verify_mode": "telegram_secret_token"
  },
  "routing": {
    "default_agent_id": "main",
    "dm_scope": "per-channel-peer"
  },
  "config": {
    "bot_username": "support_bot",
    "allowed_updates": ["message", "callback_query"]
  },
  "accounts": []
}
```

推荐字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 平台类型，如 `qq`、`feishu`、`telegram`、`whatsapp` |
| `variant` | string | 可选，插件变体，如飞书 `official` / `community`、企业微信 `bot` / `app` |
| `receive_mode` | string | `webhook`、`websocket`、`polling`、`gateway`、`bridge`、`qrcode` |
| `webhook.path` | string | 系统生成的回调路径，不含域名 |
| `webhook.verify_mode` | string | 平台验签策略标识 |
| `routing.default_agent_id` | string | 默认投递 Agent |
| `routing.dm_scope` | string | 会话隔离，沿用 OpenClaw：`main`、`per-peer`、`per-channel-peer`、`per-account-channel-peer` |
| `config` | object | 非敏感平台参数 |
| `accounts` | array | 多账号配置；账号级非敏感字段放这里，账号密钥仍走 `channel_credential` |

`metadata_json` 推荐结构：

```json
{
  "icon_url": "",
  "catalog_group": "office",
  "catalog_source": "plugin_manifest",
  "external_id": "page_or_app_id",
  "last_error_code": "",
  "last_error_message": "",
  "connected_at": "",
  "schema_version": 1
}
```

图标策略调整为：自定义图标放 `metadata_json.icon_url`；为空时前端按 `config_json.type` + `variant` 使用内置图标。不再为图标单独扩列。

### 6.4 表：`channel_credential`

对齐 aranea 现有迁移：

```sql
CREATE TABLE IF NOT EXISTS channel_credential (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  credential_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  secret_ref TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(channel_id, credential_key)
);
```

| 字段 | 说明 |
|------|------|
| `credential_key` | `app_secret`、`bot_token`、`access_token`、`verify_token`、`encoding_aes_key`、`signing_secret` 等 |
| `secret_ref` | 指向本地加密密钥、KMS、系统 credential store 或环境变量，不返回前端明文 |
| `metadata_json` | `expires_at`、`rotated_at`、`masked_preview`、`source` 等 |

编辑页密钥字段留空表示不修改；前端只展示「已配置」和脱敏预览，后端接口永不返回 `secret_ref` 背后的明文。

### 6.5 表：`channel_delivery`

用于 webhook / polling / gateway 消息入站后的投递状态，不存完整消息明文：

```sql
CREATE TABLE IF NOT EXISTS channel_delivery (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  payload_json TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
```

`payload_json` 仅存事件 ID、平台 peer、消息摘要 hash、耗时、重试次数等调试信息；完整消息进入会话消息表或按合规要求脱敏。

### 6.6 PlatformResource API 映射

后端建议复用 aranea 的 `PlatformService` 语义，资源名固定为 `channels`：

| API | 行为 |
|-----|------|
| `GET /api/channels/catalog` | 返回 20+ 可接入平台、字段 schema、UI hints、是否已安装插件 |
| `GET /api/channels` | `ListPlatformResources("channels")`，返回所有未删除 channel |
| `POST /api/channels` | 创建 `channel`，写 `config_json`，凭据写 `channel_credential` |
| `GET /api/channels/:id` | 读取单条配置；密钥只返回 configured/masked 状态 |
| `PUT /api/channels/:id` | 更新配置；空密钥不覆盖旧凭据 |
| `POST /api/channels/:id/toggle` | 只更新 `enabled`，必要时通知 runtime reload |
| `POST /api/channels/:id/test` | 临时读取 config + credential，调用平台健康检查 |
| `DELETE /api/channels/:id` | 软删除，写 `deleted_at`，停用运行时 adapter |

为了兼容现有 ClawPanel/OpenClaw 配置，可保留一个同步层：

1. 从旧 `openclaw.json.channels` 导入时，生成 `channel` 记录，`channel_key` 使用旧 key。
2. 写回 OpenClaw 运行目录时，按 `config_json.type` 渲染为旧插件需要的 `channels.<id>` 与 `plugins.entries.<id>`。
3. 新架构内以数据库为源；`openclaw.json` 只作为外部网关兼容产物。

### 6.7 运行时加载与使用

运行时新增 `ChannelSource`，接口形态参考 aranea 的 `ConfiguredPluginSource`：

```go
type ChannelRuntimeConfig struct {
    ID          string
    Key         string
    Type        string
    Enabled     bool
    ConfigJSON  string
    MetadataJSON string
}

type ConfiguredChannelSource interface {
    EnabledChannelConfigs(context.Context) ([]ChannelRuntimeConfig, error)
}
```

运行流程：

1. Runtime 启动或收到配置变更事件时，调用 `EnabledChannelConfigs(ctx)`。
2. 只加载 `enabled=1`、`deleted_at=''`、`status in ('active','pending_auth')` 的 Channel。
3. Channel factory 根据 `config_json.type` 创建对应 adapter：Webhook、WebSocket、Polling、Gateway、Bridge。
4. Adapter 启动前按需解析 `channel_credential.secret_ref`，注入运行时内存，绝不写入日志。
5. 入站消息标准化为统一 `ChannelMessage`，携带 `channel_id`、`channel_key`、`type`、`account_id`、`peer`、`sender`。
6. 路由层按 `routing.default_agent_id`、Agent 绑定、会话隔离策略投递。
7. 失败写 `channel_delivery` 和 `metadata_json.last_error_*`，严重错误可将 `status` 置为 `error`。

这与 ClawPanel 现有行为的对应关系：

| 现有 ClawPanel 行为 | 新设计 |
|---------------------|--------|
| `GET /openclaw/channel-catalog` 从插件 manifest 和手工列表组装 | `GET /api/channels/catalog` 继续读取 manifest/schema，但不等同于已配置实例 |
| `GET /openclaw/channels` 直接返回 `openclaw.json.channels` | `GET /api/channels` 返回数据库 `channel` 列表 |
| `PUT /openclaw/channels/:id` 直接写 `channels[id]` | `PUT /api/channels/:id` 更新 `config_json`，必要时同步兼容文件 |
| `POST /openclaw/toggle-channel` 同步 `channels` 和 `plugins.entries` | `POST /api/channels/:id/toggle` 更新 `enabled`，同步插件启用状态由兼容层处理 |
| QQ 停用时停止 NapCat、恢复监控 | 作为 `qq` adapter 的 lifecycle hook，而不是散落在通用 toggle handler |

### 6.8 20+ Channel 配置模板

后端 catalog 中每个平台至少包含：

```json
{
  "type": "telegram",
  "label": "Telegram",
  "group": "overseas",
  "receive_modes": ["webhook", "polling"],
  "config_schema": {},
  "credential_schema": {},
  "ui_hints": {}
}
```

`config_schema` 描述非敏感字段，`credential_schema` 描述敏感字段，`ui_hints` 描述分组、帮助链接、是否支持 pairing / qrcode / test connection。前端步骤二优先由 schema 渲染，只有 QQ、飞书、企业微信、微信这类复杂平台保留手写增强组件。

### 6.9 索引建议

- `channel(channel_key)` 唯一索引已由表结构保证。
- `channel(enabled, status, sort_order)` 用于列表和 runtime 加载。
- `channel(deleted_at, updated_at)` 用于同步与分页。
- `channel_credential(channel_id, credential_key)` 唯一索引已由表结构保证。
- `channel_delivery(channel_id, created_at)` 用于最近投递查询。

---

## 7. 附录

### 7.1 各平台「保存并测试连接」行为（建议）

| type | 测试内容（示例） | 说明 |
|--------------|------------------|------|
| `feishu` | 使用 app_id + secret 调开放平台获取 `tenant_access_token` 或等价接口 | 失败时展示飞书错误码 |
| `wechat` | 按子类型调 `gettoken` / 企微 `gettoken` 等 | 校验 app_id/secret 或 corp/agent |
| `qq` | 按官方文档校验机器人凭证 | 字段名以 QQ 开放平台为准 |
| `facebook` | 校验 Page Token 或 `debug_token`（视实现） | 可顺带校验 `page_id` |
| `telegram` | `getMe` | 校验 bot_token |
| `whatsapp` | 调 Graph 试拉 `phone_number_id` 或消息 API 健康检查 | Token 过期时 `status=error` |

具体 URL 与错误码映射由后端实现文档维护；本表用于**验收与测试用例**对齐。

### 7.2 飞书与 Lark（`feishu`）

- **产品**：同一平台 `type=feishu` 表示「飞书或国际版 Lark」，界面文案可写「飞书 / Lark」。
- **实现**：若开放平台域名、部分参数不同，在 `config_json.config.region` 内增加 `feishu | lark` 或等价字段，**不拆表**；若两套完全独立，再评估拆为两个平台 `type`。

### 7.3 微信：两种建模方式

| 方式 | 做法 | 适用 |
|------|------|------|
| A | `type=wechat` + `config_json.config.subtype` | 当前文档默认；统计时按 subtype 过滤 |
| B | 拆为 `wechat_official` / `wechat_miniprogram` / `wechat_work` 三个类型 | 列表筛选更直观，但枚举与表单模板增多 |

选型在实现前锁定，避免前后端混用。

---

## 8. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | — | 初稿：录入 UI、分平台控件、库表字段 |
| 1.1 | 2026-04-21 | 评审纳入：列表/编辑页、`status`/`enabled`、`config` 与唯一约束、安全与合规、草稿/离开、附录（测试连接、飞书/Lark、微信建模）、可选 `row_version`/`deleted_at` |
| 1.2 | 2026-04-21 | 列表：添加完成后的列字段与数据来源；操作列「编辑」「删除」及复制 Webhook、与启用开关的排布说明 |
| 1.3 | 2026-04-21 | `channel` 表增加 `icon_url`；§6.1 图标策略；列表「平台」列与 §4.2 录入控件对齐 |
| 1.4 | 2026-04-25 | 扩展 20+ Channel catalog；按 aranea `PlatformResource` 架构重写存储、凭据、delivery、API 与运行时加载设计 |
