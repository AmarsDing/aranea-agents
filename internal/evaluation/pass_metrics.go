package evaluation

import (
	trpceval "trpc.group/trpc-go/trpc-agent-go/evaluation"
)

// computePassMetrics derives pass@k and pass^k from framework evaluation summary.
func computePassMetrics(result *trpceval.EvaluationResult, numRuns int) (passAtK, passHatK float32) {
	if result == nil || result.EvalResult == nil || result.EvalResult.Summary == nil || numRuns <= 1 {
		return 0, 0
	}
	n, c, err := trpceval.ParsePassNC(result)
	if err != nil || n <= 0 {
		return 0, 0
	}
	k := numRuns
	if k > n {
		k = n
	}
	if pk, err := trpceval.PassAtK(n, c, k); err == nil {
		passAtK = float32(pk)
	}
	if ph, err := trpceval.PassHatK(n, c, k); err == nil {
		passHatK = float32(ph)
	}
	return passAtK, passHatK
}
