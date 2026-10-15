package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestUsecase_GetDocument(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		repoFn    func(_ context.Context, id string) (Document, error)
		wantErr   bool
		wantErrIs error
		check     func(t *testing.T, got Document)
	}{
		{"empty id rejected", "", nil, true, ErrIDRequired, nil},
		{"whitespace id rejected", "  ", nil, true, ErrIDRequired, nil},
		{
			"valid id returns content fields",
			"doc-1",
			func(_ context.Context, id string) (Document, error) {
				return Document{ID: id, ContentText: "# 标题\n正文", Organized: true, AssetURI: ""}, nil
			},
			false, nil,
			func(t *testing.T, got Document) {
				if got.ID != "doc-1" {
					t.Errorf("ID = %q, want %q", got.ID, "doc-1")
				}
				if got.ContentText != "# 标题\n正文" {
					t.Errorf("ContentText = %q, want organized markdown", got.ContentText)
				}
				if !got.Organized {
					t.Error("Organized = false, want true")
				}
			},
		},
		{
			"repo error propagated",
			"doc-missing",
			func(_ context.Context, _ string) (Document, error) { return Document{}, fmt.Errorf("not found") },
			true, nil, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docGetFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			got, err := u.GetDocument(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_RequireRepo(t *testing.T) {
	tests := []struct {
		name string
		u    *Usecase
		want bool
	}{
		{"nil usecase", nil, true},
		{"nil collections", &Usecase{}, true},
		{"all present", NewUsecaseFromRepo(noOpMockRepo()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.u.requireRepo()
			got := err != nil
			if got != tt.want {
				t.Errorf("requireRepo() error = %v, want error %v", err, tt.want)
			}
		})
	}
}
