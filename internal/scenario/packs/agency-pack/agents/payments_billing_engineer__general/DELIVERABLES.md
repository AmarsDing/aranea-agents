## 📋 你的技术交付物
### 幂等支付创建（TypeScript + Stripe）

```typescript
// 幂等键从业务操作派生，因此客户端重试、服务器重试
// 和双击都解析为同一次扣款。
import Stripe from 'stripe';

const stripe = new Stripe(process.env.STRIPE_SECRET_KEY!, { apiVersion: '2024-06-20' });

export async function createPaymentForOrder(order: Order): Promise<Stripe.PaymentIntent> {
  return stripe.paymentIntents.create(
    {
      amount: order.totalMinorUnits,          // 整数美分 - 绝不用浮点数
      currency: order.currency,               // ISO 4217，小写
      customer: order.stripeCustomerId,
      metadata: { order_id: order.id },       // 始终将 PSP 对象链接回你的领域
      automatic_payment_methods: { enabled: true },
    },
    { idempotencyKey: `order-${order.id}-attempt-${order.paymentAttempt}` }
  );
}
```

### Webhook 处理器：签名、去重、乱序安全

```typescript
export async function handleStripeWebhook(req: Request): Promise<Response> {
  // 1. 根据原始正文验证签名 - 解析后的 JSON 会破坏验证
  const event = stripe.webhooks.constructEvent(
    await req.text(),
    req.headers.get('stripe-signature')!,
    process.env.STRIPE_WEBHOOK_SECRET!
  );

  // 2. 去重：至少一次投递意味着实践中"两次"
  const alreadyProcessed = await db.webhookEvents.insertIgnore({ id: event.id });
  if (alreadyProcessed) return new Response('duplicate', { status: 200 });

  // 3. 绝不信任事件顺序 - 重新获取当前状态而非应用增量
  switch (event.type) {
    case 'payment_intent.succeeded': {
      const pi = await stripe.paymentIntents.retrieve(
        (event.data.object as Stripe.PaymentIntent).id
      );
      if (pi.status === 'succeeded') {
        await fulfillOrder(pi.metadata.order_id); // 本身必须幂等
      }
      break;
    }
    case 'charge.dispute.created':
      await freezeOrderAndNotifyFinance(event); // 证据截止日期从此刻开始
      break;
  }

  // 4. 快速返回 2xx；在队列中做重活，让 PSP 不会重试风暴你
  return new Response('ok', { status: 200 });
}
```

### 订阅生命周期状态机

```text
trialing ──试用结束──▶ active ──支付失败──▶ past_due ──催收耗尽──▶ canceled
   │                       │  ▲                        │
   │ 需要预付卡             │  └──支付恢复──────────────┘
   ▼                       ▼
incomplete ──3DS/行动──▶ 升级/降级 → 按比例 credit 或发票行项目
```

| 转换 | 触发器 | 你的系统必须 |
|------------|---------|------------------|
| `active → past_due` | 续费扣款失败 | 保持访问（宽限期）、启动催收邮件、按智能计划重试 |
| `past_due → active` | 重试成功或卡更新 | 静默恢复、记录恢复来源用于流失分析 |
| `past_due → canceled` | 催收耗尽（例如 4 次重试 / 21 天） | 撤销访问、为赢回窗口保留数据、发出流失事件 |
| `active → active`（计划变更） | 周期中升级 | 按比例：credit 未使用时间、立即发票差额 |

### 每日对账查询

```sql
-- 每个处理器付款必须等于该付款的账本条目总和。
-- 任何非零偏差都是事件，而非好奇。
SELECT
  p.payout_id,
  p.arrival_date,
  p.amount_minor                             AS processor_amount,
  COALESCE(SUM(l.amount_minor), 0)           AS ledger_amount,
  p.amount_minor - COALESCE(SUM(l.amount_minor), 0) AS drift
FROM processor_payouts p
LEFT JOIN ledger_entries l ON l.payout_id = p.payout_id
GROUP BY p.payout_id, p.arrival_date, p.amount_minor
HAVING p.amount_minor <> COALESCE(SUM(l.amount_minor), 0)
ORDER BY p.arrival_date DESC;
```

### PCI 范围速查表

| 集成方式 | PCI 验证 | 经验法则 |
|-------------------|---------------|----------------|
| 托管结账页面（Stripe Checkout、PayPal 重定向） | SAQ A | 卡数据永不触碰你的页面 - 最小范围，默认选择 |
| 嵌入式 iframe 字段（Stripe Elements、Adyen Drop-in） | SAQ A | 你的页面托管 iframe；PSP 托管输入 |
| 你的表单通过 PSP JS 提交卡数据（旧式直接提交） | SAQ A-EP | 你的页面可能被攻击 - 新构建避免使用 |
| 卡数据触碰你的服务器 | SAQ D / 完整审计 | 几乎从未合理 - 重新设计 |
