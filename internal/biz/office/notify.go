package office

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// NotifyUsecase 通知中心 MVP：应用内收件箱（只读 + 已读状态管理）。
// 写路径不开放 RPC——生产方（提醒扫描/邮件摘要/审批待办 job）经
// CreateNotification 进程内写入。
type NotifyUsecase struct {
	repo NotificationRepo
}

func NewNotifyUsecase(repo NotificationRepo) *NotifyUsecase {
	return &NotifyUsecase{repo: repo}
}

// CreateNotification 生产方写口（进程内；kind 非法时归 system，诚实不拒）。
func (u *NotifyUsecase) CreateNotification(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = "ntf_" + uuid.NewString()
	}
	if n.Kind == "" {
		n.Kind = NotifyKindSystem
	}
	return u.repo.CreateNotification(ctx, n)
}

// List 本人通知分页（unreadOnly 过滤；total 为过滤后总数，unread 恒为本人
// 未读总数——铃铛徽标不因翻页/过滤抖动）。
func (u *NotifyUsecase) List(ctx context.Context, userID string, unreadOnly bool, page, pageSize int) ([]*Notification, int64, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return u.repo.ListNotifications(ctx, userID, unreadOnly, page, pageSize)
}

func (u *NotifyUsecase) Dismiss(ctx context.Context, id, userID string) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotificationNotFound
	}
	return u.repo.DismissNotification(ctx, id, userID)
}

func (u *NotifyUsecase) MarkAllRead(ctx context.Context, userID string) error {
	return u.repo.MarkAllNotificationsRead(ctx, userID)
}
