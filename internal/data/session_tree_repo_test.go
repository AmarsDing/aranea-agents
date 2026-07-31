package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TS9-BUG-5: GetSessionTree must return a populated root even when the root
// session row carries the production-real values session_type=” and
// root_session_id=” (sessions created via API / channel ingress never get
// session_type='spirit' assigned — no creation path sets it).
//
// Tree shape under test (mirrors the TS9 it-ops closed-loop run):
//
//	spirit (session_type='', root_session_id='')
//	├── team-1 (session_type='team')
//	│   └── member-1 (session_type='agent')
//	└── team-2 (session_type='team')
func TestGetSessionTree_RootWithEmptyTypeAndRootID(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)
	repo := NewSessionRepo(d, nil, nil)

	mk := func(id, sessionType, parentID, rootID string, depth int) {
		t.Helper()
		_, err := client.Session.Create().
			SetID(id).
			SetTitle(id).
			SetStatus("completed").
			SetSessionType(sessionType).
			SetParentSessionID(parentID).
			SetRootSessionID(rootID).
			SetAgentDepth(depth).
			Save(ctx)
		if err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
	}
	mk("spirit-1", "", "", "", 0)
	mk("team-1", "team", "spirit-1", "spirit-1", 1)
	mk("team-2", "team", "spirit-1", "spirit-1", 1)
	mk("member-1", "agent", "team-1", "spirit-1", 2)

	tree, err := repo.GetSessionTree(ctx, "spirit-1")
	if err != nil {
		t.Fatalf("GetSessionTree: %v", err)
	}
	if tree == nil || tree.Root.ID == "" {
		t.Fatal("tree.Root must be populated for the spirit session")
	}
	if tree.Root.ID != "spirit-1" {
		t.Fatalf("root id = %q, want spirit-1", tree.Root.ID)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("root children = %d, want 2 (team-1, team-2)", len(tree.Children))
	}
	for _, ch := range tree.Children {
		if ch.Session.ID == "" {
			t.Fatal("child session must be populated")
		}
		if ch.Session.ID == "team-1" && len(ch.Children) != 1 {
			t.Fatalf("team-1 children = %d, want 1 (member-1)", len(ch.Children))
		}
	}
}

// Legacy path: root explicitly typed 'spirit' with root_session_id pointing at
// itself must keep working (root identified by ID match, not only by type).
func TestGetSessionTree_RootTypedSpirit(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()
	client, _ := testhelper.SetupTestPG(t)
	d := newDataFromClient(client, lg)
	repo := NewSessionRepo(d, nil, nil)

	mk := func(id, sessionType, parentID, rootID string, depth int) {
		t.Helper()
		_, err := client.Session.Create().
			SetID(id).
			SetTitle(id).
			SetStatus("completed").
			SetSessionType(sessionType).
			SetParentSessionID(parentID).
			SetRootSessionID(rootID).
			SetAgentDepth(depth).
			Save(ctx)
		if err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
	}
	mk("spirit-2", "spirit", "", "spirit-2", 0)
	mk("team-3", "team", "spirit-2", "spirit-2", 1)

	tree, err := repo.GetSessionTree(ctx, "spirit-2")
	if err != nil {
		t.Fatalf("GetSessionTree: %v", err)
	}
	if tree == nil || tree.Root.ID != "spirit-2" {
		t.Fatalf("root = %+v, want spirit-2", tree.Root)
	}
	if len(tree.Children) != 1 || tree.Children[0].Session.ID != "team-3" {
		t.Fatalf("children = %+v, want [team-3]", tree.Children)
	}
}
