package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

func TestDocumentVisibleTo(t *testing.T) {
	t.Parallel()
	private := Document{ID: "d1", Visibility: DocVisibilityPrivate, OwnerUserID: "7"}
	shared := Document{ID: "d2", Visibility: DocVisibilityCollection, OwnerUserID: "7"}
	legacy := Document{ID: "d3"}

	sys := workspace.WithSystemWorkspace(context.Background())
	if !DocumentVisibleTo(sys, private) {
		t.Fatal("system must see private docs")
	}

	anon := context.Background()
	if DocumentVisibleTo(anon, private) {
		t.Fatal("anonymous must not see private docs")
	}
	if !DocumentVisibleTo(anon, shared) || !DocumentVisibleTo(anon, legacy) {
		t.Fatal("anonymous must see collection/legacy docs")
	}

	owner := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	if !DocumentVisibleTo(owner, private) {
		t.Fatal("owner must see own private doc")
	}
	other := auth.NewContext(context.Background(), &auth.Auth{UserID: 8})
	if DocumentVisibleTo(other, private) {
		t.Fatal("non-owner must not see private doc")
	}
}

func TestUpdateDocumentVisibility_RequiresSignInForPrivate(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection},
	}
	u := NewUsecaseFromRepo(&mockRepo{
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
	})
	u.SetDocumentACLStore(&stubACL{})
	if _, err := u.UpdateDocumentVisibility(context.Background(), "d1", DocVisibilityPrivate); err == nil {
		t.Fatal("anonymous private mark must fail")
	}
}

func TestUpdateDocumentVisibility_SetsOwner(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection},
	}
	acl := &stubACL{}
	u := NewUsecaseFromRepo(&mockRepo{
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
	})
	u.SetDocumentACLStore(acl)
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 9})
	got, err := u.UpdateDocumentVisibility(ctx, "d1", DocVisibilityPrivate)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visibility != DocVisibilityPrivate || got.OwnerUserID != "9" {
		t.Fatalf("got %+v", got)
	}
	if acl.id != "d1" || acl.visibility != DocVisibilityPrivate || acl.owner != "9" {
		t.Fatalf("acl %+v", acl)
	}
}

type stubACL struct {
	id, visibility, owner string
}

func (s *stubACL) UpdateDocumentVisibility(_ context.Context, id, visibility, ownerUserID string) error {
	s.id = id
	s.visibility = visibility
	s.owner = ownerUserID
	return nil
}
