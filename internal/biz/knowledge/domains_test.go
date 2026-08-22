package knowledge

import (
	"context"
	"errors"
	"testing"
)

func TestUsecase_DomainFacades(t *testing.T) {
	t.Run("nil usecase returns nil facades", func(t *testing.T) {
		var u *Usecase
		if u.Vault() != nil || u.Retrieve() != nil || u.Graph() != nil || u.WriteBack() != nil || u.Curate() != nil {
			t.Fatal("nil usecase must return nil domain facades")
		}
	})

	t.Run("nil vault methods are unavailable", func(t *testing.T) {
		var v *Vault
		if _, err := v.CreateCollection(context.Background(), Collection{Name: "x", EmbeddingModel: "m"}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("CreateCollection = %v, want ErrUnavailable", err)
		}
		var r *Retrieve
		if _, err := r.Search(context.Background(), SearchQuery{Query: "q"}, nil); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Search = %v, want ErrUnavailable", err)
		}
		var w *WriteBack
		if w.HasReplay() {
			t.Fatal("nil WriteBack must not report replay")
		}
	})

	t.Run("constructed usecase exposes all domains", func(t *testing.T) {
		u := NewUsecaseFromRepo(noOpMockRepo())
		if u.Vault() == nil || u.Retrieve() == nil || u.Graph() == nil || u.WriteBack() == nil || u.Curate() == nil {
			t.Fatal("constructed usecase must expose all domain facades")
		}
		if u.WriteBack().HasReplay() {
			t.Fatal("replay should start unbound")
		}
		u.WriteBack().SetReplay(func(context.Context, Collection, []PromoteTouchedDoc) error { return nil })
		if !u.WriteBack().HasReplay() || !u.HasWriteBackReplay() {
			t.Fatal("WriteBack.SetReplay must bind usecase hook")
		}
	})
}
