package memory

import (
	"regexp"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// Codex Phase2 consolidator contract (C3):
//   - no high-signal memories → skip the LLM write (no-op)
//   - never persist secrets / raw PII
//   - isolated: no tools, no network, no recursive spawn (prompt contract)

var (
	piiPlaceholderRe = regexp.MustCompile(`\[[a-z_]+\]`)
	lowSignalExact   = map[string]struct{}{
		"hi": {}, "hello": {}, "hey": {}, "thanks": {}, "thank you": {},
		"ok": {}, "okay": {}, "yes": {}, "no": {}, "lol": {},
		"嗯": {}, "好的": {}, "谢谢": {}, "你好": {},
	}
	highSignalTopics = map[string]struct{}{
		"core": {}, "preference": {}, "constraint": {}, "profile": {},
		"goal": {}, "instruction": {}, "decision": {}, "correction": {},
		"user_preference": {}, "agent_instruction": {},
	}
)

// prepareConsolidationMemories redacts secrets and drops low-signal / secret-only
// entries. Returns the sanitized set and whether any durable high-signal
// memory remains. Callers skip the LLM when ok is false.
func prepareConsolidationMemories(memories []*trpcmemory.Entry) (kept []*trpcmemory.Entry, ok bool) {
	for _, e := range memories {
		sanitized := sanitizeMemoryForConsolidate(e)
		if sanitized == nil || !isHighSignalMemory(sanitized) {
			continue
		}
		kept = append(kept, sanitized)
	}
	return kept, len(kept) > 0
}

func sanitizeMemoryForConsolidate(e *trpcmemory.Entry) *trpcmemory.Entry {
	if e == nil || e.Memory == nil {
		return nil
	}
	text := strings.TrimSpace(e.Memory.Memory)
	if text == "" {
		return nil
	}
	if scan := biz.ScanPII(text); scan.PIIFlag {
		text = strings.TrimSpace(scan.RedactedStatement)
	}
	if text == "" {
		return nil
	}
	cp := *e
	mem := *e.Memory
	mem.Memory = text
	cp.Memory = &mem
	return &cp
}

func isHighSignalMemory(e *trpcmemory.Entry) bool {
	if e == nil || e.Memory == nil {
		return false
	}
	for _, topic := range e.Memory.Topics {
		if _, hit := highSignalTopics[strings.ToLower(strings.TrimSpace(topic))]; hit {
			return !isLowSignalUtterance(e.Memory.Memory)
		}
	}
	return !isLowSignalUtterance(e.Memory.Memory)
}

func isLowSignalUtterance(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(".!。！?？~～", r)
	})
	stripped := strings.TrimSpace(piiPlaceholderRe.ReplaceAllString(s, ""))
	if stripped == "" {
		return true
	}
	key := strings.ToLower(stripped)
	if _, hit := lowSignalExact[key]; hit {
		return true
	}
	return len([]rune(stripped)) < 8
}

func operationContainsSecret(op ConsolidationOperation) bool {
	chunks := []string{op.MergedContent, op.Reflection}
	for _, u := range op.Updates {
		chunks = append(chunks, u.Key, u.Value)
	}
	for _, c := range chunks {
		if biz.ScanPII(c).PIIFlag {
			return true
		}
	}
	return false
}
