package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchOAuth2ClientCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-abc"})
	}))
	defer srv.Close()

	tok, err := fetchOAuth2ClientCredentials(context.Background(), mcpAuthConfigJSON{
		Type:         "oauth2_client_credentials",
		TokenURL:     srv.URL,
		ClientID:     "cid",
		ClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-abc" {
		t.Fatalf("token=%q", tok)
	}
}
