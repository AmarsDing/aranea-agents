package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	worldStateHashStateKey = "aranea:world_state_hash"
	worldStateCueStateKey  = "aranea:world_state_cue"

	worldStateDiffOpen  = "<world_state_diff>"
	worldStateDiffClose = "</world_state_diff>"
	worldStateOpen      = "<world_state>"
	worldStateClose     = "</world_state>"
)

const (
	worldStateKindFull      = "full"
	worldStateKindDiff      = "diff"
	worldStateKindUnchanged = "unchanged"
)

// resolveWorldStateCue turns a freshly built dynamic cue into the text that
// should be appended this model call (E7). The first call in an invocation
// sends the full cue; later calls send a line-level diff or nothing when
// the fingerprint is unchanged. State is stored on the invocation so a
// mid-turn compact (E7b) can re-inject the last known snapshot.
func resolveWorldStateCue(ctx context.Context, cue string) (payload, kind string) {
	cue = strings.TrimSpace(cue)
	if cue == "" {
		return "", worldStateKindUnchanged
	}
	hash := hashWorldStateCue(cue)
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return cue, worldStateKindFull
	}
	prevHash, _ := inv.GetState(worldStateHashStateKey)
	prevCue, _ := inv.GetState(worldStateCueStateKey)
	inv.SetState(worldStateHashStateKey, hash)
	inv.SetState(worldStateCueStateKey, cue)
	if prev, _ := prevHash.(string); prev == hash {
		return "", worldStateKindUnchanged
	}
	if prev, ok := prevCue.(string); ok {
		if diff := renderWorldStateDiff(prev, cue); diff != "" {
			return diff, worldStateKindDiff
		}
	}
	return cue, worldStateKindFull
}

func lastWorldStateCue(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	raw, _ := inv.GetState(worldStateCueStateKey)
	s, _ := raw.(string)
	return strings.TrimSpace(s)
}

func hashWorldStateCue(cue string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(cue)))
	return hex.EncodeToString(sum[:8])
}

// renderWorldStateDiff emits added/removed bullet lines. Empty when the
// normalized line sets match (caller then treats it as a first full send
// only if there was no previous cue).
func renderWorldStateDiff(prev, next string) string {
	prev = strings.TrimSpace(prev)
	if prev == "" {
		return ""
	}
	oldLines := worldStateLineSet(prev)
	newLines := worldStateLineSet(next)
	var added, removed []string
	for _, line := range splitWorldStateLines(next) {
		if !oldLines[line] {
			added = append(added, "+ "+line)
		}
	}
	for _, line := range splitWorldStateLines(prev) {
		if !newLines[line] {
			removed = append(removed, "- "+line)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(worldStateDiffOpen)
	b.WriteByte('\n')
	for _, line := range removed {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	for _, line := range added {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(worldStateDiffClose)
	return b.String()
}

func splitWorldStateLines(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func worldStateLineSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, line := range splitWorldStateLines(s) {
		out[line] = true
	}
	return out
}

func wrapWorldStateSnapshot(cue string) string {
	cue = strings.TrimSpace(cue)
	if cue == "" {
		return ""
	}
	return worldStateOpen + "\n" + cue + "\n" + worldStateClose
}

func messagesHaveWorldState(msgs []trpcmodel.Message) bool {
	for _, m := range msgs {
		if !isDynamicCueMessage(m) && m.Role != trpcmodel.RoleSystem && m.Role != trpcmodel.RoleUser {
			continue
		}
		if strings.Contains(m.Content, worldStateOpen) ||
			strings.Contains(m.Content, worldStateDiffOpen) ||
			strings.Contains(m.Content, "Effective tool keys") {
			return true
		}
	}
	return false
}

// insertBeforeLastUserMessage places msg immediately before the last real
// user turn (not a dynamic-cue sentinel). Codex mid-turn compact uses
// BeforeLastUserMessage so the model sees current world state next to the
// latest ask. If no real user message remains, msg is appended.
func insertBeforeLastUserMessage(msgs []trpcmodel.Message, msg trpcmodel.Message) []trpcmodel.Message {
	idx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == trpcmodel.RoleUser && !isDynamicCueMessage(msgs[i]) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return append(msgs, msg)
	}
	out := make([]trpcmodel.Message, 0, len(msgs)+1)
	out = append(out, msgs[:idx]...)
	out = append(out, msg)
	out = append(out, msgs[idx:]...)
	return out
}

// reinjectWorldStateAfterCompact restores the last dynamic snapshot when
// emergency truncation dropped history or tail cues (E7b).
func reinjectWorldStateAfterCompact(ctx context.Context, msgs []trpcmodel.Message) []trpcmodel.Message {
	cue := lastWorldStateCue(ctx)
	if cue == "" || messagesHaveWorldState(msgs) {
		return msgs
	}
	return insertBeforeLastUserMessage(msgs, asDynamicCue(wrapWorldStateSnapshot(cue)))
}
