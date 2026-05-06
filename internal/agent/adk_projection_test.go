package agent

import (
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestAppendLLMTextFromEvent(t *testing.T) {
	var main, rsn strings.Builder
	ev := &session.Event{
		LLMResponse: model.LLMResponse{
			Content: genai.NewContentFromText("hello", genai.RoleModel),
		},
	}
	AppendLLMTextFromEvent(ev, &main, &rsn)
	if main.String() != "hello" {
		t.Fatalf("got %q", main.String())
	}
}
