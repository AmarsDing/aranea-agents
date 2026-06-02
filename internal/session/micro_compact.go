package session

import (
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
)

const microCompactMinAgeTurns = 2

type microCompactResult struct {
	summaryMarkdown string
	fromTurn        int
	toTurn          int
	didCompact      bool
}

func tryMicroCompact(body []biz.ChatMessage, currentTurn int) microCompactResult {
	if len(body) == 0 {
		return microCompactResult{}
	}
	minTurn := currentTurn - microCompactMinAgeTurns
	cleared := 0
	for _, m := range body {
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if r == "tool" && m.TurnNumber <= minTurn && len(m.ContentMarkdown) > 200 {
			cleared++
		}
	}
	if cleared == 0 {
		return microCompactResult{}
	}
	from := body[0].TurnNumber
	to := body[len(body)-1].TurnNumber
	return microCompactResult{
		summaryMarkdown: "[MicroCompact: " + strconv.Itoa(cleared) + " tool result(s) from turns " + strconv.Itoa(from) + "–" + strconv.Itoa(to) + " cleared]",
		fromTurn:        from,
		toTurn:          to,
		didCompact:      true,
	}
}
