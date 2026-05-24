package lark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReactionController_AddRemove(t *testing.T) {
	var addBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/auth/v3/tenant_access_token"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":                0,
				"tenant_access_token": "tok",
				"expire":              7200,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reactions"):
			addBody, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0,
				"data": map[string]string{"reaction_id": "reaction_abc"},
			})
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/reactions/reaction_abc"):
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := testAPIBase
	testAPIBase = func(string) string { return srv.URL }
	defer func() { testAPIBase = orig }()

	rc := &ReactionController{
		Region:    RegionFeishu,
		AppID:     "app",
		AppSecret: "secret",
		HTTP:      srv.Client(),
	}
	reactionID, err := rc.Add(context.Background(), "om_msg")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if reactionID != "reaction_abc" {
		t.Fatalf("reaction_id: got %q", reactionID)
	}
	if !strings.Contains(string(addBody), "emoji_type") {
		t.Fatalf("add body missing emoji_type: %s", string(addBody))
	}
	if err := rc.Remove(context.Background(), "om_msg", reactionID); err != nil {
		t.Fatalf("remove: %v", err)
	}
}
