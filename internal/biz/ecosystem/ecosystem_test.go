package ecosystem

import (
	"context"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type mockRepo struct {
	listProducts   func(ctx context.Context, q Query) (ListResult, error)
	getProduct     func(ctx context.Context, id string) (Product, error)
	createProduct  func(ctx context.Context, p Product) (Product, error)
	recordInstall  func(ctx context.Context, productID, refID string) error
	removeInstall  func(ctx context.Context, productID string) error
	isInstalled    func(ctx context.Context, productID string) (bool, error)
}

func (m *mockRepo) ListProducts(ctx context.Context, q Query) (ListResult, error) {
	if m.listProducts != nil {
		return m.listProducts(ctx, q)
	}
	return ListResult{}, nil
}

func (m *mockRepo) GetProduct(ctx context.Context, id string) (Product, error) {
	if m.getProduct != nil {
		return m.getProduct(ctx, id)
	}
	return Product{}, nil
}

func (m *mockRepo) CreateProduct(ctx context.Context, p Product) (Product, error) {
	if m.createProduct != nil {
		return m.createProduct(ctx, p)
	}
	return p, nil
}

func (m *mockRepo) RecordInstall(ctx context.Context, productID, refID string) error {
	if m.recordInstall != nil {
		return m.recordInstall(ctx, productID, refID)
	}
	return nil
}

func (m *mockRepo) RemoveInstall(ctx context.Context, productID string) error {
	if m.removeInstall != nil {
		return m.removeInstall(ctx, productID)
	}
	return nil
}

func (m *mockRepo) IsInstalled(ctx context.Context, productID string) (bool, error) {
	if m.isInstalled != nil {
		return m.isInstalled(ctx, productID)
	}
	return false, nil
}

func isKerrorReason(err error, reason string) bool {
	if err == nil {
		return false
	}
	ke, ok := err.(*kerrors.Error)
	if !ok {
		return false
	}
	return ke.Reason == reason
}

func TestNewUsecase(t *testing.T) {
	uc := NewUsecase(&mockRepo{})
	if uc == nil {
		t.Fatal("expected non-nil Usecase")
	}
}

func TestUsecase_List(t *testing.T) {
	tests := []struct {
		name      string
		query     Query
		setupRepo func() *mockRepo
		wantErr   bool
		wantLimit int32
	}{
		{
			name:  "default limit when zero",
			query: Query{},
			setupRepo: func() *mockRepo {
				return &mockRepo{listProducts: func(_ context.Context, q Query) (ListResult, error) {
					if q.Limit != 50 {
						t.Fatalf("expected default limit 50, got %d", q.Limit)
					}
					return ListResult{}, nil
				}}
			},
		},
		{
			name:  "explicit limit preserved",
			query: Query{Limit: 20},
			setupRepo: func() *mockRepo {
				return &mockRepo{listProducts: func(_ context.Context, q Query) (ListResult, error) {
					if q.Limit != 20 {
						t.Fatalf("expected limit 20, got %d", q.Limit)
					}
					return ListResult{Items: []Product{{ID: "p1"}}, Total: 1}, nil
				}}
			},
		},
		{
			name:  "repo error",
			query: Query{Limit: 10},
			setupRepo: func() *mockRepo {
				return &mockRepo{listProducts: func(_ context.Context, _ Query) (ListResult, error) {
					return ListResult{}, errors.New("db fail")
				}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			_, err := uc.List(context.Background(), tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUsecase_List_NilUsecase(t *testing.T) {
	var uc *Usecase
	result, err := uc.List(context.Background(), Query{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) > 0 || result.Total > 0 {
		t.Fatalf("expected empty result for nil usecase, got %+v", result)
	}
}

func TestUsecase_Get(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		setupRepo func() *mockRepo
		wantErr   bool
		reason    string
		checkFunc func(t *testing.T, p Product)
	}{
		{
			name:    "empty ID returns bad request",
			id:      "",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr: true,
			reason:  "ECOSYSTEM",
		},
		{
			name:    "whitespace ID returns bad request",
			id:      "   ",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr: true,
			reason:  "ECOSYSTEM",
		},
		{
			name: "valid ID returns product",
			id:   "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{
					getProduct: func(_ context.Context, id string) (Product, error) {
						return Product{ID: id, Name: "test"}, nil
					},
					isInstalled: func(_ context.Context, _ string) (bool, error) {
						return true, nil
					},
				}
			},
			checkFunc: func(t *testing.T, p Product) {
				if p.ID != "p1" {
					t.Fatalf("expected ID p1, got %s", p.ID)
				}
				if !p.Installed {
					t.Fatal("expected Installed=true")
				}
			},
		},
		{
			name: "repo error",
			id:   "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{getProduct: func(_ context.Context, _ string) (Product, error) {
					return Product{}, errors.New("db fail")
				}}
			},
			wantErr: true,
		},
		{
			name: "IsInstalled flag false",
			id:   "p2",
			setupRepo: func() *mockRepo {
				return &mockRepo{
					getProduct: func(_ context.Context, id string) (Product, error) {
						return Product{ID: id, Name: "test2"}, nil
					},
					isInstalled: func(_ context.Context, _ string) (bool, error) {
						return false, nil
					},
				}
			},
			checkFunc: func(t *testing.T, p Product) {
				if p.Installed {
					t.Fatal("expected Installed=false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			p, err := uc.Get(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.reason != "" {
				if !isKerrorReason(err, tt.reason) {
					t.Fatalf("expected kerror reason %s, got %v", tt.reason, err)
				}
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, p)
			}
		})
	}
}

func TestUsecase_Publish(t *testing.T) {
	tests := []struct {
		name      string
		input     Product
		setupRepo func() *mockRepo
		wantErr   bool
		reason    string
		checkFunc func(t *testing.T, p Product)
	}{
		{
			name:    "empty name returns bad request",
			input:   Product{Name: ""},
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr: true,
			reason:  "ECOSYSTEM",
		},
		{
			name:    "whitespace name returns bad request",
			input:   Product{Name: "   "},
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr: true,
			reason:  "ECOSYSTEM",
		},
		{
			name:  "defaults filled when empty",
			input: Product{Name: "my-pack"},
			setupRepo: func() *mockRepo {
				return &mockRepo{createProduct: func(_ context.Context, p Product) (Product, error) {
					return p, nil
				}}
			},
			checkFunc: func(t *testing.T, p Product) {
				if p.DisplayName != "my-pack" {
					t.Fatalf("expected DisplayName my-pack, got %s", p.DisplayName)
				}
				if p.Type != "skill_pack" {
					t.Fatalf("expected Type skill_pack, got %s", p.Type)
				}
				if p.Version != "1.0.0" {
					t.Fatalf("expected Version 1.0.0, got %s", p.Version)
				}
				if p.PriceModel != "free" {
					t.Fatalf("expected PriceModel free, got %s", p.PriceModel)
				}
				if p.Status != "published" {
					t.Fatalf("expected Status published, got %s", p.Status)
				}
				if p.CreatedAt == "" {
					t.Fatal("expected non-empty CreatedAt")
				}
				if p.UpdatedAt == "" {
					t.Fatal("expected non-empty UpdatedAt")
				}
			},
		},
		{
			name:  "explicit values preserved",
			input: Product{Name: "my-pack", DisplayName: "My Pack", Type: "agent", Version: "2.0.0", PriceModel: "paid", Status: "draft"},
			setupRepo: func() *mockRepo {
				return &mockRepo{createProduct: func(_ context.Context, p Product) (Product, error) {
					return p, nil
				}}
			},
			checkFunc: func(t *testing.T, p Product) {
				if p.DisplayName != "My Pack" {
					t.Fatalf("expected DisplayName My Pack, got %s", p.DisplayName)
				}
				if p.Type != "agent" {
					t.Fatalf("expected Type agent, got %s", p.Type)
				}
				if p.Version != "2.0.0" {
					t.Fatalf("expected Version 2.0.0, got %s", p.Version)
				}
				if p.PriceModel != "paid" {
					t.Fatalf("expected PriceModel paid, got %s", p.PriceModel)
				}
				if p.Status != "draft" {
					t.Fatalf("expected Status draft, got %s", p.Status)
				}
			},
		},
		{
			name:  "repo error",
			input: Product{Name: "my-pack"},
			setupRepo: func() *mockRepo {
				return &mockRepo{createProduct: func(_ context.Context, _ Product) (Product, error) {
					return Product{}, errors.New("db fail")
				}}
			},
			wantErr: true,
		},
		{
			name:  "ID generated when empty",
			input: Product{Name: "my-pack"},
			setupRepo: func() *mockRepo {
				return &mockRepo{createProduct: func(_ context.Context, p Product) (Product, error) {
					if p.ID == "" {
						t.Fatal("expected generated ID, got empty")
					}
					return p, nil
				}}
			},
		},
		{
			name:  "existing ID preserved",
			input: Product{ID: "existing-id", Name: "my-pack"},
			setupRepo: func() *mockRepo {
				return &mockRepo{createProduct: func(_ context.Context, p Product) (Product, error) {
					if p.ID != "existing-id" {
						t.Fatalf("expected ID existing-id, got %s", p.ID)
					}
					return p, nil
				}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			p, err := uc.Publish(context.Background(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Publish() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.reason != "" {
				if !isKerrorReason(err, tt.reason) {
					t.Fatalf("expected kerror reason %s, got %v", tt.reason, err)
				}
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, p)
			}
		})
	}
}

func TestUsecase_Install(t *testing.T) {
	tests := []struct {
		name      string
		productID string
		setupRepo func() *mockRepo
		wantErr   bool
		reason    string
		checkFunc func(t *testing.T, r InstallResult)
	}{
		{
			name:      "empty product ID returns bad request",
			productID: "",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr:   true,
			reason:    "ECOSYSTEM",
		},
		{
			name:      "whitespace product ID returns bad request",
			productID: "   ",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr:   true,
			reason:    "ECOSYSTEM",
		},
		{
			name:      "repo GetProduct error",
			productID: "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{getProduct: func(_ context.Context, _ string) (Product, error) {
					return Product{}, errors.New("not found")
				}}
			},
			wantErr: true,
		},
		{
			name:      "success",
			productID: "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{
					getProduct: func(_ context.Context, id string) (Product, error) {
						return Product{ID: id, DisplayName: "TestPack"}, nil
					},
					recordInstall: func(_ context.Context, productID, refID string) error {
						if productID != "p1" {
							t.Fatalf("expected productID p1, got %s", productID)
						}
						if refID == "" {
							t.Fatal("expected non-empty refID")
						}
						return nil
					},
				}
			},
			checkFunc: func(t *testing.T, r InstallResult) {
				if r.ProductID != "p1" {
					t.Fatalf("expected ProductID p1, got %s", r.ProductID)
				}
				if len(r.InstalledIDs) != 1 {
					t.Fatalf("expected 1 InstalledID, got %d", len(r.InstalledIDs))
				}
				if r.Message != "installed TestPack" {
					t.Fatalf("expected message 'installed TestPack', got %s", r.Message)
				}
			},
		},
		{
			name:      "RecordInstall error",
			productID: "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{
					getProduct: func(_ context.Context, id string) (Product, error) {
						return Product{ID: id, DisplayName: "TestPack"}, nil
					},
					recordInstall: func(_ context.Context, _, _ string) error {
						return errors.New("install fail")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			r, err := uc.Install(context.Background(), tt.productID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Install() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.reason != "" {
				if !isKerrorReason(err, tt.reason) {
					t.Fatalf("expected kerror reason %s, got %v", tt.reason, err)
				}
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, r)
			}
		})
	}
}

func TestUsecase_Uninstall(t *testing.T) {
	tests := []struct {
		name      string
		productID string
		setupRepo func() *mockRepo
		wantErr   bool
		reason    string
	}{
		{
			name:      "empty product ID returns bad request",
			productID: "",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr:   true,
			reason:    "ECOSYSTEM",
		},
		{
			name:      "whitespace product ID returns bad request",
			productID: "   ",
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantErr:   true,
			reason:    "ECOSYSTEM",
		},
		{
			name:      "success",
			productID: "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{removeInstall: func(_ context.Context, id string) error {
					if id != "p1" {
						t.Fatalf("expected id p1, got %s", id)
					}
					return nil
				}}
			},
		},
		{
			name:      "repo error",
			productID: "p1",
			setupRepo: func() *mockRepo {
				return &mockRepo{removeInstall: func(_ context.Context, _ string) error {
					return errors.New("db fail")
				}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			err := uc.Uninstall(context.Background(), tt.productID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Uninstall() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.reason != "" {
				if !isKerrorReason(err, tt.reason) {
					t.Fatalf("expected kerror reason %s, got %v", tt.reason, err)
				}
			}
		})
	}
}
