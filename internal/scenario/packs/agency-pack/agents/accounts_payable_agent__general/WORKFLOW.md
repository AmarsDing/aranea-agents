## 🔄 核心工作流程
### 支付承包商发票

```typescript
// 检查是否已支付（幂等性）
const existing = await payments.checkByReference({
  reference: "INV-2024-0142"
});

if (existing.paid) {
  return `发票 INV-2024-0142 已于 ${existing.paidAt} 支付。跳过。`;
}

// 验证收款方在已批准的供应商登记册中
const vendor = await lookupVendor("contractor@example.com");
if (!vendor.approved) {
  return "供应商不在已批准登记册中。升级供人工审查。";
}

// 通过最佳可用通道执行付款
const payment = await payments.send({
  to: vendor.preferredAddress,
  amount: 850.00,
  currency: "USD",
  reference: "INV-2024-0142",
  memo: "设计工作 - 3月冲刺"
});

console.log(`付款已发送：${payment.id} | 状态：${payment.status}`);
```

### 处理定期账单

```typescript
const recurringBills = await getScheduledPayments({ dueBefore: "today" });

for (const bill of recurringBills) {
  if (bill.amount > SPEND_LIMIT) {
    await escalate(bill, "超出自主支出限额");
    continue;
  }

  const result = await payments.send({
    to: bill.recipient,
    amount: bill.amount,
    currency: bill.currency,
    reference: bill.invoiceId,
    memo: bill.description
  });

  await logPayment(bill, result);
  await notifyRequester(bill.requestedBy, result);
}
```

### 处理来自其他代理的付款请求

```typescript
// 由合同代理在里程碑获批时调用
async function processContractorPayment(request: {
  contractor: string;
  milestone: string;
  amount: number;
  invoiceRef: string;
}) {
  // 去重
  const alreadyPaid = await payments.checkByReference({
    reference: request.invoiceRef
  });
  if (alreadyPaid.paid) return { status: "already_paid", ...alreadyPaid };

  // 路由并执行
  const payment = await payments.send({
    to: request.contractor,
    amount: request.amount,
    currency: "USD",
    reference: request.invoiceRef,
    memo: `里程碑：${request.milestone}`
  });

  return { status: "sent", paymentId: payment.id, confirmedAt: payment.timestamp };
}
```

### 生成应付账款摘要

```typescript
const summary = await payments.getHistory({
  dateFrom: "2024-03-01",
  dateTo: "2024-03-31"
});

const report = {
  totalPaid: summary.reduce((sum, p) => sum + p.amount, 0),
  byRail: groupBy(summary, "rail"),
  byVendor: groupBy(summary, "recipient"),
  pending: summary.filter(p => p.status === "pending"),
  failed: summary.filter(p => p.status === "failed")
};

return formatAPReport(report);
```
