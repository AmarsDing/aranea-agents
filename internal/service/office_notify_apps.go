package service

import (
	"context"
	"time"

	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "aranea-agents/api/kratos/office/v1"
	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
)

// GetICalFeed 公开 feed（proto public:true，不经 JWT 中间件）：token-in-path
// 即凭据；吊销/未知 → 401。输出 text/calendar。
func (s *OfficeScheduleService) GetICalFeed(ctx context.Context, req *v1.GetICalFeedRequest) (*httpbody.HttpBody, error) {
	body, err := s.ical.RenderFeed(ctx, req.GetToken(), time.Now())
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &httpbody.HttpBody{ContentType: "text/calendar; charset=utf-8", Data: []byte(body)}, nil
}

// OfficeNotifyService 通知中心 MVP（只读 + 已读管理；写路径不开放）。
type OfficeNotifyService struct {
	v1.UnimplementedOfficeNotifyServiceServer
	notify *office.NotifyUsecase
}

func NewOfficeNotifyService(n *office.NotifyUsecase) *OfficeNotifyService {
	return &OfficeNotifyService{notify: n}
}

func (s *OfficeNotifyService) ListNotifications(ctx context.Context, req *v1.ListNotificationsRequest) (*v1.ListNotificationsResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	items, total, unread, err := s.notify.List(ctx, userID, req.GetUnreadOnly(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	out := &v1.ListNotificationsResponse{
		Items:       make([]*v1.OfficeNotification, 0, len(items)),
		Total:       total,
		UnreadCount: unread,
	}
	for _, n := range items {
		out.Items = append(out.Items, &v1.OfficeNotification{
			Id:        n.ID,
			UserId:    n.UserID,
			Kind:      n.Kind,
			Title:     n.Title,
			Body:      n.Body,
			Link:      n.Link,
			Read:      n.Read,
			CreatedAt: timestamppb.New(n.CreatedAt),
		})
	}
	return out, nil
}

func (s *OfficeNotifyService) DismissNotification(ctx context.Context, req *v1.DismissNotificationRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.notify.Dismiss(ctx, req.GetId(), userID); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeNotifyService) MarkAllNotificationsRead(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.notify.MarkAllRead(ctx, userID); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

// OfficeAppsService 应用启动器（K-5）。
type OfficeAppsService struct {
	v1.UnimplementedOfficeAppsServiceServer
	apps *office.AppsUsecase
}

func NewOfficeAppsService(a *office.AppsUsecase) *OfficeAppsService {
	return &OfficeAppsService{apps: a}
}

func appToProto(a *office.App) *v1.OfficeApp {
	return &v1.OfficeApp{
		Id:          a.ID,
		WorkspaceId: a.WorkspaceID,
		Name:        a.Name,
		Url:         a.URL,
		Icon:        a.Icon,
		GroupName:   a.GroupName,
		SortOrder:   int32(a.SortOrder),
		CreatedAt:   timestamppb.New(a.CreatedAt),
	}
}

func (s *OfficeAppsService) ListApps(ctx context.Context, _ *emptypb.Empty) (*v1.ListAppsResponse, error) {
	_, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	apps, err := s.apps.List(ctx, ws)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	out := &v1.ListAppsResponse{Items: make([]*v1.OfficeApp, 0, len(apps))}
	for _, a := range apps {
		out.Items = append(out.Items, appToProto(a))
	}
	return out, nil
}

func (s *OfficeAppsService) UpsertApp(ctx context.Context, req *v1.UpsertAppRequest) (*v1.OfficeApp, error) {
	_, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	a, err := s.apps.Upsert(ctx, &office.App{
		ID:          req.GetId(),
		WorkspaceID: ws,
		Name:        req.GetName(),
		URL:         req.GetUrl(),
		Icon:        req.GetIcon(),
		GroupName:   req.GetGroupName(),
		SortOrder:   int(req.GetSortOrder()),
	})
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return appToProto(a), nil
}

func (s *OfficeAppsService) DeleteApp(ctx context.Context, req *v1.DeleteAppRequest) (*emptypb.Empty, error) {
	_, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.apps.Delete(ctx, req.GetId(), ws); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}
