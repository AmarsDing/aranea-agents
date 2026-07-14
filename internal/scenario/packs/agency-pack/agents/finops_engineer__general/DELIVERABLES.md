## 📋 你的技术交付物
### 标签与分摊策略（其他一切的基础）

```yaml
# 强制标签策略——在开通时强制执行，持续审计。
# 未打标签的资源被隔离到「未分摊」桶，团队对将其驱动到零负责。
required_tags:
  team:        # 责任团队——把成本 + 优化动作路由到人
  service:     # 逻辑服务/应用——产品关心的单位
  environment: # prod | staging | dev——dev/staging 是首要关停目标
  cost_center: # 财务分摊键——桥接到损益表
enforcement:
  - 没有必需标签拒绝开通（SCP / Azure Policy / GCP org policy）
  - 日度审计：已分摊支出 %；目标 > 95%
  - 共享成本（网络、可观测性、共享集群）按文档化、协商一致的键拆分
    （尽可能按使用量，否则按人数）
```

### 优化杠杆优先级（按此顺序执行）

| 优先级 | 杠杆 | 典型节省 | 可靠性风险 | 规则 |
|----------|-------|-----------------|------------------|------|
| 1 | 关停闲置/孤儿（未挂载磁盘、闲置负载均衡器、僵尸环境） | 高 | ~无 | 免费的钱——自动化检测 |
| 2 | 调度非生产（夜间 + 周末停 dev/staging） | ~非生产的 65% | 如果真非生产则无 | 启停自动化，opt-out 而非 opt-in |
| 3 | 合理调整超配计算/DB | 中–高 | 中 | 只在按 SLO 保留余量时 |
| 4 | 存储分层 + 快照生命周期 | 中 | 低 | 生命周期策略，非手动清理 |
| 5 | 出网路径优化（VPC 终端节点、CDN、区域本地化） | 视情况，有时巨大 | 低–中 | 先追踪数据流 |
| 6 | 承诺（RIs / 节省计划 / CUDs）用于稳定剩余 | 覆盖支出的 20–72% | 财务（锁定） | 最后——只在 1–5 稳定后 |

### 承诺规划（量化，非凭感觉）

```text
购买任何预留实例 / 节省计划前：
  1. 基线：过去 30–90 天的常开用量底（不是峰值）
  2. 稳定性检查：这个工作负载在承诺期内会留在原地吗？
     （无待迁移、重构或弃用——与团队确认）
  3. 覆盖目标：覆盖稳定基线的 ~70–85%，留 on-demand
     余量给增长和改变架构的能力
  4. 期限 + 付款：1 年 vs 3 年，预付 vs 无预付，按现金流 + 信心
  5. 事后追踪：利用率（我们用了买的吗？）AND
     覆盖率（多少合格支出被折扣？）——两者都要，月度
未充分利用的承诺是你付了钱又扔掉的折扣。
```

### 单位经济仪表盘（支出以价值衡量）

```sql
-- 每活跃客户成本，趋势化——区分增长与浪费的数字。
-- 总云成本上升是好事，IF 每单位成本持平或下降。
SELECT
  date_trunc('month', usage_date)               AS month,
  SUM(unblended_cost)                            AS total_cloud_cost,
  COUNT(DISTINCT customer_id)                    AS active_customers,
  SUM(unblended_cost) / NULLIF(COUNT(DISTINCT customer_id), 0) AS cost_per_customer,
  SUM(unblended_cost) FILTER (WHERE tag_environment = 'prod')  AS prod_cost,
  SUM(unblended_cost) FILTER (WHERE tag_environment != 'prod') AS nonprod_cost
FROM cost_and_usage
JOIN customer_activity USING (usage_date)
GROUP BY 1 ORDER BY 1;
-- 同时呈现：已分摊 %、承诺覆盖率 %、承诺利用率 %。
```
