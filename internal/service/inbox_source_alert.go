package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/inbox"
	"aranea-agents/internal/biz/monitor/alert"
	"aranea-agents/pkg/apierror"
)

// 正在触发的平台告警进统一收件箱（运行与治理审查 P1，2026-11-19）。
// 此前告警只写事件表，三条默认规则也都没配通知对象，告警响了没人知道。
//
// 「正在触发」与健康与告警页同一口径：规则启用，且指标当前值达到阈值。不用引擎里存的
// firing_state——窗口里没数据时引擎刻意不恢复，状态会一直停在 firing（本机曾停了 3 天），
// 拿它进收件箱会把早已没有证据的旧告警当成眼下的事。
// 告警不能手动「处理掉」，指标回落后自然消失，所以条目不带决议动作，只带去处。
const (
	inboxSourceMonitorAlert = "monitor_alert"
	inboxPermMonitor        = "system.monitor"
)

type inboxAlertSource struct {
	monitor *MonitorService
}

// NewInboxAlertSource 返回「正在触发的平台告警」收件箱来源；monitor 为 nil 时返回 nil（聚合器会跳过）。
func NewInboxAlertSource(monitor *MonitorService) inbox.Source {
	if monitor == nil {
		return nil
	}
	return &inboxAlertSource{monitor: monitor}
}

// RegisterInboxSource 在启动装配阶段给统一收件箱追加一个来源；收件箱未配置时忽略。
func (s *ApprovalCenterService) RegisterInboxSource(src inbox.Source) {
	if s == nil || s.inbox == nil {
		return
	}
	s.inbox.Register(src)
}

func (s *inboxAlertSource) Name() string { return inboxSourceMonitorAlert }

func (s *inboxAlertSource) List(ctx context.Context, limit int) ([]inbox.Item, int64, error) {
	if !inboxCan(ctx, inboxPermMonitor) {
		return nil, 0, nil
	}
	rules, err := s.monitor.uc.ListAlertRulesWithDefaults(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := inboxFiringAlerts(rules, s.monitor.uc.ListAlertMetricCatalog(ctx), limit)
	return items, int64(len(items)), nil
}

// inboxFiringAlerts 是纯函数：启用、且指标当前值 ≥ 阈值的规则；critical 记高风险。
func inboxFiringAlerts(rules []alert.AlertRule, catalog []alert.AlertMetricCatalogEntry, limit int) []inbox.Item {
	current := make(map[string]alert.AlertMetricCatalogEntry, len(catalog))
	for _, m := range catalog {
		current[m.Key] = m
	}
	out := make([]inbox.Item, 0)
	for _, r := range rules {
		m, ok := current[r.MetricKey]
		if !r.Enabled || !ok || m.CurrentValue < r.Threshold {
			continue
		}
		risk := inbox.RiskMedium
		if r.Severity == "critical" {
			risk = inbox.RiskHigh
		}
		item := inbox.Item{
			NativeID:  r.ID,
			Kind:      inbox.KindException,
			Title:     inboxClip("平台告警："+r.Name, 80),
			Summary:   fmt.Sprintf("%s 当前 %s，阈值 %s", r.MetricKey, alertValue(m.Unit, m.CurrentValue), alertValue(m.Unit, r.Threshold)),
			Risk:      risk,
			Requester: "monitor",
			CreatedAt: m.EvaluatedAt,
			Link:      "/monitor/health",
		}
		if r.LastFiredAt != nil {
			item.CreatedAt = *r.LastFiredAt
		}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// alertValue：比率显示成百分比，计数显示整数（与前端 formatMetricValue 一致）。
func alertValue(unit string, v float64) string {
	if unit == "ratio" {
		return fmt.Sprintf("%.1f%%", v*100)
	}
	return fmt.Sprintf("%.0f", v)
}

func (s *inboxAlertSource) Decide(context.Context, inbox.Item, string, string) error {
	return apierror.BadRequest(apierror.DomainMonitor, "告警在指标回落后自动消失，不能手动处理")
}
