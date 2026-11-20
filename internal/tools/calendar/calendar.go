// Package calendar 实现 agent 侧日程两动词（E 阶段 W1c，需求文档 §2.3）：
//
//   - calendar_list          — 读本会话属主的时间窗事件（只读，低风险）。
//   - calendar_create_draft  — 提交排期草案（不落事件；草案进审批中心
//     schedule_draft lane，人批准后方落 office_events——审批中心即闸门，
//     故工具本身不设 reqConfirm）。
//
// 身份解析在 service 层适配口完成：工具只带 GateSessionID 注入的 sessionID，
// user/workspace 由适配器查会话落位（agent 不可自报身份）。
//
// 装配：tools 表种子 enabled=false（按 agent 授予）；Service 为 nil 时
// 工具集被裁剪（诚实降级，不伪造空表）。
package calendar

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// EventView 是日历事件的工具结果投影（仅展示字段，不含内部列）。
type EventView struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Note            string    `json:"note,omitempty"`
	StartAt         time.Time `json:"start_at"`
	EndAt           time.Time `json:"end_at"`
	Timezone        string    `json:"timezone"`
	ReminderMinutes int       `json:"reminder_minutes,omitempty"`
	Source          string    `json:"source"`
	TaskRef         string    `json:"task_ref,omitempty"`
	// Kind plan = 打算做的，log = 已经做了的工作记录；Status todo/doing/done/cancelled。
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	AllDay  bool   `json:"all_day,omitempty"`
	Outcome string `json:"outcome,omitempty"`
}

// DraftItemInput 是草案条目的工具入参。
type DraftItemInput struct {
	Title           string    `json:"title"`
	Note            string    `json:"note,omitempty"`
	StartAt         time.Time `json:"start_at"`
	EndAt           time.Time `json:"end_at"`
	ReminderMinutes int       `json:"reminder_minutes,omitempty"`
	TaskRef         string    `json:"task_ref,omitempty"`
}

// Service 是日程工具的服务端窄口（由 service 层 officeCalendarTool 适配器
// 满足：session→user 解析 + office.ScheduleUsecase/DraftUsecase 调用）。
// Stability:evolving
type Service interface {
	// ListMyEvents 返回会话属主与 [start,end) 相交的事件（start/end 升序）。
	ListMyEvents(ctx context.Context, sessionID string, start, end time.Time) ([]EventView, error)
	// CreateDraft 提交排期草案（形状校验在服务端；冲突为批准时点复检语义），
	// 返回草案 id（= 审批中心条目 id）。
	CreateDraft(ctx context.Context, sessionID string, items []DraftItemInput) (string, error)
}

// NewToolset 构造两动词工具集；svc 为 nil 返回 nil（未装配日程模块时不暴露）。
func NewToolset(svc Service) []trpctool.CallableTool {
	if svc == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&listTool{svc: svc},
		&draftTool{svc: svc},
	}
}

// ---------- calendar_list ----------

type listTool struct{ svc Service }

func (t *listTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "calendar_list",
		Description: "List the current user's calendar events overlapping a time window. " +
			"Args start/end are RFC3339; when omitted, defaults to now..+7d. Read-only.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "List calendar events in a window",
			Properties: map[string]*trpctool.Schema{
				"start": {Type: "string", Description: "Window start, RFC3339 (optional; default now)"},
				"end":   {Type: "string", Description: "Window end, RFC3339 (optional; default now+7d)"},
			},
		},
	}
}

func (t *listTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: invalid args: "+err.Error())
	}
	start, err := parseTimeOrDefault(in.Start, time.Now())
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: start: "+err.Error())
	}
	end, err := parseTimeOrDefault(in.End, start.Add(7*24*time.Hour))
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: "+err.Error())
	}
	if !end.After(start) {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: end must be after start")
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: session context unavailable")
	}
	events, err := t.svc.ListMyEvents(ctx, sessionID, start, end)
	if err != nil {
		return nil, err
	}
	return map[string]any{"events": events, "count": len(events)}, nil
}

func (t *listTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "calendar_list: not streamable")
}

// ---------- calendar_create_draft ----------

type draftTool struct{ svc Service }

func (t *draftTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "calendar_create_draft",
		Description: "Submit a schedule draft (one or more events) for human approval. " +
			"Nothing is written to the calendar until the draft is approved in the approval center. " +
			"Each item needs title/start_at/end_at (RFC3339); note/reminder_minutes/task_ref optional.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Submit schedule draft for approval",
			Properties: map[string]*trpctool.Schema{
				"items": {
					Type:        "array",
					Description: "Draft items",
					Items: &trpctool.Schema{
						Type: "object",
						Properties: map[string]*trpctool.Schema{
							"title":            {Type: "string", Description: "Event title"},
							"note":             {Type: "string", Description: "Optional note"},
							"start_at":         {Type: "string", Description: "Start, RFC3339"},
							"end_at":           {Type: "string", Description: "End, RFC3339"},
							"reminder_minutes": {Type: "integer", Description: "Minutes before start to remind (0 = none)"},
							"task_ref":         {Type: "string", Description: "Optional linked task id"},
						},
						Required: []string{"title", "start_at", "end_at"},
					},
				},
			},
			Required: []string{"items"},
		},
	}
}

func (t *draftTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		Items []struct {
			Title           string `json:"title"`
			Note            string `json:"note"`
			StartAt         string `json:"start_at"`
			EndAt           string `json:"end_at"`
			ReminderMinutes int    `json:"reminder_minutes"`
			TaskRef         string `json:"task_ref"`
		} `json:"items"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: invalid args: "+err.Error())
	}
	if len(in.Items) == 0 {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: items is required")
	}
	if len(in.Items) > 50 {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: too many items (max 50)")
	}
	items := make([]DraftItemInput, 0, len(in.Items))
	for i, raw := range in.Items {
		start, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.StartAt))
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: items["+itoa(i)+"].start_at: invalid RFC3339")
		}
		end, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.EndAt))
		if err != nil {
			return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: items["+itoa(i)+"].end_at: invalid RFC3339")
		}
		items = append(items, DraftItemInput{
			Title:           strings.TrimSpace(raw.Title),
			Note:            raw.Note,
			StartAt:         start,
			EndAt:           end,
			ReminderMinutes: raw.ReminderMinutes,
			TaskRef:         strings.TrimSpace(raw.TaskRef),
		})
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: session context unavailable")
	}
	draftID, err := t.svc.CreateDraft(ctx, sessionID, items)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":   "pending_approval",
		"draft_id": draftID,
		"detail":   "draft submitted to the approval center (kind=schedule_draft); events are created only after a human approves",
	}, nil
}

func (t *draftTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "calendar_create_draft: not streamable")
}

// ---------- helpers ----------

func parseTimeOrDefault(s string, def time.Time) (time.Time, error) {
	if strings.TrimSpace(s) == "" {
		return def, nil
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(s))
}

func itoa(i int) string { return strconv.Itoa(i) }
