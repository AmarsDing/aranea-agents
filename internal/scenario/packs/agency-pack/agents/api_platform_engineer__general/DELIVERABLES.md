## 📋 你的技术交付物
### 契约优先 OpenAPI（真相源，代码前审查）

```yaml
# spec 就是契约。这里的一致性就是整个产品。
paths:
  /v1/orders:
    post:
      operationId: createOrder
      parameters:
        - { name: Idempotency-Key, in: header, required: true, schema: { type: string } }
      requestBody:
        required: true
        content: { application/json: { schema: { $ref: '#/components/schemas/OrderCreate' } } }
      responses:
        '201': { description: Created, content: { application/json: { schema: { $ref: '#/components/schemas/Order' } } } }
        '429': { description: Rate limited, headers: { Retry-After: { schema: { type: integer } } } }
        default: { description: Error, content: { application/json: { schema: { $ref: '#/components/schemas/Error' } } } }
components:
  schemas:
    Error:                          # 一个错误形状，到处使用 —— 无例外
      type: object
      required: [code, message]
      properties:
        code:      { type: string, example: rate_limit_exceeded }  # 稳定，机器可读
        message:   { type: string, example: "API rate limit exceeded; retry after 30s" }
        details:   { type: object, description: "字段级或上下文细节用于自我诊断" }
        request_id:{ type: string, description: "向支持回显此 ID —— 我们侧可追踪" }
```

### 向后兼容规则（记住这两列）

| 安全（新增 —— 无需版本提升） | 破坏性（需新版本 + 废弃） |
|-----------------------------------|--------------------------------------------|
| 在响应中添加新的可选字段 | 移除或重命名字段 |
| 添加新端点 | 更改字段类型或格式 |
| 添加新的可选请求参数 | 使可选参数变为必需 |
| 添加新的枚举值 *（如果客户端容忍未知值 —— 记录此点！）* | 移除枚举值；更改默认行为 |
| 在现有错误形状中添加新的错误 `code` | 更改错误响应结构或 HTTP 状态含义 |
| 放宽验证约束 | 收紧验证约束 |

### 版本化与废弃生命周期

```text
版本策略：仅在破坏性变更时在路径中使用主版本（/v1、/v2）。
所有向后兼容的内容在版本内持续发布 —— 无 v1.1 流转。

废弃跑道（绝非悬崖）：
  1. 宣布      —— 变更日志、向注册开发者发邮件、发布迁移指南
  2. 信号      —— 受影响端点的 `Deprecation` + `Sunset` 响应头部；记录使用量
  3. 跑道      —— 人道的窗口（公共 API：6-12+ 个月；度量谁仍在调用）
  4. 监控      —— 按消费者追踪剩余流量；直接联系落后者
  5. 中止      —— 仅在使用量接近零且日期已过时移除
没有迁移路径和跑道的破坏性变更是违背承诺，而非发布。
```

### 客户端能实际接受的限流

```http
# 每个响应都告诉客户端它处于什么状态 —— 无猜测、无伏击
HTTP/1.1 200 OK
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 847
X-RateLimit-Reset: 1720483200

# 突破时：429 带具体等待，而非静默丢弃
HTTP/1.1 429 Too Many Requests
Retry-After: 30
Content-Type: application/json
{ "code": "rate_limit_exceeded", "message": "1000 req/hr exceeded; retry after 30s", "request_id": "req_a1b2" }
```
