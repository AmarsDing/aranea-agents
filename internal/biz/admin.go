package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/errors"
)

// Admin is a Admin model.
type Admin struct {
	ID         int64
	Name       string
	Email      string
	Password   string
	Access     string
	Avatar     string
	CreateTime time.Time
	UpdateTime time.Time
}

// AdminRepo is a Greater repo.
type AdminRepo interface {
	FindByID(context.Context, int64) (*Admin, error)
	FindByName(context.Context, string) (*Admin, error)
	FindByEmail(context.Context, string) (*Admin, error)
	ListAdmins(context.Context, ...ListOption) ([]*Admin, error)
	CreateAdmin(context.Context, *Admin) (*Admin, error)
	UpdateAdmin(context.Context, *Admin) (*Admin, error)
	DeleteAdmin(context.Context, int64) error
}

// AdminUsecase is a Admin usecase.
type AdminUsecase struct {
	admin AdminRepo
	lg    loggateway.Logger
}

// NewAdminUsecase new a Admin usecase.
func NewAdminUsecase(repo AdminRepo, lg loggateway.Logger) *AdminUsecase {
	return &AdminUsecase{admin: repo, lg: lg}
}

// LoginByUsername logs in a user by username and password.
func (uc *AdminUsecase) LoginByUsername(ctx context.Context, username, password string) (*Admin, error) {
	user, err := uc.admin.FindByName(ctx, username)
	if err != nil {
		uc.lg.Warn("admin login failed: user not found",
			loggateway.StepID("system.admin.login_failed"), loggateway.Str("method", "username"), loggateway.Str("username", username))
		return nil, err
	}
	if user.Password != password {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("system.admin.login_failed"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
		return nil, errors.Unauthorized("AUTH", "invalid credentials")
	}
	uc.lg.Info("admin logged in",
		loggateway.StepID("system.admin.login"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
	return user, nil
}

// LoginByEmail logs in a user by email and password.
func (uc *AdminUsecase) LoginByEmail(ctx context.Context, email, password string) (*Admin, error) {
	user, err := uc.admin.FindByEmail(ctx, email)
	if err != nil {
		uc.lg.Warn("admin login failed: user not found",
			loggateway.StepID("system.admin.login_failed"), loggateway.Str("method", "email"), loggateway.Str("email", email))
		return nil, err
	}
	if user.Password != password {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("system.admin.login_failed"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
		return nil, errors.Unauthorized("AUTH", "invalid credentials")
	}
	uc.lg.Info("admin logged in",
		loggateway.StepID("system.admin.login"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
	return user, nil
}

// Logout logs out the current user.
func (uc *AdminUsecase) Logout(ctx context.Context, adminID int64) error {
	admin, err := uc.admin.FindByID(ctx, adminID)
	if err != nil {
		return err
	}
	uc.lg.Info("admin logged out",
		loggateway.StepID("system.admin.logout"), loggateway.Str("admin_name", admin.Name))
	return nil
}

// Current returns the current logged in user.
func (uc *AdminUsecase) GetAdmin(ctx context.Context, id int64) (*Admin, error) {
	return uc.admin.FindByID(ctx, id)
}

// ListAdmins lists admin users with pagination.
func (uc *AdminUsecase) ListAdmins(ctx context.Context, opts ...ListOption) ([]*Admin, error) {
	admins, err := uc.admin.ListAdmins(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return admins, nil
}

// CreateAdmin creates a new admin user.
func (uc *AdminUsecase) CreateAdmin(ctx context.Context, admin *Admin) (*Admin, error) {
	return uc.admin.CreateAdmin(ctx, admin)
}

// UpdateAdmin updates an existing admin user.
func (uc *AdminUsecase) UpdateAdmin(ctx context.Context, admin *Admin) (*Admin, error) {
	return uc.admin.UpdateAdmin(ctx, admin)
}

// DeleteAdmin deletes an admin user by ID.
func (uc *AdminUsecase) DeleteAdmin(ctx context.Context, id int64) error {
	return uc.admin.DeleteAdmin(ctx, id)
}
