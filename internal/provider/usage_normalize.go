package provider

import (
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// NormalizeUsageTokens 把框架 usage 归一化为统一的账单口径（2026-10-20，
// 报告 F4/F7 修复）：
//
//   - billingInput = PromptTokens + CacheReadTokens + CacheCreationTokens
//     （Anthropic/Bedrock 显式缓存语义：input_tokens 不含 read/write 分量，
//     需补回才是账单输入总量；OpenAI/DeepSeek 语义下 read/write 恒 0 → no-op，
//     PromptTokens 本身即账单输入总量）
//   - output   = CompletionTokens
//   - cached   = CachedTokens（命中读取；Anthropic 下框架已映射
//     CachedTokens=CacheReadInputTokens ≡ CacheReadTokens）
//   - cacheWrite = CacheCreationTokens
//   - reasoning  = CompletionTokensDetails.ReasoningTokens
//
// 统一后的不变量（全链路记账 / 命中率 / 计费只认这一种口径）：
//
//	cached ⊂ billingInput、cacheWrite ⊂ billingInput、reasoning ⊂ output
//
// 归一化发生在提取层（stream_consumer / llmcompat / aux 直读点），biz 层
// 不 import trpcmodel，故本 helper 放在 provider（provider 持有框架模型
// 构造 + usageTap，且 subagent/evolution 等依赖方向为 agent→tools→biz，
// 放 agent 会造成内部环）。
func NormalizeUsageTokens(u *trpcmodel.Usage) (billingInput, output, cached, cacheWrite, reasoning int) {
	if u == nil {
		return 0, 0, 0, 0, 0
	}
	det := u.PromptTokensDetails
	return u.PromptTokens + det.CacheReadTokens + det.CacheCreationTokens,
		u.CompletionTokens,
		det.CachedTokens,
		det.CacheCreationTokens,
		u.CompletionTokensDetails.ReasoningTokens
}
