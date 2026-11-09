package main

// office 套件 providers（E 阶段 W1c，需求文档 §2，拍板 K-1..K-5）。
//
// 本文件不带构建标签：wire.go/wire_part2.go（wireinject）与 wire_gen.go
// （!wireinject，手工镜像）共享同一份 provider 定义，循 wire_approval.go 先例；
// 改动只需维护本文件 + wire_gen.go 的装配调用。

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/approval"
	"aranea-agents/internal/biz/office"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
	"aranea-agents/internal/tools/calendar"
	loggateway "aranea-agents/pkg/loggateway"
)

// provideOfficeRepo 装配 office 套件仓储（raw-SQL，迁移 20261321；循
// provideApprovalMatrix 内联 NewApprovalRiskMatrixRepo 先例，不进 data.ProviderSet）。
func provideOfficeRepo(d *data.Data) *data.OfficeRepo {
	return data.NewOfficeRepo(d)
}

// provideScheduleDraftReader 将 office 仓储窄化为审批中心日程草案读口
// （签名与 office.DraftRepo 对应方法一致，断言恒成功）。
func provideScheduleDraftReader(repo *data.OfficeRepo) approval.ScheduleDraftReader {
	return repo
}

func provideOfficeScheduleUsecase(repo *data.OfficeRepo) *office.ScheduleUsecase {
	return office.NewScheduleUsecase(repo)
}

func provideOfficeDraftUsecase(repo *data.OfficeRepo) *office.DraftUsecase {
	return office.NewDraftUsecase(repo)
}

func provideOfficeNotifyUsecase(repo *data.OfficeRepo) *office.NotifyUsecase {
	return office.NewNotifyUsecase(repo)
}

func provideOfficeICalUsecase(repo *data.OfficeRepo) *office.ICalUsecase {
	return office.NewICalUsecase(repo, repo)
}

func provideOfficeAppsUsecase(repo *data.OfficeRepo) *office.AppsUsecase {
	return office.NewAppsUsecase(repo)
}

// provideOfficeReminderWorker 装配提醒扫描 worker（K-4）。IM 投递腿经
// officeReminderDelivery 适配 channel 域；channel 不可用时不应发生（biz
// 基础域），故硬依赖而非 nil 降级。
func provideOfficeReminderWorker(repo *data.OfficeRepo, channels *biz.ChannelUsecase, lg loggateway.Logger) *office.ReminderWorker {
	return office.NewReminderWorker(repo, repo, officeReminderDelivery{channels: channels}, lg)
}

func provideOfficeScheduleService(s *office.ScheduleUsecase, d *office.DraftUsecase, i *office.ICalUsecase) *service.OfficeScheduleService {
	return service.NewOfficeScheduleService(s, d, i)
}

func provideOfficeNotifyService(n *office.NotifyUsecase) *service.OfficeNotifyService {
	return service.NewOfficeNotifyService(n)
}

func provideOfficeAppsService(a *office.AppsUsecase) *service.OfficeAppsService {
	return service.NewOfficeAppsService(a)
}

// provideOfficeCalendarTool 装配 agent 日程工具窄口（calendar_list /
// calendar_create_draft；service 层 session→user 解析适配），注入
// ToolBridges.Calendar。任一为 nil → nil（工具被裁剪，诚实降级）。
func provideOfficeCalendarTool(s *office.ScheduleUsecase, d *office.DraftUsecase, sessions *biz.SessionUsecase) calendar.Service {
	return service.ProvideOfficeCalendarTool(s, d, sessions)
}

// ============================================================================
// officeReminderDelivery —— 提醒 IM 投递适配（office.ReminderDelivery ←
// biz.ChannelUsecase）。接收点声明式：channel MetadataJSON
// {"reminder_sink": true, "reminder_recipient": "<平台接收方 id>"}；
// 工作区过滤=共享通道（workspace 空）或属主空间；停用/已删通道跳过。
// ============================================================================

type officeReminderDelivery struct {
	channels *biz.ChannelUsecase
}

type reminderChannelMeta struct {
	ReminderSink      bool   `json:"reminder_sink"`
	ReminderRecipient string `json:"reminder_recipient"`
}

func (d officeReminderDelivery) ListSinks(ctx context.Context, workspaceID string) ([]office.ReminderSink, error) {
	chs, err := d.channels.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]office.ReminderSink, 0)
	for _, ch := range chs {
		if !ch.Enabled || ch.DeletedAt != "" {
			continue
		}
		if ch.WorkspaceID != "" && ch.WorkspaceID != workspaceID {
			continue
		}
		var meta reminderChannelMeta
		if ch.MetadataJSON == "" || json.Unmarshal([]byte(ch.MetadataJSON), &meta) != nil {
			continue
		}
		if !meta.ReminderSink || strings.TrimSpace(meta.ReminderRecipient) == "" {
			continue
		}
		plat := biz.ChannelTypeFromConfig(ch.ConfigJSON)
		if plat == "" {
			plat = ch.Key
		}
		out = append(out, office.ReminderSink{ChannelID: ch.ID, Platform: plat, Recipient: meta.ReminderRecipient})
	}
	return out, nil
}

func (d officeReminderDelivery) Enqueue(ctx context.Context, sink office.ReminderSink, text, idempotencyKey string) error {
	_, _, err := d.channels.EnqueueOutboundDelivery(ctx, sink.ChannelID, biz.ChannelOutboundPayload{
		Kind:           biz.ChannelOutboundTextKind,
		Platform:       sink.Platform,
		Recipient:      sink.Recipient,
		Text:           text,
		IdempotencyKey: idempotencyKey,
	})
	return err
}
