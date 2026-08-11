package wechatilink

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
)

type mockInboundHandler struct {
	onEvent func(ev port.InboundEvent)
}

func (m *mockInboundHandler) ProcessInbound(_ context.Context, _ biz.Channel, ev port.InboundEvent) error {
	if m.onEvent != nil {
		m.onEvent(ev)
	}
	return nil
}

func testLookup(baseURL string) runtime.CredentialLookup {
	return func(_ context.Context, _ []biz.ChannelCredential, key string) (string, error) {
		if key == "baseurl" {
			return baseURL, nil
		}
		return "tk", nil
	}
}

func TestPollingReceivesMessage(t *testing.T) {
	stateDir = t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getupdates" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret": 0,
			"msgs": []map[string]any{
				{
					"message_id":    100,
					"from_user_id":  "user@im.wechat",
					"message_type":  MessageTypeUser,
					"message_state": MessageStateNew,
					"item_list":     []map[string]any{{"type": ItemTypeText, "text_item": map[string]any{"text": "hi"}}},
					"context_token": "ctx-1",
					"session_id":    "s1",
				},
			},
			"get_updates_buf": "buf_v2",
		})
	}))
	defer server.Close()

	received := make(chan port.InboundEvent, 1)
	handler := &mockInboundHandler{onEvent: func(ev port.InboundEvent) {
		received <- ev
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := biz.Channel{ID: "ch-poll", ConfigJSON: `{"type":"wechat_ilink"}`, Enabled: true}

	done := make(chan error, 1)
	go func() {
		done <- RunPolling(ctx, ch, nil, testLookup(server.URL), handler, loggateway.NewNoop())
	}()

	select {
	case ev := <-received:
		if ev.Text != "hi" {
			t.Errorf("text want hi, got %s", ev.Text)
		}
		if ev.PlatformType != "wechat_ilink" {
			t.Errorf("platform want wechat_ilink, got %s", ev.PlatformType)
		}
		if ev.OutboundMeta[port.MetaContextToken] != "ctx-1" {
			t.Errorf("context_token not propagated: %v", ev.OutboundMeta)
		}
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("no message received within 5s")
	}

	<-done
	// 游标与 context_token 应已持久化
	state, err := readState("ch-poll")
	if err != nil {
		t.Fatal(err)
	}
	if state.GetUpdatesBuf != "buf_v2" {
		t.Errorf("buf want buf_v2, got %s", state.GetUpdatesBuf)
	}
	if state.ContextTokens["user@im.wechat"] != "ctx-1" {
		t.Errorf("context token not persisted: %v", state.ContextTokens)
	}
}

func TestSessionExpiredRecovery(t *testing.T) {
	stateDir = t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret": 0, "errcode": -14, "errmsg": "session expired",
		})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := biz.Channel{ID: "ch-exp", ConfigJSON: `{"type":"wechat_ilink"}`, Enabled: true}

	err := RunPolling(ctx, ch, nil, testLookup(server.URL), &mockInboundHandler{}, loggateway.NewNoop())
	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("want ErrSessionExpired, got %v", err)
	}

	state, rerr := readState("ch-exp")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if state.LoginStatus != "expired" {
		t.Errorf("login status want expired, got %s", state.LoginStatus)
	}
}

func TestPollingSkipsBotEcho(t *testing.T) {
	stateDir = t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret": 0,
			"msgs": []map[string]any{
				{
					"message_id":    200,
					"from_user_id":  "bot-self",
					"message_type":  MessageTypeBot, // bot 回声，必须跳过
					"message_state": MessageStateFinish,
					"item_list":     []map[string]any{{"type": ItemTypeText, "text_item": map[string]any{"text": "echo"}}},
				},
			},
			"get_updates_buf": "buf_v3",
		})
	}))
	defer server.Close()

	var handled atomic.Int32
	handler := &mockInboundHandler{onEvent: func(ev port.InboundEvent) {
		handled.Add(1)
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	ch := biz.Channel{ID: "ch-echo", ConfigJSON: `{"type":"wechat_ilink"}`, Enabled: true}
	_ = RunPolling(ctx, ch, nil, testLookup(server.URL), handler, loggateway.NewNoop())

	if handled.Load() != 0 {
		t.Errorf("bot echo should be skipped, handled %d", handled.Load())
	}
}
