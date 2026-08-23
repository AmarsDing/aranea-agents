package main

import "testing"

func TestParseHealthOK(t *testing.T) {
	oursReady := `{"status":"ok","auth_mode":"jwt","cookie_name":"aranea_token","ws_path":"/v1/ws","deploy_env":"dev"}`
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"ours ready", 200, oursReady, true},
		{"foreign 200 (the 2026-08-23 phantom)", 200, `{"ok":true}`, false},
		{"foreign 200 empty body", 200, "", false},
		{"ours still starting", 503, `{"status":"starting","auth_mode":"jwt"}`, false},
		{"ours failed", 503, `{"status":"failed","reason":"migrations","auth_mode":"jwt"}`, false},
		{"foreign 404", 404, `not found`, false},
	}
	for _, c := range cases {
		if got := parseHealthOK(c.status, c.body); got != c.want {
			t.Errorf("%s: parseHealthOK(%d, %q) = %v, want %v", c.name, c.status, c.body, got, c.want)
		}
	}
}

func TestLooksLikeOurs(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"status":"starting","auth_mode":"jwt"}`, true},
		{`{"status":"failed","reason":"x","auth_mode":"bypass"}`, true},
		{`{"status":"ok","auth_mode":"jwt","ws_path":"/v1/ws"}`, true},
		{`{"ok":true}`, false},
		{"", false},
		{"<html>proxy error</html>", false},
	}
	for _, c := range cases {
		if got := looksLikeOurs(c.body); got != c.want {
			t.Errorf("looksLikeOurs(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}
