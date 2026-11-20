package trpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
)

type recordingRunner struct {
	mu      sync.Mutex
	userIDs []string
}

func (r *recordingRunner) Run(_ context.Context, userID, _ string, _ model.Message, _ ...agent.RunOption) (<-chan *event.Event, error) {
	r.mu.Lock()
	r.userIDs = append(r.userIDs, userID)
	r.mu.Unlock()
	ch := make(chan *event.Event, 1)
	ch <- event.NewResponseEvent("inv", "agent", &model.Response{
		ID: "resp", Object: model.ObjectTypeChatCompletion, Created: time.Now().Unix(), Done: true,
		Choices: []model.Choice{{Message: model.Message{Role: model.RoleAssistant, Content: "ok"}}},
	})
	close(ch)
	return ch, nil
}

func (r *recordingRunner) Close() error { return nil }

// 公开端点免登录：未经舰队签名的 X-User-ID 不能被当成真实身份（否则可冒用他人的会话与记忆）。
func TestBuildA2AEndpointServer_StripsUnsignedUserID(t *testing.T) {
	t.Setenv("ARANEA_A2A_IDENTITY_HMAC_SECRET", "")
	runner := &recordingRunner{}
	h, err := BuildA2AEndpointServer(runner, biz.Agent{AgentKey: "probe"}, biz.A2AAgentCard{}, "http://placeholder.local", false)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"message/send","params":{"message":{"kind":"message","role":"user","messageId":"m1","contextId":"c1","parts":[{"kind":"text","text":"hi"}]}}}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "victim-user")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()

	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.userIDs) != 1 {
		t.Fatalf("runner calls = %d (status %d), want 1", len(runner.userIDs), resp.StatusCode)
	}
	if runner.userIDs[0] == "victim-user" {
		t.Fatalf("unsigned X-User-ID was trusted as the caller identity")
	}
}
