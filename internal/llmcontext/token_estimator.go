package llmcontext

import "sync"

// defaultCharsPerToken is the blended chars-per-token ratio used before any
// provider-reported (authoritative) usage calibrates the estimator. For CJK
// text 1 char ≈ 1-2 tokens; for Latin text ~4 chars per token; 2.5 avoids
// severe underestimation for CJK-dominant content.
const defaultCharsPerToken = 2.5

// incrementalBytesPerToken converts bytes observed since the last
// authoritative anchor into tokens (~4 bytes per token for UTF-8 mixed text).
const incrementalBytesPerToken = 4.0

// TokenEstimator is a dual-anchor token estimator: a provider-reported
// authoritative anchor (tokens for a known char count) calibrates the ratio,
// and bytes observed after that anchor are estimated incrementally.
// Safe for concurrent use.
type TokenEstimator struct {
	mu                  sync.Mutex
	authoritativeTokens int
	authoritativeChars  int
	incrementalBytes    int
}

func NewTokenEstimator() *TokenEstimator { return &TokenEstimator{} }

// RecordAuthoritative anchors the estimator at a provider-reported token count
// for a known char count, and resets incremental accumulation.
func (e *TokenEstimator) RecordAuthoritative(tokens, chars int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.authoritativeTokens = tokens
	e.authoritativeChars = chars
	e.incrementalBytes = 0
}

// RecordIncremental accumulates bytes observed since the last anchor.
func (e *TokenEstimator) RecordIncremental(bytes int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.incrementalBytes += bytes
}

// EstimateChars estimates tokens for the given char count using the calibrated
// ratio (or the default blended ratio when no anchor exists).
func (e *TokenEstimator) EstimateChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	charsPerToken := e.charsPerToken()
	tokens := int(float64(chars) / charsPerToken)
	if tokens == 0 {
		return 1
	}
	return tokens
}

// EstimateTotal returns authoritative tokens plus incremental bytes converted
// at the incremental ratio.
func (e *TokenEstimator) EstimateTotal() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authoritativeTokens + int(float64(e.incrementalBytes)/incrementalBytesPerToken)
}

func (e *TokenEstimator) charsPerToken() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.authoritativeChars > 0 && e.authoritativeTokens > 0 {
		return float64(e.authoritativeChars) / float64(e.authoritativeTokens)
	}
	return defaultCharsPerToken
}

// shared is the process-level estimator calibrated by provider usage reports
// and read by budget-relevant estimation call sites.
var shared = NewTokenEstimator()

// RecordAuthoritativeUsage calibrates the shared estimator with a
// provider-reported token count for a known char count.
// 注意：共享估算器是进程级单例，多模型混用时比率会漂移（后一次校准覆盖前一次）。
// 与 Grok 同语义（单一比率），接受此近似——压缩场景的 token 估算不需要 per-model 精度。
func RecordAuthoritativeUsage(tokens, chars int) {
	shared.RecordAuthoritative(tokens, chars)
}

// EstimateTokensFromChars estimates tokens using the shared calibrated
// estimator. Replaces ad-hoc per-site char/token ratios so all budget-relevant
// estimates share one calibrated ratio.
func EstimateTokensFromChars(chars int) int {
	return shared.EstimateChars(chars)
}

// resetSharedEstimatorForTest restores the shared estimator to its default
// state. For tests only.
func resetSharedEstimatorForTest() {
	shared = NewTokenEstimator()
}
