package outbound

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestMessageTool_Call_NotConfigured(t *testing.T) {
	var mt *MessageTool
	if _, err := mt.Call(context.Background(), []byte(`{"text":"hi"}`)); err == nil {
		t.Fatal("nil tool must error")
	}
	mt = NewMessageTool(nil)
	if _, err := mt.Call(context.Background(), []byte(`{"text":"hi"}`)); err == nil {
		t.Fatal("nil router must error")
	}
}

func TestMessageTool_Call_InvalidJSON(t *testing.T) {
	mt := NewMessageTool(NewRouter(loggateway.NewNoop()))
	if _, err := mt.Call(context.Background(), []byte(`{`)); err == nil {
		t.Fatal("invalid json must error")
	}
}

func TestMessageTool_Call_TextOrFilesRequired(t *testing.T) {
	mt := NewMessageTool(NewRouter(loggateway.NewNoop()))
	if _, err := mt.Call(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("empty payload must error")
	}
}

func TestMessageTool_Call_ResolveMissIsError(t *testing.T) {
	t.Cleanup(resetSessionResolvers)
	resetSessionResolvers()
	mt := NewMessageTool(NewRouter(loggateway.NewNoop()))
	_, err := mt.Call(context.Background(), []byte(`{"text":"hi"}`))
	if err == nil {
		t.Fatal("unresolved channel/target must be an observable error, not ok=false")
	}
}

func TestMessageTool_Call_UnsupportedChannel(t *testing.T) {
	mt := NewMessageTool(NewRouter(loggateway.NewNoop()))
	_, err := mt.Call(context.Background(), []byte(`{"text":"hi","channel":"nope","target":"x"}`))
	if err == nil {
		t.Fatal("unsupported channel must error")
	}
}

func TestMessageTool_Call_SendHit(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram"})
	mt := NewMessageTool(r)
	out, err := mt.Call(context.Background(), []byte(`{"text":"hello","channel":"telegram","target":"chat-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", out)
	}
	if m["ok"] != true || m["channel"] != "telegram" || m["target"] != "chat-1" {
		t.Fatalf("got %#v", m)
	}
}

func TestMessageTool_Call_SendError(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram", sendErr: errors.New("network")})
	mt := NewMessageTool(r)
	if _, err := mt.Call(context.Background(), []byte(`{"text":"hi","channel":"telegram","target":"c"}`)); err == nil {
		t.Fatal("sender error must surface")
	}
}

func TestMessageTool_Call_FilesOnTextOnlyChannel(t *testing.T) {
	r := NewRouter(loggateway.NewNoop())
	r.RegisterTextSender(&stubTextSender{id: "telegram"})
	mt := NewMessageTool(r)
	if _, err := mt.Call(context.Background(), []byte(`{"text":"hi","file":"/tmp/a.txt","channel":"telegram","target":"c"}`)); err == nil {
		t.Fatal("files on text-only channel must error")
	}
}

func TestMessageTool_Declaration(t *testing.T) {
	d := NewMessageTool(NewRouter(loggateway.NewNoop())).Declaration()
	if d == nil || d.Name != "message" {
		t.Fatalf("declaration=%+v", d)
	}
	raw, _ := json.Marshal(map[string]string{"text": "x"})
	if len(raw) == 0 {
		t.Fatal("sanity")
	}
}
