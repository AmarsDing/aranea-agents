package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestDeliverableArtifact_LegacyJSONDefaultsStateKey(t *testing.T) {
	raw := `{"st_1":{"summary":"结论","team_id":"t1","artifacts":[{"key":"article","title":"旧","size_chars":12}]}}`
	refs := ParseDeliverableRefs(raw)
	art := refs["st_1"].Artifacts[0]
	if art.ResolvedKind() != DeliverableArtifactKindStateKey {
		t.Fatalf("legacy artifact Kind empty must resolve to state_key, got %q", art.ResolvedKind())
	}
	if art.IsBulkPointer() {
		t.Fatal("legacy state_key must not be a bulk pointer")
	}
}

func TestDeliverableArtifact_ParseBulkFields(t *testing.T) {
	raw := `{"st_1":{"summary":"结论","team_id":"up1","artifacts":[{"key":"pack","kind":"workspace_rel","title":"包","rel_path":"report.zip","size_bytes":4096,"sha256":"abc","mime_type":"application/zip"}]}}`
	art := ParseDeliverableRefs(raw)["st_1"].Artifacts[0]
	if art.ResolvedKind() != DeliverableArtifactKindWorkspaceRel {
		t.Fatalf("kind=%q", art.Kind)
	}
	if art.RelPath != "report.zip" || art.SizeBytes != 4096 || art.SHA256 != "abc" {
		t.Fatalf("bulk fields: %+v", art)
	}
	if !art.IsBulkPointer() {
		t.Fatal("workspace_rel must be bulk")
	}
}

func TestSanitizeInboxRelPath(t *testing.T) {
	if got := sanitizeInboxRelPath("../etc/passwd"); got != "" {
		t.Fatalf("escape must reject, got %q", got)
	}
	if got := sanitizeInboxRelPath("/abs/file"); got != "" {
		t.Fatalf("absolute must reject, got %q", got)
	}
	if got := sanitizeInboxRelPath(`out\report.pdf`); got != "out/report.pdf" {
		t.Fatalf("backslash normalize, got %q", got)
	}
}

func TestBuildDeliverableArtifacts_WorkspaceRelFromPayload(t *testing.T) {
	arts := buildDeliverableArtifacts(map[string]any{
		"pack": map[string]any{
			"title":      "数据包",
			"rel_path":   "exports/data.csv",
			"size_bytes": float64(128),
			"mime_type":  "text/csv",
		},
	}, "")
	if len(arts) != 1 {
		t.Fatalf("got %+v", arts)
	}
	if arts[0].ResolvedKind() != DeliverableArtifactKindWorkspaceRel || arts[0].RelPath != "exports/data.csv" {
		t.Fatalf("got %+v", arts[0])
	}
}

func TestMarshalEnvelopeStructuredJSON_OmitsOversizedContent(t *testing.T) {
	body := strings.Repeat("长", MaxEnvelopeStructuredPayloadChars+1)
	state := map[string]any{
		"article": map[string]any{"title": "长文", "format": "markdown", "content": body},
	}
	arts := buildDeliverableArtifacts(state, "")
	raw := marshalEnvelopeStructuredJSON(state, arts)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatal(err)
	}
	payload := parsed["article"].(map[string]any)
	if _, ok := payload["content"]; ok {
		t.Fatal("oversized content must not be copied into StructuredJSON")
	}
	if payload["content_omitted"] != true {
		t.Fatalf("want content_omitted, got %+v", payload)
	}
}

func TestMarshalEnvelopeStructuredJSON_BulkOmitsBody(t *testing.T) {
	state := map[string]any{
		"pack": map[string]any{
			"title":    "zip",
			"rel_path": "out.zip",
			"content":  strings.Repeat("x", 50),
		},
	}
	arts := buildDeliverableArtifacts(state, "")
	raw := marshalEnvelopeStructuredJSON(state, arts)
	if strings.Contains(raw, strings.Repeat("x", 50)) {
		t.Fatalf("file body must not enter StructuredJSON: %s", raw)
	}
}

func TestInjectUpstreamDeliverables_BulkPrefixHasInventoryNotBody(t *testing.T) {
	teams := newDeliverableTeamRepo()
	secret := "THIS_FILE_BODY_MUST_NOT_APPEAR"
	up := Team{
		ID: "up-team", SpiritSessionID: "sp1", DagNodeID: "n1",
		DisplayName: "上游", Status: TeamStatusCompleted,
		DefinitionJSON: `{"members":[{"agent_id":"writer"}]}`,
		DeliverablesOutput: marshalJSONValue(map[string]any{
			"n1": DeliverableRef{
				Summary: "设计完成",
				TeamID:  "up-team",
				Artifacts: []DeliverableArtifact{{
					Key: "pack", Kind: DeliverableArtifactKindWorkspaceRel,
					Title: "设计稿", Format: "zip", RelPath: "design.zip", SizeBytes: 12,
				}},
				StructuredJSON: `{"pack":{"content":"` + secret + `"}}`,
			},
		}),
	}
	down := Team{
		ID: "dn-team", SpiritSessionID: "sp1", DagNodeID: "n2",
		DependsOn: []string{"n1"}, TaskDescription: "继续实现",
		DefinitionJSON: `{"members":[{"agent_id":"impl"}]}`,
	}
	teams.items["up-team"] = up
	teams.items["dn-team"] = down
	u := NewSpiritTeamUsecase(teams, newDeliverableSessionAccessor(), nil, loggateway.NewNoop())
	prefix := u.InjectUpstreamDeliverables(context.Background(), down)
	if prefix == "" {
		t.Fatal("expected prefix")
	}
	if !strings.Contains(prefix, "inbox/up-team/design.zip") {
		t.Fatalf("prefix must list inbox path, got:\n%s", prefix)
	}
	if strings.Contains(prefix, secret) {
		t.Fatalf("file body leaked into prefix:\n%s", prefix)
	}
	if strings.Contains(prefix, "read_upstream_deliverable") && strings.Contains(prefix, "pack") {
		// bulk must not instruct dumping the file via the text tool
		if strings.Contains(prefix, `key="pack"`) {
			t.Fatalf("bulk must not use read_upstream_deliverable, got:\n%s", prefix)
		}
	}
}

func TestDeliverableProtocolSuffix_ContainsBulkRule(t *testing.T) {
	u := NewSpiritTeamUsecase(nil, nil, nil, loggateway.NewNoop())
	got := u.DeliverableProtocolSuffix(Team{DagNodeID: "n1"})
	if !strings.Contains(got, "rel_path") || !strings.Contains(got, "inbox/") {
		t.Fatalf("protocol must mention Brief/Bulk attachments, got:\n%s", got)
	}
}

type memInboxFS struct {
	copies []InboxCopySpec
	err    error
}

func (m *memInboxFS) MaterializeFile(_ context.Context, spec InboxCopySpec) error {
	m.copies = append(m.copies, spec)
	return m.err
}

func TestMaterializeUpstreamInbox_CopiesDeclaredOnly(t *testing.T) {
	teams := newDeliverableTeamRepo()
	up := Team{
		ID: "up-team", SpiritSessionID: "sp1", DagNodeID: "n1",
		DisplayName: "上游", Status: TeamStatusCompleted,
		DefinitionJSON: `{"members":[{"agent_id":"writer"}]}`,
		DeliverablesOutput: marshalJSONValue(map[string]any{
			"n1": DeliverableRef{
				Summary: "ok",
				TeamID:  "up-team",
				Artifacts: []DeliverableArtifact{
					{Key: "pack", Kind: DeliverableArtifactKindWorkspaceRel, RelPath: "out/a.bin"},
					{Key: "article", SizeChars: 40}, // state_key: not copied
				},
			},
		}),
	}
	down := Team{
		ID: "dn-team", SpiritSessionID: "sp1", DagNodeID: "n2",
		DependsOn:       []string{"n1"},
		TaskDescription: "go",
		DefinitionJSON:  `{"members":[{"agent_id":"impl"}]}`,
	}
	teams.items["up-team"] = up
	teams.items["dn-team"] = down
	fs := &memInboxFS{}
	u := NewSpiritTeamUsecase(teams, newDeliverableSessionAccessor(), nil, loggateway.NewNoop(), WithTeamInboxFS(fs))
	_ = u.BuildTeamTurnInput(context.Background(), down)
	if len(fs.copies) != 1 {
		t.Fatalf("must copy only declared bulk files, got %+v", fs.copies)
	}
	c := fs.copies[0]
	if c.UpstreamTeamID != "up-team" || c.DestName != "a.bin" || c.RelPath != "out/a.bin" {
		t.Fatalf("copy spec %+v", c)
	}
	if len(c.DestAgentKeys) != 1 || c.DestAgentKeys[0] != "impl" {
		t.Fatalf("dest keys %+v", c.DestAgentKeys)
	}
}

func TestMaterializeUpstreamInbox_MissingFileWarnsNotFail(t *testing.T) {
	teams := newDeliverableTeamRepo()
	up := Team{
		ID: "up-team", SpiritSessionID: "sp1", DagNodeID: "n1",
		Status: TeamStatusCompleted, DefinitionJSON: `{"members":[{"agent_id":"writer"}]}`,
		DeliverablesOutput: marshalJSONValue(map[string]any{
			"n1": DeliverableRef{
				Summary:   "ok",
				Artifacts: []DeliverableArtifact{{Key: "pack", Kind: DeliverableArtifactKindWorkspaceRel, RelPath: "gone.bin"}},
			},
		}),
	}
	down := Team{
		SpiritSessionID: "sp1", DependsOn: []string{"n1"},
		DefinitionJSON: `{"members":[{"agent_id":"impl"}]}`, TaskDescription: "x",
	}
	teams.items["up-team"] = up
	fs := &memInboxFS{err: fmt.Errorf("source file not found")}
	u := NewSpiritTeamUsecase(teams, newDeliverableSessionAccessor(), nil, loggateway.NewNoop(), WithTeamInboxFS(fs))
	out := u.BuildTeamTurnInput(context.Background(), down)
	if !strings.Contains(out, "x") {
		t.Fatalf("turn input must still be produced, got %q", out)
	}
}

func marshalJSONValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestInboxDestNameIsBase(t *testing.T) {
	if path.Base("out/a.bin") != "a.bin" {
		t.Fatal("path.Base contract")
	}
}
