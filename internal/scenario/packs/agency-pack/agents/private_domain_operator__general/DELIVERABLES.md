## 技术交付物
### 企微 SCRM 配置蓝图

```yaml
# 企微 SCRM 核心配置
scrm_config:
  # 渠道活码配置
  channel_codes:
    - name: "包裹卡 - 华东仓"
      type: "auto_assign"
      staff_pool: ["sales_team_east"]
      welcome_message: "Hi~ 我是你的专属顾问 {staff_name}。感谢购买！回复 1 领取 VIP 社群邀请，回复 2 领取产品使用指南"
      auto_tags: ["package_insert", "east_china", "new_customer"]
      channel_tracking: "parcel_card_east"

    - name: "直播二维码"
      type: "round_robin"
      staff_pool: ["live_team"]
      welcome_message: "嗨，感谢从直播间过来！发送'直播福利'领取你的专属优惠券~"
      auto_tags: ["livestream_referral", "high_intent"]

    - name: "门店二维码"
      type: "location_based"
      staff_pool: ["store_staff_{city}"]
      welcome_message: "欢迎光临 {store_name}！我是你的专属购物顾问——有任何需要随时找我"
      auto_tags: ["in_store_customer", "{city}", "{store_name}"]

  # 客户标签体系
  tag_system:
    dimensions:
      - name: "客户来源"
        tags: ["package_insert", "livestream", "in_store", "sms", "referral", "organic_search"]
      - name: "消费层级"
        tags: ["high_aov(>500)", "mid_aov(200-500)", "low_aov(<200)"]
      - name: "生命周期阶段"
        tags: ["new_customer", "active_customer", "dormant_customer", "churn_warning", "churned"]
      - name: "兴趣偏好"
        tags: ["skincare", "cosmetics", "personal_care", "baby_care", "health"]
    auto_tagging_rules:
      - trigger: "首购完成"
        add_tags: ["new_customer"]
        remove_tags: []
      - trigger: "30 天无互动"
        add_tags: ["dormant_customer"]
        remove_tags: ["active_customer"]
      - trigger: "累计消费 > 2000"
        add_tags: ["high_value_customer", "vip_candidate"]

  # 客户群配置
  group_config:
    types:
      - name: "福利群"
        max_members: 200
        auto_welcome: "欢迎！我们每天分享好物种草和专属福利。置顶消息有群规哦~"
        sop_template: "welfare_group_sop"
      - name: "VIP 会员群"
        max_members: 100
        entry_condition: "累计消费 > 1000 或被打上 'VIP' 标签"
        auto_welcome: "恭喜成为 VIP 会员！享受专属折扣、新品优先体验和 1 对 1 顾问服务"
        sop_template: "vip_group_sop"
```

### 社群运营 SOP 模板

```markdown
# 福利群日常运营 SOP
