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
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/approval"
	cbstore "aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/biz/monitor"
	"aranea-agents/internal/biz/office"
	"aranea-agents/internal/data"
	"aranea-agents/internal/service"
	"aranea-agents/internal/tools/calendar"
	"aranea-agents/internal/tools/mail"
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

// provideOfficeMailTool 装配 agent 邮件工具窄口（mail_list/mail_read/
// mail_search/mail_to_task；service 层 session→user 解析适配），注入
// ToolBridges.Mail。任一为 nil → nil（工具被裁剪，诚实降级）。
func provideOfficeMailTool(m *office.MailMessageUsecase, sessions *biz.SessionUsecase) mail.Service {
	return service.ProvideOfficeMailTool(m, sessions)
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

// ============================================================================
// E 阶段 W1d 邮件 providers（需求文档 §1，拍板 E-1..E-5；与 wire_part2.go
// Build 列表同步镜像，循上方 W1c 段先例）
// ============================================================================

// provideOfficeMailRepo 装配邮件仓储（raw-SQL，迁移 20261322）。
func provideOfficeMailRepo(d *data.Data) *data.OfficeMailRepo {
	return data.NewOfficeMailRepo(d)
}

// provideOfficeMailAccountUsecase 账号 CRUD。E-1：密码信封加解密复用
// credential 设施（*biz.CredentialCrypto 的方法签名与 office.MailSecretCrypto
// 逐字匹配，直接满足窄口，无需适配器）。
func provideOfficeMailAccountUsecase(repo *data.OfficeMailRepo, crypto *biz.CredentialCrypto, lg loggateway.Logger) *office.MailAccountUsecase {
	return office.NewMailAccountUsecase(repo, crypto, lg)
}

// provideOfficeMailSyncUsecase 同步编排（dialer 默认真实 IMAP；mock 由
// 测试注入，不经 wire）。
func provideOfficeMailSyncUsecase(repo *data.OfficeMailRepo, crypto *biz.CredentialCrypto, audit office.MailAuditSink, lg loggateway.Logger) *office.MailSyncUsecase {
	return office.NewMailSyncUsecase(repo, repo, crypto, nil, audit, lg)
}

func provideOfficeMailMessageUsecase(repo *data.OfficeMailRepo, knowledge office.MailKnowledgeSink, lg loggateway.Logger) *office.MailMessageUsecase {
	return office.NewMailMessageUsecase(repo, repo, repo, knowledge, lg)
}

// provideOfficeMailDigestUsecase 摘要编排：通知中心 + IM 腿（复用
// reminder_sink 声明式接收点）+ 作战总台卡片。
func provideOfficeMailDigestUsecase(mailRepo *data.OfficeMailRepo, officeRepo *data.OfficeRepo, channels *biz.ChannelUsecase, cb office.MailCeoboardSink, lg loggateway.Logger) *office.MailDigestUsecase {
	return office.NewMailDigestUsecase(mailRepo, officeRepo, officeReminderDelivery{channels: channels}, cb, lg)
}

func provideOfficeMailIdleSupervisor(repo *data.OfficeMailRepo, syncer *office.MailSyncUsecase, lg loggateway.Logger) *office.MailIdleSupervisor {
	return office.NewMailIdleSupervisor(repo, syncer, lg)
}

func provideOfficeMailDigestWorker(repo *data.OfficeMailRepo, digest *office.MailDigestUsecase, lg loggateway.Logger) *office.MailDigestWorker {
	return office.NewMailDigestWorker(repo, digest, lg)
}

func provideOfficeMailService(a *office.MailAccountUsecase, m *office.MailMessageUsecase, s *office.MailSyncUsecase, d *office.MailDigestUsecase) *service.OfficeMailService {
	return service.NewOfficeMailService(a, m, s, d)
}

// provideOfficeMailAuditSink 邮件域审计窄口（适配 monitor.Usecase）。
func provideOfficeMailAuditSink(mon *biz.MonitorUsecase) office.MailAuditSink {
	return officeMailAuditSink{mon: mon}
}

// provideOfficeMailCeoboardSink 摘要作战总台窄口（适配 ceoboard store；
// UID aranea:mail-digest-<user>-<date> 幂等）。
func provideOfficeMailCeoboardSink(st *cbstore.Store) office.MailCeoboardSink {
	if st == nil {
		return nil
	}
	return officeMailCeoboardSink{st: st}
}

// provideOfficeMailKnowledgeSink 附件入知识库窄口（适配 knowledge 域；
// 密级随行透传——D6 标签体系落位前，classification 记录于文档 Source
// 元数据注释，标签落地后映射）。
func provideOfficeMailKnowledgeSink(k *biz.KnowledgeUsecase, lg loggateway.Logger) office.MailKnowledgeSink {
	if k == nil {
		return nil
	}
	return officeMailKnowledgeSink{knowledge: k, lg: lg}
}

// ============================================================================
// 适配器实现
// ============================================================================

type officeMailAuditSink struct {
	mon *biz.MonitorUsecase
}

func (s officeMailAuditSink) RecordMailAudit(ctx context.Context, action, resourceID, summary string) {
	if s.mon == nil {
		return
	}
	s.mon.RecordAdminAudit(ctx, monitor.AdminAuditEntry{
		Action:     action,
		Resource:   "mail_account",
		ResourceID: resourceID,
		Summary:    summary,
	})
}

type officeMailCeoboardSink struct {
	st *cbstore.Store
}

func (s officeMailCeoboardSink) EmitMailDigest(ctx context.Context, userID, title, body string, occurredAt time.Time) error {
	uid := "aranea:mail-digest-" + userID + "-" + occurredAt.Format("2006-01-02")
	payload, _ := json.Marshal(map[string]any{
		"title":   title,
		"body":    body,
		"user_id": userID,
		"kind":    "mail_digest",
	})
	_, err := s.st.IngestEvent(ctx, &cbstore.Event{
		UID:             uid,
		Platform:        "aranea",
		Subject:         "office.mail.digest.v1",
		SubjectKey:      "office.mail.digest",
		EventID:         uid,
		EntityID:        userID,
		OccurredAt:      occurredAt,
		ReceivedAt:      occurredAt,
		Payload:         string(payload),
		PayloadComplete: true,
	}, nil)
	return err
}

// officeMailKnowledgeCollectionName 附件入库的默认集合名（每工作区
// find-or-create；team 后端=PG 真相源，无需文件系统 vault）。
const officeMailKnowledgeCollectionName = "邮件附件"

type officeMailKnowledgeSink struct {
	knowledge *biz.KnowledgeUsecase
	lg        loggateway.Logger
}

func (s officeMailKnowledgeSink) IngestAttachment(ctx context.Context, in office.MailKnowledgeIngest) (string, error) {
	colID, err := s.resolveCollection(ctx, in.WorkspaceID)
	if err != nil {
		return "", err
	}
	// 文本类附件落 content_text（可检索）；二进制附件 F1 仅元数据行
	// （vault 文件留存随任务中心/对象存储评估，见需求文档 §1.4 F2）。
	var contentText string
	if mailAttachmentTextLike(in.Filename, in.ContentType) {
		content := in.Content
		if len(content) > 1<<20 { // 文本检索面 1MB 截断（超大文本附件罕见）
			content = content[:1<<20]
		}
		contentText = string(content)
	}
	doc, err := s.knowledge.CreateDocument(ctx, biz.KnowledgeDocument{
		CollectionID:   colID,
		Source:         in.Filename,
		MimeType:       in.ContentType,
		SizeBytes:      int64(len(in.Content)),
		ContentText:    contentText,
		Status:         "pending",
		Classification: in.Classification, // D6 L-3 密级随行（落库 + embedding 闸载体）
	})
	if err != nil {
		return "", err
	}
	return doc.ID, nil
}

// resolveCollection find-or-create 默认「邮件附件」集合（team 后端）。
func (s officeMailKnowledgeSink) resolveCollection(ctx context.Context, workspaceID string) (string, error) {
	cols, _, err := s.knowledge.ListCollections(ctx, workspaceID, 500, 0)
	if err != nil {
		return "", err
	}
	for _, c := range cols {
		if c.Name == officeMailKnowledgeCollectionName {
			return c.ID, nil
		}
	}
	col, err := s.knowledge.CreateCollection(ctx, biz.KnowledgeCollection{
		Name:         officeMailKnowledgeCollectionName,
		Description:  "office 邮件附件入库集合（W1d 自动创建；team 后端=PG 真相源）",
		Workspace:    workspaceID,
		VaultBackend: "team",
	})
	if err != nil {
		return "", err
	}
	return col.ID, nil
}

// mailAttachmentTextLike 文本类附件判定（content_text 落库面）。
func mailAttachmentTextLike(filename, contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if strings.HasPrefix(ct, "text/") || ct == "application/json" || ct == "application/xml" {
		return true
	}
	name := strings.ToLower(filename)
	for _, ext := range []string{".txt", ".md", ".csv", ".log", ".json", ".yaml", ".yml"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
