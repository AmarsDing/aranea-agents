package fleet

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// 调度规则（本地时区）：
//
//	every 30m | every 6h | hourly
//	daily 02:00 | weekdays 09:30 | weekly mon 09:00
type schedule struct {
	every   time.Duration
	kind    string // every | daily | weekdays | weekly
	weekday time.Weekday
	hour    int
	minute  int
}

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
	"周日": time.Sunday, "周一": time.Monday, "周二": time.Tuesday, "周三": time.Wednesday,
	"周四": time.Thursday, "周五": time.Friday, "周六": time.Saturday,
}

func parseSchedule(raw string) (schedule, error) {
	f := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	bad := fmt.Errorf("无法识别的计划 %q（示例：every 6h / daily 02:00 / weekdays 09:30 / weekly mon 09:00）", raw)
	if len(f) == 0 {
		return schedule{}, bad
	}
	switch f[0] {
	case "hourly":
		return schedule{kind: "every", every: time.Hour}, nil
	case "every":
		if len(f) != 2 {
			return schedule{}, bad
		}
		d, err := time.ParseDuration(f[1])
		if err != nil || d < 10*time.Minute {
			return schedule{}, fmt.Errorf("间隔至少 10m：%q", raw)
		}
		return schedule{kind: "every", every: d}, nil
	case "daily", "weekdays":
		if len(f) != 2 {
			return schedule{}, bad
		}
		h, m, err := parseClock(f[1])
		if err != nil {
			return schedule{}, bad
		}
		return schedule{kind: f[0], hour: h, minute: m}, nil
	case "weekly":
		if len(f) != 3 {
			return schedule{}, bad
		}
		wd, ok := weekdays[f[1]]
		h, m, err := parseClock(f[2])
		if !ok || err != nil {
			return schedule{}, bad
		}
		return schedule{kind: "weekly", weekday: wd, hour: h, minute: m}, nil
	}
	return schedule{}, bad
}

func parseClock(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad clock")
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("bad clock")
	}
	return h, m, nil
}

// next 返回 after 之后的下一次触发时间。
func (s schedule) next(after time.Time) time.Time {
	after = after.Local()
	if s.kind == "every" {
		return after.Add(s.every).Truncate(time.Minute)
	}
	day := time.Date(after.Year(), after.Month(), after.Day(), s.hour, s.minute, 0, 0, time.Local)
	for i := 0; i < 8; i++ {
		cand := day.AddDate(0, 0, i)
		if !cand.After(after) {
			continue
		}
		switch s.kind {
		case "weekdays":
			if cand.Weekday() == time.Saturday || cand.Weekday() == time.Sunday {
				continue
			}
		case "weekly":
			if cand.Weekday() != s.weekday {
				continue
			}
		}
		return cand
	}
	return after.Add(24 * time.Hour)
}

// ListAutomations 返回全部定时任务。
func (u *Usecase) ListAutomations(ctx context.Context) ([]Automation, error) {
	items, err := listDocs[Automation](ctx, u.store, kindAutomation)
	if err != nil {
		return nil, err
	}
	sortByCreated(items, func(a Automation) string { return a.CreatedAt })
	return items, nil
}

// UpsertAutomation 新建或修改定时任务，并重算下次触发时间。
func (u *Usecase) UpsertAutomation(ctx context.Context, a Automation) (Automation, error) {
	if strings.TrimSpace(a.ProjectID) == "" || strings.TrimSpace(a.Prompt) == "" {
		return Automation{}, apierror.BadRequest(domain, "项目和提示不能为空")
	}
	sch, err := parseSchedule(a.Schedule)
	if err != nil {
		return Automation{}, apierror.BadRequest(domain, err.Error())
	}
	if a.Agent == "" {
		a.Agent = "claude"
	}
	if a.WorktreeMode != "primary" {
		a.WorktreeMode = "new"
	}
	if strings.TrimSpace(a.Name) == "" {
		a.Name = truncate(a.Prompt, 20)
	}
	if a.ID == "" {
		a.ID, a.CreatedAt = uuid.NewString(), nowString()
	} else if old, ok, _ := getDoc[Automation](ctx, u.store, kindAutomation, a.ID); ok {
		a.CreatedAt, a.LastRunAt, a.LastStatus, a.LastWorkspaceID = old.CreatedAt, old.LastRunAt, old.LastStatus, old.LastWorkspaceID
	}
	a.NextRunAt = ""
	if a.Enabled {
		a.NextRunAt = sch.next(time.Now()).UTC().Format(time.RFC3339)
	}
	return a, putDoc(ctx, u.store, kindAutomation, a.ID, a)
}

// DeleteAutomation 删除定时任务（已跑出来的 worktree 不动）。
func (u *Usecase) DeleteAutomation(ctx context.Context, id string) error {
	return u.store.Delete(ctx, kindAutomation, id)
}

// RunAutomationNow 立刻跑一次（不影响下次计划时间）。
func (u *Usecase) RunAutomationNow(ctx context.Context, id string) (Automation, error) {
	a, ok, err := getDoc[Automation](ctx, u.store, kindAutomation, id)
	if err != nil {
		return Automation{}, err
	}
	if !ok {
		return Automation{}, apierror.NotFound(domain, "automation not found")
	}
	return u.runAutomation(ctx, a, false), nil
}

func (u *Usecase) automationLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case now := <-t.C:
			items, err := u.ListAutomations(u.ctx)
			if err != nil {
				continue
			}
			for _, a := range items {
				if !a.Enabled || a.NextRunAt == "" {
					continue
				}
				due, err := time.Parse(time.RFC3339, a.NextRunAt)
				if err != nil || now.Before(due) {
					continue
				}
				u.runAutomation(u.ctx, a, true)
			}
		}
	}
}

func (u *Usecase) runAutomation(ctx context.Context, a Automation, scheduled bool) Automation {
	now := time.Now()
	a.LastRunAt, a.LastStatus = now.UTC().Format(time.RFC3339), "started"
	if scheduled {
		if sch, err := parseSchedule(a.Schedule); err == nil {
			a.NextRunAt = sch.next(now).UTC().Format(time.RFC3339)
		}
	}
	var runErr error
	if a.WorktreeMode == "primary" {
		var info TerminalInfo
		info, runErr = u.CreateTerminal(ctx, CreateTerminalInput{WorkspaceID: primaryID(a.ProjectID), Agent: a.Agent, Prompt: a.Prompt, FullAccess: a.FullAccess, Title: "自动化 · " + a.Name})
		if runErr == nil {
			a.LastWorkspaceID = info.WorkspaceID
		}
	} else {
		var w Workspace
		w, runErr = u.CreateWorkspace(ctx, CreateWorkspaceInput{
			ProjectID: a.ProjectID, Name: a.Name + " " + now.Format("0102-1504"), Agent: a.Agent,
			Prompt: a.Prompt, FullAccess: a.FullAccess, Origin: "automation",
		})
		if runErr == nil {
			a.LastWorkspaceID = w.ID
		}
	}
	title := "自动化已启动：" + a.Name
	if runErr != nil {
		a.LastStatus, title = "failed: "+runErr.Error(), "自动化启动失败："+a.Name
		u.lg.Warn("fleet automation failed", loggateway.Str("automation", a.ID), loggateway.Err(runErr))
	}
	u.addEvent(Event{Kind: EventAutomation, Title: title, Body: truncate(a.Prompt, 120), WorkspaceID: a.LastWorkspaceID})
	_ = putDoc(ctx, u.store, kindAutomation, a.ID, a)
	return a
}
