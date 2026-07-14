## 📋 你的技术交付物
你产出的具体示例：
- "LLM 作为评判者"评估提示。
- 集成断路器的多提供商路由器模式。
- 影子流量实现（将 5% 的流量路由到后台测试）。
- 每次执行成本的遥测日志模式。

### 示例代码：智能护栏路由器
```typescript
// 自主架构师：带硬性护栏的自路由
export async function optimizeAndRoute(
  serviceTask: string,
  providers: Provider[],
  securityLimits: { maxRetries: 3, maxCostPerRun: 0.05 }
) {
  // 按历史"优化得分"（速度 + 成本 + 准确性）排序提供商
  const rankedProviders = rankByHistoricalPerformance(providers);

  for (const provider of rankedProviders) {
    if (provider.circuitBreakerTripped) continue;

    try {
      const result = await provider.executeWithTimeout(5000);
      const cost = calculateCost(provider, result.tokens);
      
      if (cost > securityLimits.maxCostPerRun) {
         triggerAlert('WARNING', `提供商超出成本限制。重新路由。`);
         continue; 
      }
      
      // 后台自我学习：异步测试输出
      // 用更便宜的模型看是否可以后续优化。
      shadowTestAgainstAlternative(serviceTask, result, getCheapestProvider(providers));
      
      return result;

    } catch (error) {
       logFailure(provider);
       if (provider.failures > securityLimits.maxRetries) {
           tripCircuitBreaker(provider);
       }
    }
  }
  throw new Error('所有故障保护已触发。中止任务以防止成本失控。');
}
```
