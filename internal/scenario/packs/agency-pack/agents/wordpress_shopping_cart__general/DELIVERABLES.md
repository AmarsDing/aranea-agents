## 📋 你的技术交付物
### 产品架构蓝图

```
WOOCOMMERCE 产品架构
───────────────────────────────────────
商店配置
  销售地区:  [特定国家 / 全部 / 除...外全部]
  货币:             [USD / EUR / 多币种插件]
  价格输入:       [含税 / 不含税]
  税务计算基于:    [客户配送 / 账单 / 商店地址]

产品类型
  类型:                 [Simple / Variable / Grouped / External / Subscription]
  目录字段:       [名称、描述、图片、分类、标签、品牌]
  库存:            [管理库存? Y/N — 库存量、backorder]
  配送:             [重量、尺寸、配送等级]

变体产品设置
  属性:           [用于变体? Y/N]
    属性:          [Size]   值: [S, M, L, XL]
    属性:          [Color]  值: [Red, Blue, Black]
  变体:           [按属性组合生成]
  每变体:        [SKU、价格、促销价、库存、图片]

定价
  常规价:        [基础价]
  促销价:           [可选 + 排期]
  税类:            [Standard / Reduced / Zero / 自定义]
```

### 结账定制规范

```
结账配置
───────────────────────────────────────
结账类型:         [区块结账（推荐） / 经典 shortcode]

字段:
  标准:            [账单、配送、联系 — 哪些必填]
  自定义字段:       [礼品留言 / 公司 / VAT ID / 配送日期]
  添加方式:           [区块结账: Store API + 扩展
                         经典: woocommerce_checkout_fields filter]

定制契约:
  - 区块结账定制用 Store API / Checkout Blocks
    扩展性 — 不是会在更新时坏的 jQuery DOM hack
  - 经典结账用文档化的 hooks/filters
  - 自定义字段数据存到 order meta + 在管理 + 邮件中显示
  - 服务端验证（绝不信任客户端）; 优雅失败
  - 失败的自定义字段绝不能默默阻塞订单完成

流程验证（每次部署测试，移动端）:
  □ 加入购物车           □ 更新数量
  □ 应用优惠券          □ 计算配送
  □ 计算税         □ 输入支付
  □ 下单           □ 收到订单邮件
  □ 订单出现在管理后台，含正确总额 + 自定义字段
```

### 支付网关集成规范

```
支付网关集成
───────────────────────────────────────
网关:               [WooPayments / Stripe / PayPal / Square / Authorize.Net]
集成类型:      [托管字段/重定向（SAQ A） / 直接（SAQ A-EP）]
模式:                  [沙箱/测试 / 生产 — 显式且在管理后台可见]

凭证（绝不在 DB 明文 / 提交代码中）:
  来源:              [wp-config.php 常量 / 环境变量]
  所需密钥:       [Publishable key, secret key, webhook secret]

支持操作:
  □ Authorize          □ Authorize + Capture
  □ Capture (deferred) □ Void
  □ Refund (full)      □ Refund (partial)
  □ 保存卡片（令牌化 / SCA-3DS）

WEBHOOK / IPN 处理:
  端点:            [WC API 端点 / REST 路由]
  签名验证:  [Header + 签名 secret]
  幂等性:         [按事件/事务 ID 去重]
  日志:              [每个事件通过 WC_Logger]
  映射到:             [订单状态转换]

对账:
  真相源:     [网关结算/支付报告]
  匹配键:           [订单事务 ID ↔ 网关 charge ID]
  差异告警:   [不匹配如何浮现]

上线清单:
  □ 生产密钥仅在生产 wp-config
  □ Webhook 已注册 + 签名生产验证
  □ 测试 charge 已成功 capture 且 refund
  □ 模式确认为生产 LIVE，其他环境沙箱
  □ 订单 + 管理邮件验证
```

### 订单工作流图

```
WOOCOMMERCE 订单状态 + 转换
───────────────────────────────────────
标准生命周期:
  pending ──(收到支付)──▶ processing ──(履约)──▶ completed
     │
     ├──(支付失败)──▶ failed
     └──(未支付超时)──▶ cancelled

其他状态:
  on-hold     [等待支付确认 / 人工审核]
  refunded    [全款或部分退款已发 — 订单保留]
  cancelled   [无履约、无 charge — 记录保留]

自定义状态（示例）:
  processing ─▶ wc-packed ─▶ wc-shipped ─▶ completed
  （通过 register_post_status + woocommerce_order_statuses 注册）

规则:
  - 订单绝不删除 — 只转换/退款
  - 库存按 [processing]（或按设置）减少，cancel/refund 时恢复
  - 每次转换触发钩子: 邮件、履约、ERP/3PL 同步、分析
  - 退款保留完整支付 + 行项历史
```

### 税务与优惠券配置

```
税务配置
───────────────────────────────────────
税务状态:            [启用税务? Y/N]
  价格输入:      [含税 / 不含税]
  计算基于:  [客户配送 / 账单 / 商店基地]
  税类:         [Standard / Reduced rate / Zero rate / 自定义]
  税率:               [按国家/州/邮编 — 标准税率表]
  显示:             [商店 + 购物车中显示含税/不含税价格]

优惠券配置
───────────────────────────────────────
优惠券:                [代码 — 如 SPRING15]
  折扣类型:       [% 折扣 / 固定购物车 / 固定产品]
  金额:              [值]
  限制:        [最低/最高消费、产品/分类、排除促销商品]
  使用限制:        [每优惠券 / 每用户 / X 件]
  仅单独使用: [Y/N — 阻止与其他优惠券叠加]
  过期:              [日期]

叠加行为:
  - 文档化优惠券是否组合或单独使用
  - 测试组合优惠券 + 促销价 + 税务对总额的交互
  - 验证免运费券 + 百分比折扣数学
```

---
