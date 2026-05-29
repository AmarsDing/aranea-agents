export function hasPricingConfigured(prices: {
  inputPrice?: number;
  outputPrice?: number;
  inputPriceCached?: number;
  outputPriceReasoning?: number;
  embeddingPrice?: number;
  cacheWritePrice?: number;
}): boolean {
  const vals = [
    prices.inputPrice,
    prices.outputPrice,
    prices.inputPriceCached,
    prices.outputPriceReasoning,
    prices.embeddingPrice,
    prices.cacheWritePrice
  ];
  return vals.some((v) => typeof v === "number" && v > 0);
}

export function shouldWarnZeroCost(input: {
  totalTokens?: number;
  totalCostMicroUsd?: number;
}): boolean {
  const tokens = input.totalTokens ?? 0;
  const cost = input.totalCostMicroUsd ?? 0;
  return tokens > 0 && cost <= 0;
}

export function pricingWarningMessage(): string {
  return "未配置模型定价时费用显示为 $0，配额 SUM 统计将不准确。请在「模型资源」页维护价格。";
}
