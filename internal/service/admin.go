package service

import (
	v1 "aranea-agents/api/kratos/admin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"time"

	"aranea-agents/pkg/apierror"

	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
	"go.einride.tech/aip/pagination"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// encodePassword hashes a plain password with bcrypt. Returns the empty string
// when the input is empty so callers can distinguish "no password change" from
// "set password to empty". bcrypt is non-deterministic (salted), so two calls
// with the same input produce different hashes.
func encodePassword(password string) string {
	if password == "" {
		return ""
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// bcrypt.GenerateFromPassword only fails for invalid cost; DefaultCost
		// is always valid. Treat any failure as fatal to avoid storing an
		// unhashed password.
		panic(fmt.Sprintf("encodePassword: bcrypt hash failed: %v", err))
	}
	return string(hashed)
}

func convertAdmin(m *biz.Admin) *v1.Admin {
	return &v1.Admin{
		Id:          m.ID,
		Name:        m.Name,
		Email:       m.Email,
		Avatar:      m.Avatar,
		Access:      m.Access,
		WorkspaceId: m.WorkspaceID,
		CreateTime:  timestamppb.New(m.CreateTime),
		UpdateTime:  timestamppb.New(m.UpdateTime),
	}
}

// AdminService is a greeter service.
type AdminService struct {
	v1.UnimplementedAdminServiceServer

	uc *biz.AdminUsecase
	lg loggateway.Logger
}

func NewAdminService(uc *biz.AdminUsecase, lg loggateway.Logger) *AdminService {
	return &AdminService{uc: uc, lg: lg}
}

// Current implements auth current admin retrieval.
func (s *AdminService) Current(ctx context.Context, req *emptypb.Empty) (*v1.Admin, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	admin, err := s.uc.GetAdmin(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	return convertAdmin(admin), nil
}

// Login implements auth login.
func (s *AdminService) Login(ctx context.Context, req *v1.LoginRequest) (*v1.Admin, error) {
	var (
		err    error
		admin  *biz.Admin
		method string
	)
	switch v := req.Identity.(type) {
	case *v1.LoginRequest_Username:
		method = "username"
		admin, err = s.uc.LoginByUsername(ctx, v.Username, req.Password)
		if err != nil {
			return nil, err
		}
	case *v1.LoginRequest_Email:
		method = "email"
		admin, err = s.uc.LoginByEmail(ctx, v.Email, req.Password)
		if err != nil {
			return nil, err
		}
	default:
		s.lg.Warn("admin login failed: unsupported identity type",
			loggateway.StepID("admin.login_failed"))
		return nil, apierror.BadRequest("AUTH", "unsupported identity type")
	}
	if err := auth.SetCookieForWorkspace(ctx, admin.ID, admin.Access, admin.WorkspaceID, time.Now().Add(7*24*time.Hour)); err != nil {
		s.lg.Warn("admin login: set cookie failed",
			loggateway.StepID("admin.login"), loggateway.Str("method", method), loggateway.Str("admin_name", admin.Name), loggateway.Err(err))
		return nil, err
	}
	return convertAdmin(admin), nil
}

// Logout implements auth logout.
func (s *AdminService) Logout(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if err := s.uc.Logout(ctx, a.UserID); err != nil {
		return nil, err
	}
	if err := auth.DeleteCookie(ctx); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// CreateAdmin implements admin creation.
func (s *AdminService) CreateAdmin(ctx context.Context, req *v1.CreateAdminRequest) (*v1.Admin, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	if req.GetAdmin() == nil {
		return nil, apierror.BadRequest("AUTH", "admin field is required")
	}
	pwd := req.Admin.Password
	if pwd != "" {
		pwd = encodePassword(pwd)
	}
	admin, err := s.uc.CreateAdmin(ctx, &biz.Admin{
		Name:     req.Admin.Name,
		Email:    req.Admin.Email,
		Password: pwd,
		Avatar:   req.Admin.Avatar,
		Access:   req.Admin.Access,
	})
	if err != nil {
		return nil, err
	}
	s.lg.Info("admin created",
		loggateway.StepID("admin.create"), loggateway.Str("operator_id", fmt.Sprintf("%d", a.UserID)), loggateway.Str("target", admin.Name))
	return convertAdmin(admin), nil
}

// UpdateAdmin implements admin update.
func (s *AdminService) UpdateAdmin(ctx context.Context, req *v1.UpdateAdminRequest) (*v1.Admin, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	if req.GetAdmin() == nil {
		return nil, apierror.BadRequest("AUTH", "admin field is required")
	}
	// Encode password if it's not empty
	if req.Admin.Password != "" {
		req.Admin.Password = encodePassword(req.Admin.Password)
	}
	// Call biz layer directly to bypass the redundant permission check in
	// s.GetAdmin (UpdateAdmin already verified HasAdminAccess above).
	bizAdmin, err := s.uc.GetAdmin(ctx, req.Admin.Id)
	if err != nil {
		return nil, err
	}
	admin := convertAdmin(bizAdmin)
	fieldmask.Update(req.UpdateMask, admin, req.Admin)
	updated, err := s.uc.UpdateAdmin(ctx, &biz.Admin{
		ID:       admin.Id,
		Name:     admin.Name,
		Email:    admin.Email,
		Password: admin.Password,
		Avatar:   admin.Avatar,
		Access:   admin.Access,
	})
	if err != nil {
		return nil, err
	}
	s.lg.Info("admin updated",
		loggateway.StepID("admin.update"), loggateway.Str("operator_id", fmt.Sprintf("%d", a.UserID)), loggateway.Str("target", admin.Name))
	return convertAdmin(updated), nil
}

// DeleteAdmin implements admin deletion.
func (s *AdminService) DeleteAdmin(ctx context.Context, req *v1.DeleteAdminRequest) (*emptypb.Empty, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	if err := s.uc.DeleteAdmin(ctx, req.Id); err != nil {
		return nil, err
	}
	s.lg.Info("admin deleted",
		loggateway.StepID("admin.delete"), loggateway.Str("operator_id", fmt.Sprintf("%d", a.UserID)), loggateway.Str("target_id", fmt.Sprintf("%d", req.Id)))
	return &emptypb.Empty{}, nil
}

// GetAdmin implements admin retrieval.
func (s *AdminService) GetAdmin(ctx context.Context, req *v1.GetAdminRequest) (*v1.Admin, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	admin, err := s.uc.GetAdmin(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return convertAdmin(admin), nil
}

// ListAdmins implements admin listing with filtering, ordering, and pagination.
func (s *AdminService) ListAdmins(ctx context.Context, req *v1.ListAdminsRequest) (*v1.AdminSet, error) {
	a, ok := auth.FromContext(ctx)
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	if !a.HasAdminAccess() {
		return nil, auth.ErrForbidden
	}
	declarations, err := filtering.NewDeclarations(
		filtering.DeclareStandardFunctions(),
		filtering.DeclareIdent("name", filtering.TypeString),
		filtering.DeclareIdent("email", filtering.TypeString),
		filtering.DeclareIdent("phone", filtering.TypeString),
		filtering.DeclareIdent("create_time", filtering.TypeTimestamp),
	)
	if err != nil {
		return nil, err
	}
	filter, err := filtering.ParseFilter(req, declarations)
	if err != nil {
		return nil, err
	}
	pageToken, err := pagination.ParsePageToken(req)
	if err != nil {
		return nil, err
	}
	orderBy, err := ordering.ParseOrderBy(req)
	if err != nil {
		return nil, err
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	admins, err := s.uc.ListAdmins(ctx,
		biz.ListFilter(filter),
		biz.ListOrderBy(orderBy),
		biz.ListLimit(int(req.PageSize)),
		biz.ListOffset(int(pageToken.Offset)),
	)
	if err != nil {
		return nil, err
	}
	adminSet := &v1.AdminSet{
		Admins: make([]*v1.Admin, 0, len(admins)),
	}
	if len(admins) >= int(req.PageSize) {
		adminSet.NextPageToken = pageToken.Next(req).String()
	}
	for _, admin := range admins {
		adminSet.Admins = append(adminSet.Admins, convertAdmin(admin))
	}
	return adminSet, nil
}
