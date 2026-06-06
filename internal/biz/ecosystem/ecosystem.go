// Package ecosystem implements the ecosystem marketplace workflows.
package ecosystem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

var ecosystemIDRand uint64

// Product represents an ecosystem marketplace product.
type Product struct {
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

// InstallResult holds the result of an ecosystem product install.
type InstallResult struct {
	ProductID    string
	InstalledIDs []string
	Message      string
}

// Query holds filter parameters for listing products.
type Query struct {
	Type   string
	Search string
	Limit  int32
	Offset int32
}

// ListResult holds a page of products.
type ListResult struct {
	Items []Product
	Total int32
}

// Repo abstracts ecosystem product persistence.
type Repo interface {
	ListProducts(ctx context.Context, q Query) (ListResult, error)
	GetProduct(ctx context.Context, id string) (Product, error)
	CreateProduct(ctx context.Context, p Product) (Product, error)
	RecordInstall(ctx context.Context, productID, refID string) error
	RemoveInstall(ctx context.Context, productID string) error
	IsInstalled(ctx context.Context, productID string) (bool, error)
}

// Usecase implements ecosystem workflows.
type Usecase struct {
	repo Repo
	lg   loggateway.Logger
}

// NewUsecase constructs an ecosystem Usecase.
func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase {
	return &Usecase{repo: repo, lg: lg}
}

func newEcosystemID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&ecosystemIDRand, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// List returns a page of ecosystem products.
func (u *Usecase) List(ctx context.Context, q Query) (ListResult, error) {
	if u == nil || u.repo == nil {
		return ListResult{}, nil
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return u.repo.ListProducts(ctx, q)
}

// Get returns a single ecosystem product.
func (u *Usecase) Get(ctx context.Context, id string) (Product, error) {
	if strings.TrimSpace(id) == "" {
		return Product{}, errors.BadRequest("ECOSYSTEM", "id is required")
	}
	p, err := u.repo.GetProduct(ctx, id)
	if err != nil {
		return Product{}, err
	}
	if u.repo != nil {
		installed, err := u.repo.IsInstalled(ctx, id)
		if err != nil {
			u.lg.Warn("check ecosystem install status failed", loggateway.Err(err), loggateway.Str("id", id))
		}
		p.Installed = installed
	}
	return p, nil
}

// Publish creates a new ecosystem product.
func (u *Usecase) Publish(ctx context.Context, in Product) (Product, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Product{}, errors.BadRequest("ECOSYSTEM", "name is required")
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

// Install installs an ecosystem product.
func (u *Usecase) Install(ctx context.Context, productID string) (InstallResult, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return InstallResult{}, errors.BadRequest("ECOSYSTEM", "product id is required")
	}
	p, err := u.repo.GetProduct(ctx, productID)
	if err != nil {
		return InstallResult{}, err
	}
	refID := uuid.NewString()
	if err := u.repo.RecordInstall(ctx, productID, refID); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{
		ProductID:    productID,
		InstalledIDs: []string{refID},
		Message:      "installed " + p.DisplayName,
	}, nil
}

// Uninstall removes an ecosystem product installation.
func (u *Usecase) Uninstall(ctx context.Context, productID string) error {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return errors.BadRequest("ECOSYSTEM", "product id is required")
	}
	return u.repo.RemoveInstall(ctx, productID)
}
