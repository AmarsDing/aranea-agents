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
//
// P1 权限底座扩展（三项目整合，P0 详设 01 §1.1）：逻辑名 User，物理表 admins
// 保留。Roles 为角色码读缓存（权威来源 user_roles 边表）；Clearance 为密级资质
// L1-L5；TokenVersion 为会话吊销版本（踢会话/改密/离职/角色密级变更即递增）。
type Admin struct {
	ID           int64
	Name         string
	Email        string
	Password     string
	Access       string
	Avatar       string
	WorkspaceID  string
	Roles        []string
	Clearance    int
	Dept         string
	Status       string
	ForceReset   bool
	TokenVersion int64
	CreateTime   time.Time
	UpdateTime   time.Time
}

// 账号状态取值（与 ent schema admin.go Enum 对齐）。
const (
	AdminStatusActive     = "active"
	AdminStatusFrozen     = "frozen"
	AdminStatusOffboarded = "offboarded"
)

// IsActive reports whether the account may authenticate.
func (a *Admin) IsActive() bool {
	return a == nil || a.Status == "" || a.Status == AdminStatusActive
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

// SessionRevoker 吊销指定用户的全部会话（P1-3；实现位于 data 层
// TokenVersionStore：DB 递增 + Redis write-through + pub/sub 广播）。
// 触发点：踢会话 / 改密（P1-8）/ 离职冻结 / 角色或密级变更。
type SessionRevoker interface {
	// RevokeSessions 递增 token_version，返回递增后的新版本。
	RevokeSessions(ctx context.Context, userID int64) (newVersion int64, err error)
}

// AdminUsecase is a Admin usecase.
// W-1 fix: depend on narrow interfaces instead of composite AdminRepo.
type AdminUsecase struct {
	reader  AdminReader
	writer  AdminWriter
	lg      loggateway.Logger
	revoker SessionRevoker
}

// NewAdminUsecase new a Admin usecase.
// S-1 fix: accept narrow interfaces instead of composite AdminRepo.
func NewAdminUsecase(reader AdminReader, writer AdminWriter, lg loggateway.Logger) *AdminUsecase {
	return &AdminUsecase{reader: reader, writer: writer, lg: lg}
}

// SetSessionRevoker injects the session-revocation backend (composition-root
// wiring; 与 SetTimeoutHandler 同模式的可选接线）。生产环境必接；未注入时
// RevokeSessions 返回错误（诚实失败，绝不静默假吊销）。
func (uc *AdminUsecase) SetSessionRevoker(r SessionRevoker) {
	uc.revoker = r
}

// RevokeSessions 吊销 userID 的全部会话：旧 token 在下一次请求即被拒绝
// （HTTP/gRPC 中间件逐请求比对 tv），WS 连接经 pub/sub 主动关闭（P1-4）。
func (uc *AdminUsecase) RevokeSessions(ctx context.Context, userID int64, reason string) error {
	if uc.revoker == nil {
		uc.lg.Warn("session revocation unavailable: revoker not wired",
			loggateway.StepID("admin.session_revoke"), loggateway.Int64("user_id", userID))
		return apierror.Internal("AUTH", "session revocation unavailable")
	}
	newVersion, err := uc.revoker.RevokeSessions(ctx, userID)
	if err != nil {
		return err
	}
	uc.lg.Info("admin sessions revoked",
		loggateway.StepID("admin.session_revoke"),
		loggateway.Int64("user_id", userID),
		loggateway.Int64("token_version", newVersion),
		loggateway.Str("reason", reason))
	return nil
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
	// P1: frozen/offboarded 账号拒绝登录；与密码错误同错误，防账号存在性枚举。
	if !user.IsActive() {
		uc.lg.Warn("admin login rejected: account not active",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "username"), loggateway.Str("admin_name", user.Name))
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
	// P1: frozen/offboarded 账号拒绝登录；与密码错误同错误，防账号存在性枚举。
	if !user.IsActive() {
		uc.lg.Warn("admin login rejected: account not active",
			loggateway.StepID("admin.login_failed"), loggateway.Str("method", "email"), loggateway.Str("admin_name", user.Name))
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
//
// 顺序刻意为「先吊销后删除」（评审 2026-11-09 H1，与 ChangePassword 同理）：
// 删除后 HTTP/gRPC 面因 ErrAdminNotFound fail-closed 立即 401；WS 面双保险：
// ①本函数先递增 tv 广播 bump，在线连接即时被关（主路径）；②若有连接错过
// 事件，兜底扫描在下一个周期把 ErrAdminNotFound 视为已吊销主动关闭
// （ws_revocation.go，最终一致）。
// 吊销失败则不删并如实报错（绝不留下「账号已删但会话仍活」的窗口）；
// revoker 未接线时整个操作不动任何数据。
func (uc *AdminUsecase) DeleteAdmin(ctx context.Context, id int64) error {
	if err := uc.RevokeSessions(ctx, id, "admin_deleted"); err != nil {
		return err
	}
	return uc.writer.DeleteAdmin(ctx, id)
}

// MinPasswordLength 是自助改密与管理员建号共用的最小长度（P1-8）。
const MinPasswordLength = 8

// ChangePassword 自助改密（P1-8）：校验旧密码 → 吊销该用户全部会话 → 写入新
// bcrypt 口令。
//
// 顺序刻意为「先吊销后改密」：若写入口令失败，用户只是被登出（旧口令仍可用），
// 而不是「口令已改但旧会话仍活着」——后者是安全缺口，前者只是可用性抖动。
// 同理，revoker 未接线时整个操作不动任何数据并如实报错（绝不静默假吊销）。
//
// 成功返回后调用方（含发起本次请求的会话）已全部失效，必须重新登录。
func (uc *AdminUsecase) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return apierror.BadRequest("AUTH", "new password is required")
	}
	if len(newPassword) < MinPasswordLength {
		return apierror.BadRequest("AUTH", "new password is too short")
	}
	if newPassword == oldPassword {
		return apierror.BadRequest("AUTH", "new password must differ from the old one")
	}
	user, err := uc.reader.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if !user.IsActive() {
		// 冻结/离职账号不允许自助改密（同时也是防越权的兜底）。
		return ErrInvalidCredentials
	}
	if !verifyPassword(user.Password, oldPassword) {
		uc.lg.Warn("admin change password rejected: invalid old password",
			loggateway.StepID("admin.change_password"), loggateway.Int64("user_id", userID))
		return ErrInvalidCredentials
	}
	if err := uc.RevokeSessions(ctx, userID, "password_change"); err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return apierror.Internal("AUTH", "hash password failed")
	}
	user.Password = string(hashed)
	// 改密即解除首次登录强制改密标记（P1-9 迁移账号置位）。
	user.ForceReset = false
	if _, err := uc.writer.UpdateAdmin(ctx, user); err != nil {
		return err
	}
	uc.lg.Info("admin password changed; all sessions revoked",
		loggateway.StepID("admin.change_password"), loggateway.Int64("user_id", userID))
	return nil
}
