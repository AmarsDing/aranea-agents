package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

var ecosystemIDRand uint64

type EcosystemProduct struct {
	ID           string
	Name         string
	DisplayName  string
	Description  string
	Type         string
	AuthorID     string
	Version      string
	PriceModel   string
	PriceCents   int64
	Rating       float64
	InstallCount int64
	ConfigJSON   string
	Status       string
	CreatedAt    string
	UpdatedAt    string
	Installed    bool
}

type EcosystemInstallResult struct {
	ProductID    string
	InstalledIDs []string
	Message      string
}

type EcosystemQuery struct {
	Type   string
	Search string
	Limit  int32
	Offset int32
}

type EcosystemListResult struct {
	Items []EcosystemProduct
	Total int32
}

type EcosystemRepo interface {
	ListProducts(ctx context.Context, q EcosystemQuery) (EcosystemListResult, error)
	GetProduct(ctx context.Context, id string) (EcosystemProduct, error)
	CreateProduct(ctx context.Context, p EcosystemProduct) (EcosystemProduct, error)
	RecordInstall(ctx context.Context, productID, refID string) error
	RemoveInstall(ctx context.Context, productID string) error
	IsInstalled(ctx context.Context, productID string) (bool, error)
}

type EcosystemUsecase struct {
	repo EcosystemRepo
}

func NewEcosystemUsecase(repo EcosystemRepo) *EcosystemUsecase {
	return &EcosystemUsecase{repo: repo}
}

func newEcosystemID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&ecosystemIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

func (u *EcosystemUsecase) List(ctx context.Context, q EcosystemQuery) (EcosystemListResult, error) {
	if u == nil || u.repo == nil {
		return EcosystemListResult{}, nil
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return u.repo.ListProducts(ctx, q)
}

func (u *EcosystemUsecase) Get(ctx context.Context, id string) (EcosystemProduct, error) {
	if strings.TrimSpace(id) == "" {
		return EcosystemProduct{}, errors.BadRequest("ECOSYSTEM", "id is required")
	}
	p, err := u.repo.GetProduct(ctx, id)
	if err != nil {
		return EcosystemProduct{}, err
	}
	if u.repo != nil {
		p.Installed, _ = u.repo.IsInstalled(ctx, id)
	}
	return p, nil
}

func (u *EcosystemUsecase) Publish(ctx context.Context, in EcosystemProduct) (EcosystemProduct, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return EcosystemProduct{}, errors.BadRequest("ECOSYSTEM", "name is required")
	}
	if in.ID == "" {
		in.ID = newEcosystemID()
	}
	if in.DisplayName == "" {
		in.DisplayName = in.Name
	}
	if in.Type == "" {
		in.Type = "skill_pack"
	}
	if in.Version == "" {
		in.Version = "1.0.0"
	}
	if in.PriceModel == "" {
		in.PriceModel = "free"
	}
	if in.Status == "" {
		in.Status = "published"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	in.CreatedAt = now
	in.UpdatedAt = now
	return u.repo.CreateProduct(ctx, in)
}

func (u *EcosystemUsecase) Install(ctx context.Context, productID string) (EcosystemInstallResult, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return EcosystemInstallResult{}, errors.BadRequest("ECOSYSTEM", "product id is required")
	}
	p, err := u.repo.GetProduct(ctx, productID)
	if err != nil {
		return EcosystemInstallResult{}, err
	}
	refID := uuid.NewString()
	if err := u.repo.RecordInstall(ctx, productID, refID); err != nil {
		return EcosystemInstallResult{}, err
	}
	return EcosystemInstallResult{
		ProductID:    productID,
		InstalledIDs: []string{refID},
		Message:      "installed " + p.DisplayName,
	}, nil
}

func (u *EcosystemUsecase) Uninstall(ctx context.Context, productID string) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return errors.BadRequest("ECOSYSTEM", "product id is required")
	}
	return u.repo.RemoveInstall(ctx, productID)
}
