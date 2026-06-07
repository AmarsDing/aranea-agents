package biz

import (
	"context"
	"crypto/subtle"
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

// AdminReader provides read-only access to admin records.
type AdminReader interface {
	FindByID(context.Context, int64) (*Admin, error)
	FindByName(context.Context, string) (*Admin, error)
	FindByEmail(context.Context, string) (*Admin, error)
	ListAdmins(context.Context, ...ListOption) ([]*Admin, error)
}

// AdminWriter provides write access to admin records.
type AdminWriter interface {
	CreateAdmin(context.Context, *Admin) (*Admin, error)
	UpdateAdmin(context.Context, *Admin) (*Admin, error)
	DeleteAdmin(context.Context, int64) error
}

// AdminRepo composes read and write access for admin records.
// Deprecated: use AdminReader or AdminWriter for narrower dependency.
type AdminRepo interface {
	AdminReader
	AdminWriter
}

// AdminUsecase is a Admin usecase.
// W-1 fix: depend on narrow interfaces instead of composite AdminRepo.
type AdminUsecase struct {
	reader AdminReader
	writer AdminWriter
	lg     loggateway.Logger
}

// NewAdminUsecase new a Admin usecase.
// S-1 fix: accept narrow interfaces instead of composite AdminRepo.
func NewAdminUsecase(reader AdminReader, writer AdminWriter, lg loggateway.Logger) *AdminUsecase {
	return &AdminUsecase{reader: reader, writer: writer, lg: lg}
}

// ErrInvalidCredentials is the unified error for both user-not-found and
// wrong-password cases, preventing timing attacks that reveal user existence.
var ErrInvalidCredentials = errors.Unauthorized("AUTH", "invalid credentials")

// LoginByUsername logs in a user by username and password.
// B-01/B-02 fix: constant-time comparison + unified error to prevent timing attacks.
func (uc *AdminUsecase) LoginByUsername(ctx context.Context, username, password string) (*Admin, error) {
	user, err := uc.reader.FindByName(ctx, username)
	if err != nil {
		uc.lg.Warn("admin login failed",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "username"))
		return nil, ErrInvalidCredentials
	}
	if subtle.ConstantTimeCompare([]byte(user.Password), []byte(password)) != 1 {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
		return nil, ErrInvalidCredentials
	}
	uc.lg.Info("admin logged in",
		loggateway.StepID("admin.login"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
	return user, nil
}

// LoginByEmail logs in a user by email and password.
// B-01/B-02 fix: constant-time comparison + unified error to prevent timing attacks.
func (uc *AdminUsecase) LoginByEmail(ctx context.Context, email, password string) (*Admin, error) {
	user, err := uc.reader.FindByEmail(ctx, email)
	if err != nil {
		uc.lg.Warn("admin login failed",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "email"))
		return nil, ErrInvalidCredentials
	}
	if subtle.ConstantTimeCompare([]byte(user.Password), []byte(password)) != 1 {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
		return nil, ErrInvalidCredentials
	}
	uc.lg.Info("admin logged in",
		loggateway.StepID("admin.login"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
	return user, nil
}

// Logout logs out the current user.
func (uc *AdminUsecase) Logout(ctx context.Context, adminID int64) error {
	admin, err := uc.reader.FindByID(ctx, adminID)
	if err != nil {
		return err
	}
	uc.lg.Info("admin logged out",
		loggateway.StepID("admin.logout"), loggateway.Str("admin_name", admin.Name))
	return nil
}

// Current returns the current logged in user.
func (uc *AdminUsecase) GetAdmin(ctx context.Context, id int64) (*Admin, error) {
	return uc.reader.FindByID(ctx, id)
}

// ListAdmins lists admin users with pagination.
func (uc *AdminUsecase) ListAdmins(ctx context.Context, opts ...ListOption) ([]*Admin, error) {
	admins, err := uc.reader.ListAdmins(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return admins, nil
}

// CreateAdmin creates a new admin user.
func (uc *AdminUsecase) CreateAdmin(ctx context.Context, admin *Admin) (*Admin, error) {
	return uc.writer.CreateAdmin(ctx, admin)
}

// UpdateAdmin updates an existing admin user.
func (uc *AdminUsecase) UpdateAdmin(ctx context.Context, admin *Admin) (*Admin, error) {
	return uc.writer.UpdateAdmin(ctx, admin)
}

// DeleteAdmin deletes an admin user by ID.
func (uc *AdminUsecase) DeleteAdmin(ctx context.Context, id int64) error {
	return uc.writer.DeleteAdmin(ctx, id)
}
