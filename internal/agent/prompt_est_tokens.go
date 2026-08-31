package agent

import (
	"unicode"

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
		if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			n++
		}
	}
	return n
}
