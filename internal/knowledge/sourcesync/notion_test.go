// notion_test.go —— G-38 W6 notion walker 钉阵（httptest 假服务器面）。
//
// 钉点：search 分页全量出席（多页/dup 去重/归档跳过/查询透传）、429 Retry-After
// 尊重（记账不真睡＋轮内上限）、cursor-invalid 清游标重启一次（二次失效 abort）、
// auth 归一（缺行 credential_missing/解密失败 credential_decrypt_failed/
// 401 auth_failed/env 方案）、Fetch 组装（retrieve 归档闸＋blocks 全分页渲染
// ＋512KiB 截断）、ConfirmDeleted 三态（404/archived=Confirmed；活性=
// StillExists；5xx/auth/429=Unknown）、external_id UUID 形状闸。

package sourcesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

const (
	notionTestPageID1 = "aaaaaaaa-1111-2222-3333-bbbbbbbbbbbb"
	notionTestPageID2 = "cccccccc-4444-5555-6666-dddddddddddd"
)

// ── 装配缝 ──

func newNotionTestWalker(t *testing.T, srv *httptest.Server, creds bizsync.CredentialRepo) (*NotionWalker, *[]time.Duration) {
	t.Helper()
	if creds == nil {
		creds = newMemCredentialRepo()
	}
	sleeps := &[]time.Duration{}
	w, err := NewNotionWalker(NotionWalkerDeps{
		Log:         loggateway.NewNoop(),
		Credentials: creds,
		Crypto:      biz.NewCredentialCrypto(nil, loggateway.NewNoop()),
		HTTP:        srv.Client(),
		BaseURL:     srv.URL,
		Sleep: func(ctx context.Context, d time.Duration) error {
			*sleeps = append(*sleeps, d)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return w, sleeps
}

func notionTestConnector(t *testing.T, configJSON string) bizsync.Connector {
	t.Helper()
	cfg := configJSON
	if strings.TrimSpace(cfg) == "" {
		cfg = `{}`
	}
	if _, err := bizsync.ParseNotionConfig(cfg); err != nil {
		t.Fatalf("connector config invalid: %v", err)
	}
	return bizsync.Connector{ID: "ss:conn:notion-1", Kind: bizsync.KindNotion, ConfigJSON: cfg}
}

// notionPageJSON 页行构造（search/retrieve 共用形状）。
func notionPageJSON(id, edited, title string, archived bool) string {
	return fmt.Sprintf(`{"id":%q,"archived":%v,"last_edited_time":%q,"properties":{"title":{"type":"title","title":[{"plain_text":%q}]}}}`,
		id, archived, edited, title)
}

func seedNotionEnvToken(t *testing.T, creds bizsync.CredentialRepo, connID, key string) {
	t.Helper()
	if key == "" {
		key = bizsync.DefaultNotionCredentialKey
	}
	if err := creds.UpsertCredential(context.Background(), bizsync.Credential{
		ID: "ss:cred:t1", ConnectorID: connID, CredentialKey: key,
		Status: "active", SecretRef: "env:NOTION_TEST_TOKEN",
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOTION_TEST_TOKEN", "secret_test_token")
}

// ── Walk 钉 ──

func TestNotionWalk_PaginationDedupAndArchived(t *testing.T) {
	var gotQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret_test_token" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Notion-Version") != notionAPIVersion {
			t.Errorf("notion version header = %q", r.Header.Get("Notion-Version"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if q, ok := body["query"]; ok {
			gotQueries = append(gotQueries, fmt.Sprint(q))
		}
		cursor, _ := body["start_cursor"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch cursor {
		case "":
			// 第一页：page1 + archived 页（跳过）+ dup page1（去重）。
			fmt.Fprintf(w, `{"results":[%s,%s,%s],"has_more":true,"next_cursor":"c2"}`,
				notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false),
				notionPageJSON("99999999-0000-0000-0000-888888888888", "2026-10-27T10:01:00Z", "Gone", true),
				notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false))
		case "c2":
			fmt.Fprintf(w, `{"results":[%s],"has_more":false,"next_cursor":null}`,
				notionPageJSON(notionTestPageID2, "2026-10-27T10:02:00Z", "Beta", false))
		default:
			t.Fatalf("unexpected cursor %q", cursor)
		}
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, `{"query":"roadmap"}`)
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	var seen []Snapshot
	cur, err := walker.Walk(context.Background(), conn, "", func(s Snapshot) error {
		seen = append(seen, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if cur != "" {
		t.Fatalf("notion cursor column must stay empty (as-built: no cross-run cursor), got %q", cur)
	}
	if len(seen) != 2 {
		t.Fatalf("seen = %d, want 2 (dup removed, archived skipped)", len(seen))
	}
	if seen[0].ExternalID != notionTestPageID1 || seen[0].Fingerprint != "2026-10-27T10:00:00Z" {
		t.Fatalf("snap[0] = %+v", seen[0])
	}
	if seen[0].DisplayPath != "Alpha" {
		t.Fatalf("display path = %q", seen[0].DisplayPath)
	}
	if seen[1].ExternalID != notionTestPageID2 {
		t.Fatalf("snap[1] = %+v", seen[1])
	}
	if len(gotQueries) != 2 || gotQueries[0] != "roadmap" {
		t.Fatalf("query passthrough: %v", gotQueries)
	}
}

func TestNotionWalk_RetryAfterRespectedAndCapped(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= 2 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[],"has_more":false,"next_cursor":null}`)
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, sleeps := newNotionTestWalker(t, srv, creds)

	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(*sleeps) != 2 {
		t.Fatalf("429 sleeps = %v", *sleeps)
	}
	for _, d := range *sleeps {
		if d != 3*time.Second {
			t.Fatalf("Retry-After not honored: got %v", d)
		}
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (2×429 then 200)", calls)
	}
}

func TestNotionWalk_RetryAfterCapBailsOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, sleeps := newNotionTestWalker(t, srv, creds)

	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "rate-limited") {
		t.Fatalf("want rate-limit bail, got %v", err)
	}
	if len(*sleeps) != notionMax429Waits {
		t.Fatalf("sleeps = %d, want cap %d", len(*sleeps), notionMax429Waits)
	}
	if IsAuthError(err) {
		t.Fatalf("429 exhaustion must not be auth class: %v", err)
	}
}

func TestNotionWalk_CursorInvalidRestartsOnceThenAborts(t *testing.T) {
	phase := 0 // 0=首趟半路 cursor 失效；1=重启成功
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		cursor, _ := body["start_cursor"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case phase == 0 && cursor == "":
			fmt.Fprintf(w, `{"results":[%s],"has_more":true,"next_cursor":"STALE"}`,
				notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false))
			phase = 1
		case phase == 1 && cursor == "STALE":
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"object":"error","status":400,"code":"validation_error","message":"start_cursor is invalid"}`)
			phase = 2
		case phase == 2 && cursor == "":
			// 重启全量：page1 须重新出席（首趟已 emit 的不丢失——出席是事实）。
			fmt.Fprintf(w, `{"results":[%s,%s],"has_more":false,"next_cursor":null}`,
				notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false),
				notionPageJSON(notionTestPageID2, "2026-10-27T10:02:00Z", "Beta", false))
			phase = 3
		default:
			t.Fatalf("unexpected state phase=%d cursor=%q", phase, cursor)
		}
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	var seen []string
	_, err := walker.Walk(context.Background(), conn, "", func(s Snapshot) error {
		seen = append(seen, s.ExternalID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// 首趟 page1 + 重启 page1+page2（重启重出席幂等——runner UpsertItemSeen 幂等）。
	if len(seen) != 3 || seen[0] != notionTestPageID1 || seen[1] != notionTestPageID1 || seen[2] != notionTestPageID2 {
		t.Fatalf("restart attendance = %v", seen)
	}
}

func TestNotionWalk_CursorInvalidTwiceAborts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"object":"error","status":400,"code":"validation_error","message":"cursor expired"}`)
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	if err == nil {
		t.Fatal("second cursor-invalid must abort walk")
	}
}

func TestNotionWalk_EmitErrorStopsAndCursorFrozen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"results":[%s],"has_more":false,"next_cursor":null}`,
			notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false))
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	sentinel := fmt.Errorf("budget hit")
	cur, err := walker.Walk(context.Background(), conn, "prev-state", func(Snapshot) error { return sentinel })
	if err != sentinel {
		t.Fatalf("emit error must propagate: %v", err)
	}
	if cur != "prev-state" {
		t.Fatalf("cursor must stay frozen at prev, got %q", cur)
	}
}

// ── 凭据/auth 归一钉 ──

func TestNotionAuth_MissingCredentialRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected without credential")
	}))
	defer srv.Close()
	walker, _ := newNotionTestWalker(t, srv, newMemCredentialRepo())
	conn := notionTestConnector(t, "")
	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	code, ok := AuthCodeOf(err)
	if !ok || code != bizsync.ErrCodeCredentialMissing {
		t.Fatalf("want credential_missing, got %v (code=%q ok=%v)", err, code, ok)
	}
}

func TestNotionAuth_DecryptFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no HTTP call expected when decrypt fails")
	}))
	defer srv.Close()
	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	if err := creds.UpsertCredential(context.Background(), bizsync.Credential{
		ID: "ss:cred:bad", ConnectorID: conn.ID, CredentialKey: bizsync.DefaultNotionCredentialKey,
		Status: "active", SecretRef: "enc:not-a-real-envelope",
	}); err != nil {
		t.Fatal(err)
	}
	// Crypto 无 key（env 清空）→ 解密失败。
	walker, _ := newNotionTestWalker(t, srv, creds)
	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	code, ok := AuthCodeOf(err)
	if !ok || code != bizsync.ErrCodeCredentialDecryptFailed {
		t.Fatalf("want credential_decrypt_failed, got %v (code=%q ok=%v)", err, code, ok)
	}
}

func TestNotionAuth_401WalksToAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"object":"error","status":401,"code":"unauthorized","message":"bad token"}`)
	}))
	defer srv.Close()
	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)
	_, err := walker.Walk(context.Background(), conn, "", func(Snapshot) error { return nil })
	code, ok := AuthCodeOf(err)
	if !ok || code != bizsync.ErrCodeAuthFailed {
		t.Fatalf("want auth_failed, got %v (code=%q ok=%v)", err, code, ok)
	}
}

// ── Fetch / ConfirmDeleted 钉 ──

func TestNotionFetch_RendersMarkdownAcrossPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/pages/" + notionTestPageID1:
			fmt.Fprint(w, notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false))
		case "/v1/blocks/" + notionTestPageID1 + "/children":
			if r.URL.Query().Get("start_cursor") == "" {
				fmt.Fprint(w, `{"results":[
					{"type":"heading_1","heading_1":{"rich_text":[{"plain_text":"Roadmap"}]}},
					{"type":"paragraph","paragraph":{"rich_text":[{"plain_text":"hello notion"}]}},
					{"type":"code","code":{"language":"go","rich_text":[{"plain_text":"fmt.Println()"}]}},
					{"type":"bogus_future_type","bogus_future_type":{"rich_text":[{"plain_text":"skip me"}]}}
				],"has_more":true,"next_cursor":"b2"}`)
				return
			}
			fmt.Fprint(w, `{"results":[
				{"type":"to_do","to_do":{"checked":true,"rich_text":[{"plain_text":"ship W6"}]}}
			],"has_more":false,"next_cursor":null}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	p, err := walker.Fetch(context.Background(), conn, notionTestPageID1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Body.Close() }()
	if p.MIME != "text/markdown" {
		t.Fatalf("MIME = %q", p.MIME)
	}
	raw, _ := io.ReadAll(p.Body)
	body := string(raw)
	for _, want := range []string{"# Roadmap", "hello notion", "```go", "- [x] ship W6"} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "skip me") {
		t.Fatalf("unknown block type must be skipped:\n%s", body)
	}
	if p.SizeBytes != int64(len(raw)) {
		t.Fatalf("SizeBytes = %d, want %d", p.SizeBytes, len(raw))
	}
}

func TestNotionFetch_ArchivedPageRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Gone", true))
	}))
	defer srv.Close()
	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)
	_, err := walker.Fetch(context.Background(), conn, notionTestPageID1)
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("archived page must be fetch error: %v", err)
	}
}

func TestNotionConfirmDeleted_ThreeStates(t *testing.T) {
	var mu sync.Mutex
	mode := "active"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "active":
			fmt.Fprint(w, notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", false))
		case "archived":
			fmt.Fprint(w, notionPageJSON(notionTestPageID1, "2026-10-27T10:00:00Z", "Alpha", true))
		case "404":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"object":"error","status":404,"code":"object_not_found","message":"missing"}`)
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"object":"error","status":500,"code":"internal_server_error","message":"boom"}`)
		}
	}))
	defer srv.Close()
	creds := newMemCredentialRepo()
	conn := notionTestConnector(t, "")
	seedNotionEnvToken(t, creds, conn.ID, "")
	walker, _ := newNotionTestWalker(t, srv, creds)

	cases := []struct {
		mode string
		want bizsync.DeletionVerdict
	}{
		{"active", bizsync.VerdictStillExists},
		{"archived", bizsync.VerdictConfirmed},
		{"404", bizsync.VerdictConfirmed},
		{"500", bizsync.VerdictUnknown},
	}
	for _, tc := range cases {
		mu.Lock()
		mode = tc.mode
		mu.Unlock()
		got := walker.ConfirmDeleted(context.Background(), conn, notionTestPageID1)
		if got != tc.want {
			t.Fatalf("mode=%s: got %v, want %v", tc.mode, got, tc.want)
		}
	}
	// 非 UUID 形状 → Unknown（守卫生效）。
	if got := walker.ConfirmDeleted(context.Background(), conn, "../etc/passwd"); got != bizsync.VerdictUnknown {
		t.Fatalf("malformed id: got %v, want Unknown", got)
	}
}

// TestNotionConfig_AdminGate 管理面 kind 分派：notion config 走 ParseNotionConfig
// （secret-ish 键拒；credential_key 白名单豁免）。
func TestNotionConfig_AdminGate(t *testing.T) {
	if _, err := bizsync.ParseNotionConfig(`{"credential_key":"tok","query":"x"}`); err != nil {
		t.Fatalf("credential_key must be exempt from forbidden list: %v", err)
	}
	cfg, err := bizsync.ParseNotionConfig(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CredentialKey != bizsync.DefaultNotionCredentialKey {
		t.Fatalf("default credential key = %q", cfg.CredentialKey)
	}
	if _, err := bizsync.ParseNotionConfig(`{"api_token":"x"}`); err == nil {
		t.Fatal("secret-ish key must be rejected")
	}
	if _, err := bizsync.ParseNotionConfig(`{"credential_key":" bad "}`); err == nil {
		t.Fatal("invalid credential_key shape must be rejected")
	}
	// 出库 sanitize：credential_key 保活，api_token 剥离。
	out := bizsync.SanitizeConfigForExport(`{"credential_key":"tok","api_token":"x","query":"q"}`)
	if !strings.Contains(out, "credential_key") || strings.Contains(out, "api_token") {
		t.Fatalf("sanitized config = %s", out)
	}
}
