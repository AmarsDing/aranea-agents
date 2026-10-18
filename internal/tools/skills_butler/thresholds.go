package skills_butler

// 统计分析窗口与判定阈值（审计 T3：0.6/0.8/5000/2000/4.29 等魔法数收编为
// 具名常量，optimize_skill.go 与 analyze_skill_usage.go 共用同一来源，
// 避免两处阈值漂移）。
const (
	// statsWindowDays 是调用统计的回看窗口天数。
	statsWindowDays = 30

	// weeksPerMonth 是 30 天窗口折合周数（30 天 ≈ 4.29 周），用于周频折算。
	// 保留原字面值 4.29，不做 30.0/7.0 精确换算，以免改变既有口径。
	weeksPerMonth = 4.29
	// weeksPerQuarter 是 90 天窗口折合周数（90 天 ≈ 12.86 周）。
	weeksPerQuarter = 12.86

	// successRateCriticalThreshold：成功率低于此值判为高危（critical/高优先级建议）。
	successRateCriticalThreshold = 0.6
	// successRateHealthyThreshold：成功率达到此值判为健康（healthy/无可靠性建议）。
	successRateHealthyThreshold = 0.8

	// avgDurationSlowMs：平均耗时超过此值产出中优先级性能建议。
	avgDurationSlowMs = 5000
	// avgDurationImproveMs：平均耗时超过此值产出低优先级优化建议。
	avgDurationImproveMs = 2000

	// callsPerWeekLowThreshold：周频低于此值判为使用率极低（usage 建议 / critical）。
	callsPerWeekLowThreshold = 2
	// callsPerWeekActiveThreshold：周频超过此值且成功率达标时判为 warning（高频但不够稳）。
	callsPerWeekActiveThreshold = 5
)
