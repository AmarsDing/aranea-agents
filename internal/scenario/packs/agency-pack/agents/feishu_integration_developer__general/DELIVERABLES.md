## 技术交付物
### 飞书应用项目结构

```
feishu-integration/
├── src/
│   ├── config/
│   │   ├── feishu.ts              # 飞书应用配置
│   │   └── env.ts                 # 环境变量管理
│   ├── auth/
│   │   ├── token-manager.ts       # Token 获取与缓存
│   │   └── event-verify.ts        # 事件订阅校验
│   ├── bot/
│   │   ├── command-handler.ts     # 机器人命令处理
│   │   ├── message-sender.ts      # 消息发送封装
│   │   └── card-builder.ts        # 消息卡片构建器
│   ├── approval/
│   │   ├── approval-define.ts     # 审批定义管理
│   │   ├── approval-instance.ts   # 审批实例操作
│   │   └── approval-callback.ts   # 审批事件回调
│   ├── bitable/
│   │   ├── table-client.ts        # 多维表格 CRUD 操作
│   │   └── sync-service.ts        # 数据同步服务
│   ├── sso/
│   │   ├── oauth-handler.ts       # OAuth 授权流程
│   │   └── user-sync.ts           # 用户信息同步
│   ├── webhook/
│   │   ├── event-dispatcher.ts    # 事件分发器
│   │   └── handlers/              # 按类型组织的事件处理器
│   └── utils/
│       ├── http-client.ts         # HTTP 请求封装
│       ├── logger.ts              # 日志工具
│       └── retry.ts               # 重试机制
├── tests/
├── docker-compose.yml
└── package.json
```

### Token 管理与 API 请求封装

```typescript
// src/auth/token-manager.ts
import * as lark from '@larksuiteoapi/node-sdk';

const client = new lark.Client({
  appId: process.env.FEISHU_APP_ID!,
  appSecret: process.env.FEISHU_APP_SECRET!,
  disableTokenCache: false, // SDK 内置缓存
});

export { client };

// 手动 Token 管理场景（不使用 SDK 时）
class TokenManager {
  private token: string = '';
  private expireAt: number = 0;

  async getTenantAccessToken(): Promise<string> {
    if (this.token && Date.now() < this.expireAt) {
      return this.token;
    }

    const resp = await fetch(
      'https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          app_id: process.env.FEISHU_APP_ID,
          app_secret: process.env.FEISHU_APP_SECRET,
        }),
      }
    );

    const data = await resp.json();
    if (data.code !== 0) {
      throw new Error(`获取 Token 失败: ${data.msg}`);
    }

    this.token = data.tenant_access_token;
    // 提前 5 分钟过期，避免边界问题
    this.expireAt = Date.now() + (data.expire - 300) * 1000;
    return this.token;
  }
}

export const tokenManager = new TokenManager();
```

### 消息卡片构建器与发送器

```typescript
// src/bot/card-builder.ts
interface CardAction {
  tag: string;
  text: { tag: string; content: string };
  type: string;
  value: Record<string, string>;
}

// 构建审批通知卡片
function buildApprovalCard(params: {
  title: string;
  applicant: string;
  reason: string;
  amount: string;
  instanceId: string;
}): object {
  return {
    config: { wide_screen_mode: true },
    header: {
      title: { tag: 'plain_text', content: params.title },
      template: 'orange',
    },
    elements: [
      {
        tag: 'div',
        fields: [
          {
            is_short: true,
            text: { tag: 'lark_md', content: `**申请人**\n${params.applicant}` },
          },
          {
            is_short: true,
            text: { tag: 'lark_md', content: `**金额**\n¥${params.amount}` },
          },
        ],
      },
      {
        tag: 'div',
        text: { tag: 'lark_md', content: `**事由**\n${params.reason}` },
      },
      { tag: 'hr' },
      {
        tag: 'action',
        actions: [
          {
            tag: 'button',
            text: { tag: 'plain_text', content: '通过' },
            type: 'primary',
            value: { action: 'approve', instance_id: params.instanceId },
          },
          {
            tag: 'button',
            text: { tag: 'plain_text', content: '拒绝' },
            type: 'danger',
            value: { action: 'reject', instance_id: params.instanceId },
          },
          {
            tag: 'button',
            text: { tag: 'plain_text', content: '查看详情' },
            type: 'default',
            url: `https://your-domain.com/approval/${params.instanceId}`,
          },
        ],
      },
    ],
  };
}

// 发送消息卡片
async function sendCardMessage(
  client: any,
  receiveId: string,
  receiveIdType: 'open_id' | 'chat_id' | 'user_id',
  card: object
): Promise<string> {
  const resp = await client.im.message.create({
    params: { receive_id_type: receiveIdType },
    data: {
      receive_id: receiveId,
      msg_type: 'interactive',
      content: JSON.stringify(card),
    },
  });

  if (resp.code !== 0) {
    throw new Error(`发送卡片失败: ${resp.msg}`);
  }
  return resp.data!.message_id;
}
```

### 事件订阅与回调处理

```typescript
// src/webhook/event-dispatcher.ts
import * as lark from '@larksuiteoapi/node-sdk';
import express from 'express';

const app = express();

const eventDispatcher = new lark.EventDispatcher({
  encryptKey: process.env.FEISHU_ENCRYPT_KEY || '',
  verificationToken: process.env.FEISHU_VERIFICATION_TOKEN || '',
});

// 监听机器人接收消息事件
eventDispatcher.register({
  'im.message.receive_v1': async (data) => {
    const message = data.message;
    const chatId = message.chat_id;
    const content = JSON.parse(message.content);

    // 处理纯文本消息
    if (message.message_type === 'text') {
      const text = content.text as string;
      await handleBotCommand(chatId, text);
    }
  },
});

// 监听审批状态变更
eventDispatcher.register({
  'approval.approval.updated_v4': async (data) => {
    const instanceId = data.approval_code;
    const status = data.status;

    if (status === 'APPROVED') {
      await onApprovalApproved(instanceId);
    } else if (status === 'REJECTED') {
      await onApprovalRejected(instanceId);
    }
  },
});

// 卡片动作回调处理器
const cardActionHandler = new lark.CardActionHandler({
  encryptKey: process.env.FEISHU_ENCRYPT_KEY || '',
  verificationToken: process.env.FEISHU_VERIFICATION_TOKEN || '',
}, async (data) => {
  const action = data.action.value;

  if (action.action === 'approve') {
    await processApproval(action.instance_id, true);
    // 返回更新后的卡片
    return {
      toast: { type: 'success', content: '审批已通过' },
    };
  }
  return {};
});

app.use('/webhook/event', lark.adaptExpress(eventDispatcher));
app.use('/webhook/card', lark.adaptExpress(cardActionHandler));

app.listen(3000, () => console.log('飞书事件服务已启动'));
```

### 多维表格操作

```typescript
// src/bitable/table-client.ts
class BitableClient {
  constructor(private client: any) {}

  // 查询表格记录（带过滤和分页）
  async listRecords(
    appToken: string,
    tableId: string,
    options?: {
      filter?: string;
      sort?: string[];
      pageSize?: number;
      pageToken?: string;
    }
  ) {
    const resp = await this.client.bitable.appTableRecord.list({
      path: { app_token: appToken, table_id: tableId },
      params: {
        filter: options?.filter,
        sort: options?.sort ? JSON.stringify(options.sort) : undefined,
        page_size: options?.pageSize || 100,
        page_token: options?.pageToken,
      },
    });

    if (resp.code !== 0) {
      throw new Error(`查询记录失败: ${resp.msg}`);
    }
    return resp.data;
  }

  // 批量创建记录
  async batchCreateRecords(
    appToken: string,
    tableId: string,
    records: Array<{ fields: Record<string, any> }>
  ) {
    const resp = await this.client.bitable.appTableRecord.batchCreate({
      path: { app_token: appToken, table_id: tableId },
      data: { records },
    });

    if (resp.code !== 0) {
      throw new Error(`批量创建记录失败: ${resp.msg}`);
    }
    return resp.data;
  }

  // 更新单条记录
  async updateRecord(
    appToken: string,
    tableId: string,
    recordId: string,
    fields: Record<string, any>
  ) {
    const resp = await this.client.bitable.appTableRecord.update({
      path: {
        app_token: appToken,
        table_id: tableId,
        record_id: recordId,
      },
      data: { fields },
    });

    if (resp.code !== 0) {
      throw new Error(`更新记录失败: ${resp.msg}`);
    }
    return resp.data;
  }
}

// 示例：将外部订单数据同步到多维表格
async function syncOrdersToBitable(orders: any[]) {
  const bitable = new BitableClient(client);
  const appToken = process.env.BITABLE_APP_TOKEN!;
  const tableId = process.env.BITABLE_TABLE_ID!;

  const records = orders.map((order) => ({
    fields: {
      '订单编号': order.orderId,
      '客户姓名': order.customerName,
      '订单金额': order.amount,
      '状态': order.status,
      '创建时间': order.createdAt,
    },
  }));

  // 每批最多 500 条
  for (let i = 0; i < records.length; i += 500) {
    const batch = records.slice(i, i + 500);
    await bitable.batchCreateRecords(appToken, tableId, batch);
  }
}
```

### 审批流程集成

```typescript
// src/approval/approval-instance.ts

// 通过 API 创建审批实例
async function createApprovalInstance(params: {
  approvalCode: string;
  userId: string;
  formValues: Record<string, any>;
  approvers?: string[];
}) {
  const resp = await client.approval.instance.create({
    data: {
      approval_code: params.approvalCode,
      user_id: params.userId,
      form: JSON.stringify(
        Object.entries(params.formValues).map(([name, value]) => ({
          id: name,
          type: 'input',
          value: String(value),
        }))
      ),
      node_approver_user_id_list: params.approvers
        ? [{ key: 'node_1', value: params.approvers }]
        : undefined,
    },
  });

  if (resp.code !== 0) {
    throw new Error(`创建审批失败: ${resp.msg}`);
  }
  return resp.data!.instance_code;
}

// 查询审批实例详情
async function getApprovalInstance(instanceCode: string) {
  const resp = await client.approval.instance.get({
    params: { instance_id: instanceCode },
  });

  if (resp.code !== 0) {
    throw new Error(`查询审批实例失败: ${resp.msg}`);
  }
  return resp.data;
}
```

### SSO 扫码登录

```typescript
// src/sso/oauth-handler.ts
import { Router } from 'express';

const router = Router();

// 第 1 步：重定向到飞书授权页
router.get('/login/feishu', (req, res) => {
  const redirectUri = encodeURIComponent(
    `${process.env.BASE_URL}/callback/feishu`
  );
  const state = generateRandomState();
  req.session!.oauthState = state;

  res.redirect(
    `https://open.feishu.cn/open-apis/authen/v1/authorize` +
    `?app_id=${process.env.FEISHU_APP_ID}` +
    `&redirect_uri=${redirectUri}` +
    `&state=${state}`
  );
});

// 第 2 步：飞书回调——用 code 换取 user_access_token
router.get('/callback/feishu', async (req, res) => {
  const { code, state } = req.query;

  if (state !== req.session!.oauthState) {
    return res.status(403).json({ error: 'State 不匹配——可能存在 CSRF 攻击' });
  }

  const tokenResp = await client.authen.oidcAccessToken.create({
    data: {
      grant_type: 'authorization_code',
      code: code as string,
    },
  });

  if (tokenResp.code !== 0) {
    return res.status(401).json({ error: '授权失败' });
  }

  const userToken = tokenResp.data!.access_token;

  // 第 3 步：获取用户信息
  const userResp = await client.authen.userInfo.get({
    headers: { Authorization: `Bearer ${userToken}` },
  });

  const feishuUser = userResp.data;
  // 绑定或创建与飞书用户关联的本地用户
  const localUser = await bindOrCreateUser({
    openId: feishuUser!.open_id!,
    unionId: feishuUser!.union_id!,
    name: feishuUser!.name!,
    email: feishuUser!.email!,
    avatar: feishuUser!.avatar_url!,
  });

  const jwt = signJwt({ userId: localUser.id });
  res.redirect(`${process.env.FRONTEND_URL}/auth?token=${jwt}`);
});

export default router;
```
