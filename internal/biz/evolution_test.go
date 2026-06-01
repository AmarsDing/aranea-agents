package biz_test

import (
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

func TestReplaceOrAppendPersona_AppendNew(t *testing.T) {
	body := "# IDENTITY\n\nSome intro text."
	result := biz.ReplaceOrAppendPersona(body, "I am a helpful assistant.")
	if !strings.Contains(result, "## Persona") {
		t.Error("result should contain ## Persona heading")
	}
	if !strings.Contains(result, "I am a helpful assistant.") {
		t.Error("result should contain persona content")
	}
	if strings.Contains(result, "## Persona\n\nSome intro text.") {
		t.Error("old body should not be inside Persona section")
	}
}

func TestReplaceOrAppendPersona_ReplaceExisting(t *testing.T) {
	body := "# IDENTITY\n\n## Persona\n\nOld persona content.\n\n## Other\n\nOther section."
	result := biz.ReplaceOrAppendPersona(body, "New persona content.")
	if !strings.Contains(result, "New persona content.") {
		t.Error("result should contain new persona content")
	}
	if strings.Contains(result, "Old persona content.") {
		t.Error("result should not contain old persona content")
	}
	if !strings.Contains(result, "## Other") {
		t.Error("result should preserve ## Other section")
	}
	if !strings.Contains(result, "Other section.") {
		t.Error("result should preserve Other section content")
	}
}

func TestReplaceOrAppendPersona_PersonaAtEnd(t *testing.T) {
	body := "# IDENTITY\n\nIntro.\n\n## Persona\n\nOld persona."
	result := biz.ReplaceOrAppendPersona(body, "Updated persona.")
	if !strings.Contains(result, "Updated persona.") {
		t.Error("result should contain updated persona")
	}
	if strings.Contains(result, "Old persona.") {
		t.Error("result should not contain old persona")
	}
}

func TestReplaceOrAppendPersona_EmptyBody(t *testing.T) {
	result := biz.ReplaceOrAppendPersona("", "First persona.")
	if !strings.Contains(result, "## Persona") {
		t.Error("result should contain ## Persona heading")
	}
	if !strings.Contains(result, "First persona.") {
		t.Error("result should contain persona content")
	}
}

func TestReplaceOrAppendPersona_TrailingWhitespace(t *testing.T) {
	body := "# IDENTITY\n\nIntro.\n\n## Persona\n\nOld.\n\n"
	result := biz.ReplaceOrAppendPersona(body, "New.")
	if !strings.Contains(result, "New.") {
		t.Error("result should contain new persona")
	}
}

func TestTimeRangeToSince(t *testing.T) {
	tests := []struct {
		name      string
		timeRange string
		wantDays  int
	}{
		{"7d", "7d", 7},
		{"30d", "30d", 30},
		{"90d", "90d", 90},
		{"empty defaults to 30d", "", 30},
		{"unknown defaults to 30d", "1y", 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			since := biz.TimeRangeToSince(tt.timeRange)
			now := time.Now()
			expected := now.AddDate(0, 0, -tt.wantDays)
			diff := since.Sub(expected)
			if diff < -time.Second || diff > time.Second {
				t.Errorf("TimeRangeToSince(%q) = %v, want approximately %v (diff=%v)", tt.timeRange, since, expected, diff)
			}
		})
	}
}
