package biz

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"golang.org/x/crypto/bcrypt"
)

// Admin is a Admin model.
type Admin struct {
	ID          int64
	Name        string
	Email       string
	Password    string
	Access      string
	Avatar      string
	WorkspaceID string
	CreateTime  time.Time
	UpdateTime  time.Time
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
var ErrInvalidCredentials = apierror.Unauthorized("AUTH", "invalid credentials")

// isLegacyMD5Hash reports whether the stored password looks like a legacy MD5
// hash (32 hex chars, no bcrypt prefix). This is used for lazy migration to
// bcrypt on the next successful login.
func isLegacyMD5Hash(stored string) bool {
	if len(stored) != 32 {
		return false
	}
	if strings.HasPrefix(stored, "$") {
		return false
	}
	for _, r := range stored {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// verifyPassword verifies a plain password against the stored hash. It supports
// both bcrypt (preferred) and legacy MD5 (for backward compatibility). When the
// stored hash is MD5 and the password matches, the function returns true so the
// caller can lazily re-hash with bcrypt via migratePasswordToBcrypt.
func verifyPassword(stored, plain string) bool {
	if strings.HasPrefix(stored, "$2") {
		// bcrypt hash: use constant-time bcrypt comparison.
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(plain)) == nil
	}
	if isLegacyMD5Hash(stored) {
		sum := md5.Sum([]byte(plain))
		return subtle.ConstantTimeCompare([]byte(stored), []byte(hex.EncodeToString(sum[:]))) == 1
	}
	// Unknown format: fall back to constant-time direct compare (preserves
	// previous behavior for any non-MD5, non-bcrypt stored value).
	return subtle.ConstantTimeCompare([]byte(stored), []byte(plain)) == 1
}

// migratePasswordToBcrypt re-hashes a plain password with bcrypt so the legacy
// MD5 hash can be replaced. Callers should persist the returned hash via
// UpdateAdmin. Errors are logged but non-fatal: a failed migration does not
// block the login that already succeeded.
//
// IMPORTANT: the full *Admin record (already loaded by LoginByUsername/LoginByEmail)
// must be passed so UpdateAdmin preserves name/email/access/avatar. Passing only
// ID+Password would zero out the other fields because adminRepo.UpdateAdmin is a
// full-field update, not a patch.
func (uc *AdminUsecase) migratePasswordToBcrypt(ctx context.Context, user *Admin, plain string) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		uc.lg.Warn("admin login: bcrypt migration failed",
			loggateway.StepID("admin.login"), loggateway.Err(err))
		return
	}
	user.Password = string(hashed)
	if _, err := uc.writer.UpdateAdmin(ctx, user); err != nil {
		uc.lg.Warn("admin login: persist bcrypt migration failed",
			loggateway.StepID("admin.login"), loggateway.Err(err))
	}
}

// LoginByUsername logs in a user by username and password.
// B-01/B-02 fix: constant-time comparison + unified error to prevent timing attacks.
// The password argument must be the plain-text password; legacy MD5 hashes are
// transparently migrated to bcrypt on successful login.
func (uc *AdminUsecase) LoginByUsername(ctx context.Context, username, password string) (*Admin, error) {
	user, err := uc.reader.FindByName(ctx, username)
	if err != nil {
		uc.lg.Warn("admin login failed",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "username"))
		return nil, ErrInvalidCredentials
	}
	if !verifyPassword(user.Password, password) {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
		return nil, ErrInvalidCredentials
	}
	if isLegacyMD5Hash(user.Password) {
		uc.migratePasswordToBcrypt(ctx, user, password)
	}
	uc.lg.Info("admin logged in",
		loggateway.StepID("admin.login"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
	return user, nil
}

// LoginByEmail logs in a user by email and password.
// B-01/B-02 fix: constant-time comparison + unified error to prevent timing attacks.
// The password argument must be the plain-text password; legacy MD5 hashes are
// transparently migrated to bcrypt on successful login.
func (uc *AdminUsecase) LoginByEmail(ctx context.Context, email, password string) (*Admin, error) {
	user, err := uc.reader.FindByEmail(ctx, email)
	if err != nil {
		uc.lg.Warn("admin login failed",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "email"))
		return nil, ErrInvalidCredentials
	}
	if !verifyPassword(user.Password, password) {
		uc.lg.Warn("admin login failed: invalid credentials",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
		return nil, ErrInvalidCredentials
	}
	if isLegacyMD5Hash(user.Password) {
		uc.migratePasswordToBcrypt(ctx, user, password)
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
