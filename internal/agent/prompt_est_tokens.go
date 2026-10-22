package agent

import (
	"math"
	"sync"
	"unicode"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// promptEstTokens estimates request tokens for assembly / compression gates.
// Latin still uses the shared 2.5 chars/token blended estimator; CJK runes
// are a 1:1 floor because that estimator under-counts Han/Hangul/Kana
// (S09: ~68k provider tokens estimated as ~16–28k and never hit the 60k hard).
func promptEstTokens(msgs []trpcmodel.Message) int {
	blended := analyzePromptRequest(msgs).EstTokens
	cjk := 0
	for _, m := range msgs {
		cjk += cjkRuneCountMessage(m)
	}
	if cjk > blended {
		return cjk
	}
	return blended
}

// messageEstTokens is the per-message form used when walking history to evict.
func messageEstTokens(m trpcmodel.Message) int {
	runes := messageCharLen(m)
	blended := estTokensFromChars(runes)
	if cjk := cjkRuneCountMessage(m); cjk > blended {
		return cjk
	}
	return blended
}

func cjkRuneCountMessage(m trpcmodel.Message) int {
	n := cjkRuneCount(m.Content)
	for _, p := range m.ContentParts {
		if p.Text != nil {
			n += cjkRuneCount(*p.Text)
		}
	}
	return n
}

func cjkRuneCount(s string) int {
	n := 0
	for _, r := range s {
		if isCJKRune(r) {
			n++
		}
	}
	return n
}

// isCJKRune reports whether r is priced at the CJK 1-tok floor. Beyond the
// Han/Hangul/Kana ideographs this includes CJK symbols & punctuation
// (U+3000–U+303F: 、。！？) and fullwidth forms (U+FF00–U+FFEF: ，！（）),
// which cl100k-class BPEs also tokenize at ~1 tok/rune — pricing them at the
// Latin or "other" rate measurably under-counts CJK prose (A6, design §4.1④).
func isCJKRune(r rune) bool {
	if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
		return true
	}
	return (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF)
}

// ---------------------------------------------------------------------------
// A6 — rune-class segmented estimator (shadow-mode candidate, design §4.4①)
// ---------------------------------------------------------------------------
//
// The legacy path above (max(blended 2.5, cjkCount)) stays the DEFAULT: every
// production call site returns its value unchanged this batch. The segmented
// estimator below prices each rune class separately (CJK / ASCII-Latin /
// emoji / other) so mixed-language prompts no longer pick a single blended
// arm that distorts with the class mix. It is evaluated in SHADOW mode only:
// promptEstTokensShadow returns the legacy estimate and, when a Logger is
// injected, emits one comparison entry per call with the fixed step_id
// "prompt.est_shadow" (approval note P0-3① — flowlog scripts consume that
// field name for the one-week est-diff comparison before any flip).
//
// AS-EVT-01: no new events — shadow comparison is log-only (Informational
// posture by construction), shrink/flip decisions are batch B.

// runeClassCounts breaks text down by pricing class.
type runeClassCounts struct {
	ASCII int
	CJK   int
	Emoji int
	Other int
}

func (c runeClassCounts) totalRunes() int { return c.ASCII + c.CJK + c.Emoji + c.Other }

// classifyRunes buckets every rune of s. Zero-priced runes (combining marks,
// variation selectors, ZWJ — they ride along a base glyph inside one token)
// are dropped from the counts entirely.
func classifyRunes(s string) runeClassCounts {
	var c runeClassCounts
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
			// combining marks: free-riders inside the base glyph's token.
		case r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F):
			// ZWJ / variation selectors: sequence glue, not standalone tokens.
		case r < 128:
			c.ASCII++
		case isCJKRune(r):
			c.CJK++
		case unicode.Is(unicode.So, r):
			// Symbol-Other holds the extended-pictographic emoji blocks (and
			// some dingbats); pre-calibration we price the whole class at the
			// emoji rate — the ±25% fixture band absorbs the mis-rated tail.
			c.Emoji++
		default:
			// Cyrillic/Greek/Arabic/Indic scripts and remaining punctuation:
			// cl100k-class BPEs land around 2 chars/token here.
			c.Other++
		}
	}
	return c
}

// runeClassCountsMessage sums message content + text parts (parallels
// messageCharLen / cjkRuneCountMessage scope).
func runeClassCountsMessage(m trpcmodel.Message) runeClassCounts {
	c := classifyRunes(m.Content)
	for _, p := range m.ContentParts {
		if p.Text != nil {
			d := classifyRunes(*p.Text)
			c.ASCII += d.ASCII
			c.CJK += d.CJK
			c.Emoji += d.Emoji
			c.Other += d.Other
		}
	}
	return c
}

// SegmentedEstConfig prices the four rune classes. Zero fields fall back to
// the documented pre-calibration defaults; calibration (dual-anchor style,
// see SegmentedCalibrator) only ever tightens the Latin arm — the CJK 1:1
// floor and the emoji/other bands are grounded constants until fleet data
// proves otherwise (design §4.7).
type SegmentedEstConfig struct {
	// CharsPerLatinToken prices ASCII runes (default 4.0). The legacy blended
	// arm uses 2.5 chars/token for EVERYTHING; on pure Latin prose that arm
	// over-counts ~1.9×, on mixed text the winning arm under-counts ~30-40%
	// — the class split exists to remove both distortions.
	CharsPerLatinToken float64
	// TokensPerCJKRune prices CJK-class runes (default 1.0 — the grounded
	// floor that motivated the legacy max() guard, S09 incident).
	TokensPerCJKRune float64
	// TokensPerEmojiRune prices emoji-class runes (default 2.0). Public BPE
	// (cl100k/llama class) single-codepoint emoji land in the 1–3 tok band;
	// the fixture pins the hard invariant [1,3] alongside the default total.
	TokensPerEmojiRune float64
	// TokensPerOtherRune prices the residual class (default 0.5).
	TokensPerOtherRune float64
}

func (c SegmentedEstConfig) withDefaults() SegmentedEstConfig {
	if c.CharsPerLatinToken <= 0 {
		c.CharsPerLatinToken = 4.0
	}
	if c.TokensPerCJKRune <= 0 {
		c.TokensPerCJKRune = 1.0
	}
	if c.TokensPerEmojiRune <= 0 {
		c.TokensPerEmojiRune = 2.0
	}
	if c.TokensPerOtherRune <= 0 {
		c.TokensPerOtherRune = 0.5
	}
	return c
}

// estimateTokensSegmentedCounts prices a pre-classified breakdown. Zero total
// runes returns 0 (parity with estTokensFromChars); otherwise the estimate is
// ceil-rounded per total so rounding never under-counts a gate boundary.
func estimateTokensSegmentedCounts(c runeClassCounts, cfg SegmentedEstConfig) int {
	cfg = cfg.withDefaults()
	if c.totalRunes() == 0 {
		return 0
	}
	est := float64(c.ASCII)/cfg.CharsPerLatinToken +
		float64(c.CJK)*cfg.TokensPerCJKRune +
		float64(c.Emoji)*cfg.TokensPerEmojiRune +
		float64(c.Other)*cfg.TokensPerOtherRune
	return int(math.Ceil(est))
}

// promptEstTokensSegmented is the per-run segmented estimate over full
// messages (content + text parts) — the shadow candidate for promptEstTokens.
// Pure function: no I/O, no logging, no clock (estimator free path).
func promptEstTokensSegmented(msgs []trpcmodel.Message, cfg SegmentedEstConfig) int {
	total := runeClassCounts{}
	for _, m := range msgs {
		c := runeClassCountsMessage(m)
		total.ASCII += c.ASCII
		total.CJK += c.CJK
		total.Emoji += c.Emoji
		total.Other += c.Other
	}
	return estimateTokensSegmentedCounts(total, cfg)
}

// Calibration guardrails for RecordAuthoritative: provider-usage anchors are
// trusted inside sane bounds only — a corrupt anchor can never pin the Latin
// price outside [minCharsPerLatinToken, maxCharsPerLatinToken].
const (
	minCharsPerLatinToken = 1.0
	maxCharsPerLatinToken = 6.0
)

// SegmentedCalibrator converges the Latin price from provider-authoritative
// usage anchors (仿 llmcontext 双锚点先例): each anchor carries (authoritative
// tokens, the exact text priced); CJK/emoji/other arms hold at their grounded
// constants and the Latin arm absorbs the residual. Safe for concurrent use.
type SegmentedCalibrator struct {
	mu   sync.Mutex
	base SegmentedEstConfig
	cfg  SegmentedEstConfig // current (possibly calibrated) pricing
}

// NewSegmentedCalibrator starts from cfg (zero value = documented defaults).
func NewSegmentedCalibrator(cfg SegmentedEstConfig) *SegmentedCalibrator {
	cfg = cfg.withDefaults()
	return &SegmentedCalibrator{base: cfg, cfg: cfg}
}

// RecordAuthoritative re-derives the Latin chars/token from one anchor.
// Anchors whose residual Latin tokens are non-positive (e.g. provider counted
// fewer tokens than the CJK floor alone) keep the previous rate — a single
// bad anchor can never poison the estimator.
func (s *SegmentedCalibrator) RecordAuthoritative(tokens int, text string) {
	if tokens <= 0 || text == "" {
		return
	}
	c := classifyRunes(text)
	s.mu.Lock()
	defer s.mu.Unlock()
	residual := float64(tokens) -
		float64(c.CJK)*s.cfg.TokensPerCJKRune -
		float64(c.Emoji)*s.cfg.TokensPerEmojiRune -
		float64(c.Other)*s.cfg.TokensPerOtherRune
	if c.ASCII <= 0 || residual <= 0 {
		return
	}
	rate := float64(c.ASCII) / residual
	rate = math.Max(minCharsPerLatinToken, math.Min(maxCharsPerLatinToken, rate))
	s.cfg.CharsPerLatinToken = rate
}

// Config returns the current pricing (calibrated if anchors were recorded).
func (s *SegmentedCalibrator) Config() SegmentedEstConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// Estimate prices msgs with the current pricing.
func (s *SegmentedCalibrator) Estimate(msgs []trpcmodel.Message) int {
	return promptEstTokensSegmented(msgs, s.Config())
}

// promptEstShadowStepID pins the flowlog field name (approval note P0-3①:
// fixed as "prompt.est_shadow" for the one-week comparison scripts).
const promptEstShadowStepID = "prompt.est_shadow"

// promptEstTokensShadow returns the LEGACY estimate (zero behavior change)
// and, when lg is non-nil, logs one comparison entry pairing it with the
// segmented candidate. Wiring into production call sites is batch B — until
// then only tests (and any early shadow consumers) reach this function.
func promptEstTokensShadow(msgs []trpcmodel.Message, cal *SegmentedCalibrator, lg loggateway.Logger) int {
	legacy := promptEstTokens(msgs)
	if lg == nil || cal == nil {
		return legacy
	}
	seg := cal.Estimate(msgs)
	lg.Info("prompt token estimate shadow comparison",
		loggateway.StepID(promptEstShadowStepID),
		loggateway.Int("legacy_est", legacy),
		loggateway.Int("segmented_est", seg),
		loggateway.Int("delta", seg-legacy))
	return legacy
}
