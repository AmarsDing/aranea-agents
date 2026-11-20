package knowledge

import (
	"context"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestEffectiveSpace(t *testing.T) {
	cases := []struct {
		c    Collection
		want string
	}{
		{Collection{VaultBackend: VaultBackendLocal}, SpaceMine},
		{Collection{VaultBackend: VaultBackendTeam}, SpaceCompany},
		{Collection{VaultBackend: VaultBackendLocal, Space: SpaceCompany}, SpaceCompany},
		{Collection{VaultBackend: VaultBackendTeam, Space: SpaceMine}, SpaceMine},
		{Collection{VaultBackend: VaultBackendTeam, Space: "bogus"}, SpaceCompany},
	}
	for _, tc := range cases {
		if got := EffectiveSpace(tc.c); got != tc.want {
			t.Fatalf("EffectiveSpace(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestUpdateCollectionSpace_RejectsBadSpace(t *testing.T) {
	uc := NewUsecaseFromRepo(noOpMockRepo())
	if _, err := uc.UpdateCollectionSpace(context.Background(), "c1", "shared"); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("err = %v, want BadRequest", err)
	}
}

func TestUpdateCollectionSpace_RepoWithoutPortIsUnavailable(t *testing.T) {
	uc := NewUsecaseFromRepo(noOpMockRepo())
	if _, err := uc.UpdateCollectionSpace(context.Background(), "c1", SpaceCompany); !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("err = %v, want Unavailable", err)
	}
}
