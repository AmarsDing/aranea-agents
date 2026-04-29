package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/errors"
)

var hookIDRand uint64

func newHookID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&hookIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// Hook is one row of hooks (legacy "hooks" platform resource).
type Hook struct {
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

type HookRepo interface {
	ListHooks(ctx context.Context) ([]Hook, error)
	GetHook(ctx context.Context, id string) (Hook, error)
	CreateHook(ctx context.Context, h Hook) (Hook, error)
	UpdateHook(ctx context.Context, h Hook) (Hook, error)
	DeleteHook(ctx context.Context, id string) error
}

type HookUsecase struct {
	repo HookRepo
}

func NewHookUsecase(repo HookRepo) *HookUsecase {
	return &HookUsecase{repo: repo}
}

func (u *HookUsecase) List(ctx context.Context) ([]Hook, error) {
	return u.repo.ListHooks(ctx)
}

func (u *HookUsecase) Get(ctx context.Context, id string) (Hook, error) {
	if strings.TrimSpace(id) == "" {
		return Hook{}, errors.BadRequest("HOOK", "id is required")
	}
	return u.repo.GetHook(ctx, id)
}

func (u *HookUsecase) Create(ctx context.Context, in Hook) (Hook, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return Hook{}, errors.BadRequest("HOOK", "key and name are required")
	}
	if in.ID == "" {
		in.ID = newHookID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.repo.CreateHook(ctx, in)
}

func (u *HookUsecase) Update(ctx context.Context, id string, patch Hook) (Hook, error) {
	if strings.TrimSpace(id) == "" {
		return Hook{}, errors.BadRequest("HOOK", "id is required")
	}
	cur, err := u.repo.GetHook(ctx, id)
	if err != nil {
		return Hook{}, err
	}
	merged := cur
	if patch.Key != "" {
		merged.Key = patch.Key
	}
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Status != "" {
		merged.Status = patch.Status
	}
	merged.Description = patch.Description
	merged.Enabled = patch.Enabled
	merged.SortOrder = patch.SortOrder
	merged.ConfigJSON = patch.ConfigJSON
	merged.MetadataJSON = patch.MetadataJSON
	if merged.Key == "" {
		merged.Key = cur.Key
	}
	if merged.Name == "" {
		merged.Name = cur.Name
	}
	if merged.Status == "" {
		merged.Status = cur.Status
	}
	return u.repo.UpdateHook(ctx, merged)
}

func (u *HookUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("HOOK", "id is required")
	}
	return u.repo.DeleteHook(ctx, id)
}
