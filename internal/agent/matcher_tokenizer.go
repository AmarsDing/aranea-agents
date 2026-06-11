package agent

import (
	"strings"
	"unicode"
)

// ---------------------------------------------------------------------------
// UnicodeTokenizer — language-aware text tokenization for agent matching.
//
// Background
// ==========
// The legacy `tokenize` helper used `strings.Fields` and a hand-rolled
// punctuation strip, which only works for ASCII whitespace-separated text.
// Chinese / Japanese / Korean (CJK) sentences have no internal whitespace,
// so a sentence like "查询今天北京的天气" would be treated as a single
// "word" of length 9. That made role/keyword overlap scoring on CJK
// descriptions effectively random: either the whole string matched (false
// positive) or it didn't (false negative).
//
// UnicodeTokenizer instead walks the string rune by rune and emits a token
// for each maximal contiguous run of "letter-or-digit" code points in any
// script. CJK ideographs, kana, hangul, and Latin letters are all treated
// as letter runs. Mixed-language input like "GPT-4 模型 fine-tune" is
// split into ["gpt", "4", "模型", "fine", "tune"] so the model id and
// the Chinese descriptor can match independently.
//
// Design choices
// ==============
//   * Stateless: no allocations beyond the output slice. Safe for
//     concurrent use across goroutines.
//   * Lowercase-normalized: callers see a canonical form so "GPT" and
//     "gpt" do not produce different match keys.
//   * Min-length filter: 1-char tokens are dropped. CJK single-character
//     "words" carry almost no discriminative power in capability matching
//     and produce huge sets that drown the real signals.
//   * Configurable via TokenizerOptions for tests and downstream callers.
//
// This tokenizer replaces the previous `tokenize` helper in agent_matcher.go
// and is the single source of truth for tokenization used by the agent
// matching pipeline. The allocator layer (`tokenizeForSemantic` in
// agent_allocator_impl.go) has a similar but narrower surface; it is a
// near-term compat shim and will be migrated in a follow-up.
// ---------------------------------------------------------------------------

// TokenizerOptions configures UnicodeTokenizer behaviour. The zero value
// is the default: lowercase, min-length 1, ASCII + CJK runs emitted.
type TokenizerOptions struct {
	// MinTokenLen drops tokens shorter than this. 0 disables the filter
	// (kept for tests that want to inspect the raw stream).
	MinTokenLen int
	// Lowercase controls whether the output is lowercased. Default true.
	// Tests may set false to inspect the original casing.
	Lowercase bool
	// DropDigits drops runs of pure digits (e.g. "4" in "GPT-4").
	// Default false — model versions and numeric tokens are useful for
	// capability matching ("claude-opus-4" vs "gpt-5").
	DropDigits bool
}

// DefaultTokenizerOptions is the production default.
func DefaultTokenizerOptions() TokenizerOptions {
	return TokenizerOptions{
		MinTokenLen: 1,
		Lowercase:   true,
		DropDigits:  false,
	}
}

// UnicodeTokenizer splits text into canonical tokens for matching.
type UnicodeTokenizer struct {
	opts TokenizerOptions
}

// NewUnicodeTokenizer constructs a tokenizer with the given options.
// Use DefaultTokenizerOptions() for the production preset.
func NewUnicodeTokenizer(opts TokenizerOptions) *UnicodeTokenizer {
	return &UnicodeTokenizer{opts: opts}
}

// Tokenize returns the list of tokens for s. The returned slice is
// safe for the caller to mutate; the tokenizer holds no reference.
func (t *UnicodeTokenizer) Tokenize(s string) []string {
	if s == "" {
		return nil
	}
	if t.opts.Lowercase {
		s = strings.ToLower(s)
	}

	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		if t.opts.MinTokenLen > 0 && len([]rune(tok)) < t.opts.MinTokenLen {
			return
		}
		if t.opts.DropDigits && isAllDigits(tok) {
			return
		}
		out = append(out, tok)
	}
	for _, r := range s {
		if isTokenRune(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// Tokenize is a package-level convenience that uses the default options.
// Internal callers should prefer NewUnicodeTokenizer so they can be tuned
// (e.g. by tests) without touching every call site.
func Tokenize(s string) []string {
	return NewUnicodeTokenizer(DefaultTokenizerOptions()).Tokenize(s)
}

// isTokenRune reports whether r should be part of a token. We treat any
// unicode letter, any unicode digit, and the underscore as token-bearing.
// ASCII punctuation, whitespace, and CJK punctuation are token boundaries.
func isTokenRune(r rune) bool {
	switch {
	case unicode.IsLetter(r):
		return true
	case unicode.IsDigit(r):
		return true
	case r == '_':
		return true
	default:
		return false
	}
}

// isAllDigits reports whether s is non-empty and contains only digits
// (after lowercasing). Used for the optional drop-pure-digits filter.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
