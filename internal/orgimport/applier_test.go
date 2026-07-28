package orgimport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recordedRequest captures one inbound HTTP call for assertion.
type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

// newFakeServer starts an httptest server whose handler dispatches per path.
// handler receives the parsed JSON body (nil for GET) and returns (status, responseBody).
func newFakeServer(t *testing.T, handler func(method, path string, body map[string]any) (int, any)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil && r.ContentLength != 0 {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
		status, resp := handler(r.Method, r.URL.Path, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if resp != nil {
			raw, _ := json.Marshal(resp)
			_, _ = w.Write(raw)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func newTestApplier(apiURL string) *Applier {
	return NewApplier(ApplyOptions{APIURL: apiURL})
}

// ─── Category (organization node) ────────────────────────────────────────────

func TestApplier_UpsertCategory_CreateMapsToOrganization(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/organization":
			return 200, map[string]any{"items": []any{}}
		case method == http.MethodPost && path == "/v1/organization":
			return 200, map[string]any{"id": "org-1"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	result := &ApplyResult{}

	id, err := a.upsertCategory("quant", "量化", "desc", "", "industry", result)
	if err != nil {
		t.Fatalf("upsertCategory: %v", err)
	}
	if id != "org-1" || result.Created != 1 {
		t.Fatalf("id=%q created=%d, want org-1/1", id, result.Created)
	}
	// Find the POST and assert payload shape.
	var post *recordedRequest
	for i := range *reqs {
		if (*reqs)[i].Method == http.MethodPost {
			post = &(*reqs)[i]
		}
	}
	if post == nil {
		t.Fatal("no POST recorded")
	}
	if post.Body["org_key"] != "quant" {
		t.Errorf("org_key = %v, want quant", post.Body["org_key"])
	}
	if post.Body["level"] != "company" {
		t.Errorf("level = %v, want company (industry mapped)", post.Body["level"])
	}
	if _, hasKey := post.Body["key"]; hasKey {
		t.Error("payload must not contain legacy 'key' field")
	}
}

func TestApplier_UpsertCategory_UpdateUsesPatch(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/organization":
			return 200, map[string]any{"items": []any{
				map[string]any{"id": "org-9", "orgKey": "quant"},
			}}
		case method == http.MethodPatch && path == "/v1/organization/org-9":
			return 200, map[string]any{"id": "org-9"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	result := &ApplyResult{}

	id, err := a.upsertCategory("quant", "量化", "", "", "industry", result)
	if err != nil {
		t.Fatalf("upsertCategory: %v", err)
	}
	if id != "org-9" || result.Updated != 1 {
		t.Fatalf("id=%q updated=%d, want org-9/1", id, result.Updated)
	}
	for _, r := range *reqs {
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			t.Errorf("unexpected %s %s: update must use PATCH", r.Method, r.Path)
		}
	}
}

// ─── Agent ───────────────────────────────────────────────────────────────────

// Regression: ListAgents has no agent_key filter; the first item may be an
// unrelated agent. Lookup must exact-match agentKey, never return items[0].
func TestApplier_UpsertAgent_ExactMatchNotFirstItem(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/agents":
			return 200, map[string]any{"items": []any{
				map[string]any{"id": "ag-other", "agentKey": "someone_else"},
				map[string]any{"id": "ag-target", "agentKey": "quant_lead"},
			}}
		case method == http.MethodPatch && path == "/v1/agents/ag-target":
			return 200, map[string]any{"id": "ag-target"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	result := &ApplyResult{}
	spec := &Spec{}

	err := a.upsertAgent(AgentSpec{Key: "quant_lead", DisplayName: "量化主管", Provider: "openai", Model: "gpt-x"}, spec, result)
	if err != nil {
		t.Fatalf("upsertAgent: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated=%d, want 1", result.Updated)
	}
	for _, r := range *reqs {
		if r.Method == http.MethodPatch && r.Path != "/v1/agents/ag-target" {
			t.Errorf("PATCH hit %s, want /v1/agents/ag-target", r.Path)
		}
		if r.Method == http.MethodPut {
			t.Errorf("unexpected PUT %s: update must use PATCH", r.Path)
		}
	}
	if got := a.keyToID["agent:quant_lead"]; got != "ag-target" {
		t.Errorf("keyToID = %q, want ag-target", got)
	}
}

func TestApplier_UpsertAgent_CreateWhenAbsent(t *testing.T) {
	srv, _ := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/agents":
			return 200, map[string]any{"items": []any{
				map[string]any{"id": "ag-other", "agentKey": "someone_else"},
			}}
		case method == http.MethodPost && path == "/v1/agents":
			return 200, map[string]any{"id": "ag-new"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	result := &ApplyResult{}

	err := a.upsertAgent(AgentSpec{Key: "new_agent", DisplayName: "新", Provider: "openai", Model: "gpt-x"}, &Spec{}, result)
	if err != nil {
		t.Fatalf("upsertAgent: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("created=%d, want 1", result.Created)
	}
}

// ─── Team ────────────────────────────────────────────────────────────────────

func TestApplier_UpsertTeam_PayloadShape(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/teams":
			return 200, map[string]any{"items": []any{}}
		case method == http.MethodPost && path == "/v1/teams":
			return 200, map[string]any{"id": "tm-1"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	a.keyToID["agent:quant_lead"] = "ag-target"
	result := &ApplyResult{}

	err := a.upsertTeam(TeamSpec{
		Key:  "quant_team",
		Name: "量化团队",
		Members: []MemberSpec{
			{AgentKey: "quant_lead", Role: "orchestrator"},
		},
	}, result)
	if err != nil {
		t.Fatalf("upsertTeam: %v", err)
	}
	var post *recordedRequest
	for i := range *reqs {
		if (*reqs)[i].Method == http.MethodPost {
			post = &(*reqs)[i]
		}
	}
	if post == nil {
		t.Fatal("no POST recorded")
	}
	if post.Body["team_key"] != "quant_team" {
		t.Errorf("team_key = %v, want quant_team", post.Body["team_key"])
	}
	if post.Body["display_name"] != "量化团队" {
		t.Errorf("display_name = %v", post.Body["display_name"])
	}
	defRaw, ok := post.Body["definition_json"].(string)
	if !ok {
		t.Fatalf("definition_json missing or not a string: %v", post.Body)
	}
	var def struct {
		Version int    `json:"version"`
		Mode    string `json:"mode"`
		Members []struct {
			AgentID string `json:"agent_id"`
			Role    string `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(defRaw), &def); err != nil {
		t.Fatalf("definition_json not valid JSON: %v", err)
	}
	if def.Version != 2 || def.Mode != "sequential" {
		t.Errorf("def = %+v, want version=2 mode=sequential", def)
	}
	if len(def.Members) != 1 || def.Members[0].AgentID != "ag-target" || def.Members[0].Role != "orchestrator" {
		t.Errorf("members = %+v", def.Members)
	}
	if _, hasMembers := post.Body["members"]; hasMembers {
		t.Error("payload must not contain top-level 'members'; members belong in definition_json")
	}
}

func TestApplier_UpsertTeam_ExactMatchPatch(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/teams":
			return 200, map[string]any{"items": []any{
				map[string]any{"id": "tm-other", "teamKey": "other_team"},
				map[string]any{"id": "tm-target", "teamKey": "quant_team"},
			}}
		case method == http.MethodPatch && path == "/v1/teams/tm-target":
			return 200, map[string]any{"id": "tm-target"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	a.keyToID["agent:quant_lead"] = "ag-target"
	result := &ApplyResult{}

	err := a.upsertTeam(TeamSpec{Key: "quant_team", Name: "量化团队"}, result)
	if err != nil {
		t.Fatalf("upsertTeam: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("updated=%d, want 1", result.Updated)
	}
	for _, r := range *reqs {
		if r.Method == http.MethodPatch && r.Path != "/v1/teams/tm-target" {
			t.Errorf("PATCH hit %s, want /v1/teams/tm-target", r.Path)
		}
		if r.Method == http.MethodPut {
			t.Errorf("unexpected PUT %s: update must use PATCH", r.Path)
		}
	}
}

// ─── Dry run ─────────────────────────────────────────────────────────────────

func TestApplier_DryRunSkipsAllWrites(t *testing.T) {
	srv, reqs := newFakeServer(t, func(_, path string, _ map[string]any) (int, any) {
		return 500, map[string]any{"message": "should not be called: " + path}
	})
	a := NewApplier(ApplyOptions{APIURL: srv.URL, DryRun: true})
	spec := &Spec{
		Spec: SpecBody{
			Companies: []OrganizationSpec{{Key: "ind", Name: "行业"}},
			Agents:    []AgentSpec{{Key: "ag", DisplayName: "代理"}},
			Teams:     []TeamSpec{{Key: "tm", Name: "团队"}},
		},
	}
	result, err := a.Apply(spec)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(*reqs) != 0 {
		t.Fatalf("dry-run made %d HTTP calls, want 0", len(*reqs))
	}
	if result.Skipped == 0 {
		t.Fatal("dry-run should count skipped items")
	}
}

// ─── Team member fallback ────────────────────────────────────────────────────

func TestApplier_UpsertTeam_MemberFallbackLookup(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/agents":
			return 200, map[string]any{"items": []any{
				map[string]any{"id": "ag-existing", "agentKey": "quant_lead"},
			}}
		case method == http.MethodGet && path == "/v1/teams":
			return 200, map[string]any{"items": []any{}}
		case method == http.MethodPost && path == "/v1/teams":
			return 200, map[string]any{"id": "tm-1"}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL) // keyToID 为空：成员不在本次导入范围内
	result := &ApplyResult{}

	err := a.upsertTeam(TeamSpec{
		Key:  "quant_team",
		Name: "量化团队",
		Members: []MemberSpec{
			{AgentKey: "quant_lead", Role: "orchestrator"},
		},
	}, result)
	if err != nil {
		t.Fatalf("upsertTeam: %v", err)
	}
	var def string
	for _, r := range *reqs {
		if r.Method == http.MethodPost && r.Path == "/v1/teams" {
			def, _ = r.Body["definition_json"].(string)
		}
	}
	if !strings.Contains(def, `"agent_id":"ag-existing"`) {
		t.Errorf("definition_json should carry fallback-resolved agent_id, got %s", def)
	}
}

func TestApplier_UpsertTeam_MemberNotFoundFailsTeam(t *testing.T) {
	srv, reqs := newFakeServer(t, func(method, path string, _ map[string]any) (int, any) {
		switch {
		case method == http.MethodGet && path == "/v1/agents":
			return 200, map[string]any{"items": []any{}}
		case method == http.MethodGet && path == "/v1/teams":
			return 200, map[string]any{"items": []any{}}
		}
		return 404, map[string]any{"message": "unexpected " + method + " " + path}
	})
	a := newTestApplier(srv.URL)
	result := &ApplyResult{}

	err := a.upsertTeam(TeamSpec{
		Key:     "quant_team",
		Name:    "量化团队",
		Members: []MemberSpec{{AgentKey: "ghost", Role: "orchestrator"}},
	}, result)
	if err == nil {
		t.Fatal("upsertTeam should fail when member agent cannot be resolved")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should name the missing member key, got %v", err)
	}
	for _, r := range *reqs {
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			t.Errorf("team must not be written on member resolution failure, got %s %s", r.Method, r.Path)
		}
	}
}
