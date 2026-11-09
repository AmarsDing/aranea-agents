package service

import (
	"context"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "aranea-agents/api/kratos/office/v1"
	"aranea-agents/internal/biz/office"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// OfficeScheduleService 日程域 RPC（E 阶段 W1c；proto 见
// api/kratos/office/v1/schedule.proto）。权限由中间件按 proto option 硬判
// （office.schedule）；属主/租户边界取自 JWT 主体（禁请求体自报）。
type OfficeScheduleService struct {
	v1.UnimplementedOfficeScheduleServiceServer
	schedule *office.ScheduleUsecase
	drafts   *office.DraftUsecase
	ical     *office.ICalUsecase
}

func NewOfficeScheduleService(s *office.ScheduleUsecase, d *office.DraftUsecase, i *office.ICalUsecase) *OfficeScheduleService {
	return &OfficeScheduleService{schedule: s, drafts: d, ical: i}
}

// officeSubject 提取 JWT 主体（userID 字符串化 + 有效工作区）；未认证即
// 401（public 端点不经此路径）。
func officeSubject(ctx context.Context) (userID, workspaceID string, err error) {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return "", "", apierror.Unauthorized(office.DomainOffice, "authentication required")
	}
	return strconv.FormatInt(a.UserID, 10), a.EffectiveWorkspaceID(), nil
}

func eventToProto(ev *office.Event) *v1.OfficeEvent {
	return &v1.OfficeEvent{
		Id:              ev.ID,
		WorkspaceId:     ev.WorkspaceID,
		UserId:          ev.UserID,
		TaskRef:         ev.TaskRef,
		TaskTitle:       ev.TaskTitle,
		Title:           ev.Title,
		Note:            ev.Note,
		StartAt:         timestamppb.New(ev.StartAt),
		EndAt:           timestamppb.New(ev.EndAt),
		Timezone:        ev.Timezone,
		ReminderMinutes: int32(ev.ReminderMinutes),
		Source:          ev.Source,
		CreatedAt:       timestamppb.New(ev.CreatedAt),
		UpdatedAt:       timestamppb.New(ev.UpdatedAt),
	}
}

func tsAsTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

func (s *OfficeScheduleService) CreateEvent(ctx context.Context, req *v1.CreateEventRequest) (*v1.OfficeEvent, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	ev, err := s.schedule.CreateEvent(ctx, &office.Event{
		WorkspaceID:     ws,
		UserID:          userID,
		TaskRef:         req.GetTaskRef(),
		Title:           req.GetTitle(),
		Note:            req.GetNote(),
		StartAt:         tsAsTime(req.GetStartAt()),
		EndAt:           tsAsTime(req.GetEndAt()),
		Timezone:        req.GetTimezone(),
		ReminderMinutes: int(req.GetReminderMinutes()),
		Source:          office.SourceManual,
	})
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return eventToProto(ev), nil
}

func (s *OfficeScheduleService) UpdateEvent(ctx context.Context, req *v1.UpdateEventRequest) (*v1.OfficeEvent, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	ev, err := s.schedule.UpdateEvent(ctx, userID, &office.Event{
		ID:              req.GetId(),
		Title:           req.GetTitle(),
		Note:            req.GetNote(),
		StartAt:         tsAsTime(req.GetStartAt()),
		EndAt:           tsAsTime(req.GetEndAt()),
		Timezone:        req.GetTimezone(),
		ReminderMinutes: int(req.GetReminderMinutes()),
	})
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return eventToProto(ev), nil
}

func (s *OfficeScheduleService) DeleteEvent(ctx context.Context, req *v1.DeleteEventRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.schedule.DeleteEvent(ctx, req.GetId(), userID); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeScheduleService) GetEvent(ctx context.Context, req *v1.GetEventRequest) (*v1.OfficeEvent, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	ev, err := s.schedule.GetEvent(ctx, req.GetId(), userID)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return eventToProto(ev), nil
}

func (s *OfficeScheduleService) ListEventsRange(ctx context.Context, req *v1.ListEventsRangeRequest) (*v1.ListEventsRangeResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	events, err := s.schedule.ListEventsRange(ctx, userID, tsAsTime(req.GetStart()), tsAsTime(req.GetEnd()))
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	out := &v1.ListEventsRangeResponse{Items: make([]*v1.OfficeEvent, 0, len(events))}
	for _, ev := range events {
		out.Items = append(out.Items, eventToProto(ev))
	}
	return out, nil
}

func (s *OfficeScheduleService) ScheduleTask(ctx context.Context, req *v1.ScheduleTaskRequest) (*v1.OfficeEvent, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	ev, err := s.schedule.ScheduleTask(ctx, ws, userID, req.GetTaskRef(), req.GetTaskTitle(),
		tsAsTime(req.GetStartAt()), tsAsTime(req.GetEndAt()), int(req.GetReminderMinutes()))
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return eventToProto(ev), nil
}

func (s *OfficeScheduleService) ListTaskEvents(ctx context.Context, req *v1.ListTaskEventsRequest) (*v1.ListTaskEventsResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	events, err := s.schedule.ListTaskEvents(ctx, userID, req.GetTaskRef())
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	out := &v1.ListTaskEventsResponse{Items: make([]*v1.OfficeEvent, 0, len(events))}
	for _, ev := range events {
		out.Items = append(out.Items, eventToProto(ev))
	}
	return out, nil
}

func (s *OfficeScheduleService) SubmitScheduleDraft(ctx context.Context, req *v1.SubmitScheduleDraftRequest) (*v1.SubmitScheduleDraftResponse, error) {
	userID, ws, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	items := make([]office.DraftItem, 0, len(req.GetItems()))
	for _, it := range req.GetItems() {
		items = append(items, office.DraftItem{
			Title:           it.GetTitle(),
			Note:            it.GetNote(),
			StartAt:         tsAsTime(it.GetStartAt()),
			EndAt:           tsAsTime(it.GetEndAt()),
			ReminderMinutes: int(it.GetReminderMinutes()),
			TaskRef:         it.GetTaskRef(),
		})
	}
	d, err := s.drafts.SubmitDraft(ctx, ws, userID, req.GetAgentId(), req.GetSessionId(), items)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &v1.SubmitScheduleDraftResponse{ApprovalId: d.ID}, nil
}

func (s *OfficeScheduleService) ApproveScheduleDraft(ctx context.Context, req *v1.ApproveScheduleDraftRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.drafts.ApproveDraft(ctx, req.GetId(), userID); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeScheduleService) RejectScheduleDraft(ctx context.Context, req *v1.RejectScheduleDraftRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.drafts.RejectDraft(ctx, req.GetId(), userID, req.GetReason()); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeScheduleService) CreateICalToken(ctx context.Context, req *v1.CreateICalTokenRequest) (*v1.ICalToken, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	t, plain, err := s.ical.CreateToken(ctx, userID, req.GetName())
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	// feed_url 为相对路径（前端拼 origin）；明文 token 仅此响应出现一次。
	return &v1.ICalToken{
		Id:        t.ID,
		Name:      t.Name,
		FeedUrl:   office.FeedPath(plain),
		CreatedAt: timestamppb.New(t.CreatedAt),
	}, nil
}

func (s *OfficeScheduleService) RevokeICalToken(ctx context.Context, req *v1.RevokeICalTokenRequest) (*emptypb.Empty, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	if err := s.ical.RevokeToken(ctx, req.GetId(), userID); err != nil {
		return nil, apierror.ToKratos(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *OfficeScheduleService) ListICalTokens(ctx context.Context, _ *emptypb.Empty) (*v1.ListICalTokensResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	tokens, err := s.ical.ListTokens(ctx, userID)
	if err != nil {
		return nil, apierror.ToKratos(err)
	}
	out := &v1.ListICalTokensResponse{Items: make([]*v1.ICalToken, 0, len(tokens))}
	for _, t := range tokens {
		// 列表永不回显明文路径（token 不可恢复）；revoked 态带时间戳。
		item := &v1.ICalToken{Id: t.ID, Name: t.Name, CreatedAt: timestamppb.New(t.CreatedAt)}
		if t.RevokedAt != nil {
			item.RevokedAt = timestamppb.New(*t.RevokedAt)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
