package server

import (
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
)

// ws_v2_wire.go — v2 事件的线上传输类型（Wire types）。
//
// 背景（P2 #12 Wire/Domain 类型分离）：
// 此前 WSV2Subscriber 直接 json.Marshal 领域事件上线，领域 struct 无 json
// tag，依赖 Go 默认 PascalCase 序列化——隐式契约，重命名/增删导出字段会
// 静默改变线上协议。本文件定义显式 wire 类型并锁定字段顺序与 key 名，
// 使领域模型可独立演进；协议变更必须显式修改本文件并被 key 契约测试捕获。
// 领域→Wire 转换见 ws_v2_wire_convert.go。
//
// 字节兼容约束：wire 类型字段顺序、json tag、零值输出行为（无 omitempty）
// 必须与领域 struct 的默认序列化完全一致（golden 测试锁定）。
// 前端消费契约见 web/src/features/chat/v2Types.ts（PascalCase key）。

// === 实体 wire 类型（嵌套于事件 payload 内）===

type taskWire struct {
	ID          string         `json:"ID"`
	SessionID   string         `json:"SessionID"`
	UserMessage string         `json:"UserMessage"`
	Status      biz.TaskStatus `json:"Status"`
	Seq         int64          `json:"Seq"`
	Version     int64          `json:"Version"`
	WorkspaceID string         `json:"WorkspaceID"`
	CreatedAt   time.Time      `json:"CreatedAt"`
	UpdatedAt   time.Time      `json:"UpdatedAt"`
	CompletedAt *time.Time     `json:"CompletedAt"`
}

type turnWire struct {
	ID              string         `json:"ID"`
	TaskID          string         `json:"TaskID"`
	SessionID       string         `json:"SessionID"`
	SpiritSessionID string         `json:"SpiritSessionID"`
	ParentTurnID    string         `json:"ParentTurnID"`
	AgentKey        string         `json:"AgentKey"`
	TeamID          string         `json:"TeamID"`
	TeamStageID     string         `json:"TeamStageID"`
	Seq             int64          `json:"Seq"`
	Version         int64          `json:"Version"`
	Status          biz.TurnStatus `json:"Status"`
	StartedAt       time.Time      `json:"StartedAt"`
	CompletedAt     *time.Time     `json:"CompletedAt"`
}

type stepWire struct {
	ID              string          `json:"ID"`
	TurnID          string          `json:"TurnID"`
	TaskID          string          `json:"TaskID"`
	SessionID       string          `json:"SessionID"`
	SpiritSessionID string          `json:"SpiritSessionID"`
	Kind            biz.StepKind    `json:"Kind"`
	AuthorAgentKey  string          `json:"AuthorAgentKey"`
	Seq             int64           `json:"Seq"`
	Version         int64           `json:"Version"`
	Content         string          `json:"Content"`
	Reasoning       string          `json:"Reasoning"`
	ToolName        string          `json:"ToolName"`
	ToolCallID      string          `json:"ToolCallID"`
	ToolArgs        json.RawMessage `json:"ToolArgs"`
	ToolResult      json.RawMessage `json:"ToolResult"`
	ToolDurationMs  int64           `json:"ToolDurationMs"`
	ToolErrorCode   string          `json:"ToolErrorCode"`
	NoticeType      string          `json:"NoticeType"`
	Danger          bool            `json:"Danger"`
	Status          biz.StepStatus  `json:"Status"`
	IsFinal         bool            `json:"IsFinal"`
	StartedAt       time.Time       `json:"StartedAt"`
	CompletedAt     *time.Time      `json:"CompletedAt"`
}

type memberInfoWire struct {
	AgentKey       string `json:"AgentKey"`
	AgentName      string `json:"AgentName"`
	AvatarURL      string `json:"AvatarURL"`
	ChildSessionID string `json:"ChildSessionID"`
	Status         string `json:"Status"`
}

type teamStageWire struct {
	ID          string              `json:"ID"`
	TaskID      string              `json:"TaskID"`
	TurnID      string              `json:"TurnID"`
	SessionID   string              `json:"SessionID"`
	TeamID      string              `json:"TeamID"`
	TeamName    string              `json:"TeamName"`
	DagNodeID   string              `json:"DagNodeID"`
	DependsOn   []string            `json:"DependsOn"`
	Status      biz.TeamStageStatus `json:"Status"`
	Stage       biz.TeamStageStage  `json:"Stage"`
	Members     []memberInfoWire    `json:"Members"`
	Strategy    string              `json:"Strategy"`
	StartedAt   time.Time           `json:"StartedAt"`
	CompletedAt *time.Time          `json:"CompletedAt"`
	Seq         int64               `json:"Seq"`
	Version     int64               `json:"Version"`
}

type teamRunWire struct {
	ID              string              `json:"ID"`
	TeamStageID     string              `json:"TeamStageID"`
	TaskID          string              `json:"TaskID"`
	SessionID       string              `json:"SessionID"`
	SpiritSessionID string              `json:"SpiritSessionID"`
	DagNodeID       string              `json:"DagNodeID"`
	DependsOn       []string            `json:"DependsOn"`
	Status          biz.TeamRunV2Status `json:"Status"`
	StartedAt       time.Time           `json:"StartedAt"`
	CompletedAt     *time.Time          `json:"CompletedAt"`
	Seq             int64               `json:"Seq"`
	Version         int64               `json:"Version"`
	Error           string              `json:"Error"`
}

type memberSessionWire struct {
	ID              string                  `json:"ID"`
	TeamRunID       string                  `json:"TeamRunID"`
	TeamStageID     string                  `json:"TeamStageID"`
	TaskID          string                  `json:"TaskID"`
	SessionID       string                  `json:"SessionID"`
	SpiritSessionID string                  `json:"SpiritSessionID"`
	AgentKey        string                  `json:"AgentKey"`
	AgentName       string                  `json:"AgentName"`
	AvatarURL       string                  `json:"AvatarURL"`
	Status          biz.MemberSessionStatus `json:"Status"`
	Seq             int64                   `json:"Seq"`
	Version         int64                   `json:"Version"`
	StartedAt       time.Time               `json:"StartedAt"`
	FinishedAt      *time.Time              `json:"FinishedAt"`
	Error           string                  `json:"Error"`
}

type tokenUsageWire struct {
	PromptTokens     int64 `json:"PromptTokens"`
	CompletionTokens int64 `json:"CompletionTokens"`
	TotalTokens      int64 `json:"TotalTokens"`
}

type memberReportWire struct {
	AgentKey   string         `json:"AgentKey"`
	AgentName  string         `json:"AgentName"`
	Output     string         `json:"Output"`
	TokensUsed tokenUsageWire `json:"TokensUsed"`
	DurationMs int64          `json:"DurationMs"`
	Error      string         `json:"Error"`
}

type stepResultWire struct {
	Output        string             `json:"Output"`
	MemberReports []memberReportWire `json:"MemberReports"`
	TokensUsed    tokenUsageWire     `json:"TokensUsed"`
	DurationMs    int64              `json:"DurationMs"`
}

type stepErrorWire struct {
	Code         string            `json:"Code"`
	Message      string            `json:"Message"`
	Retryable    bool              `json:"Retryable"`
	FailedMember *memberReportWire `json:"FailedMember"`
}

type planStepWire struct {
	ID                string             `json:"ID"`
	PlanID            string             `json:"PlanID"`
	TaskID            string             `json:"TaskID"`
	Label             string             `json:"Label"`
	Description       string             `json:"Description"`
	DependsOn         []string           `json:"DependsOn"`
	MappedTeamStageID string             `json:"MappedTeamStageID"`
	Status            biz.PlanStepStatus `json:"Status"`
	AutoSynthesis     bool               `json:"AutoSynthesis"`
	StartedAt         time.Time          `json:"StartedAt"`
	CompletedAt       *time.Time         `json:"CompletedAt"`
	Seq               int64              `json:"Seq"`
	Version           int64              `json:"Version"`
	Result            *stepResultWire    `json:"Result"`
	Error             *stepErrorWire     `json:"Error"`
	AgentKeys         []string           `json:"AgentKeys"`
	// DeliverableContract 自带 snake_case json tag（本身即 wire 定义），直接复用。
	Deliverables  []biz.DeliverableContract `json:"Deliverables"`
	InputContract []biz.DeliverableContract `json:"InputContract"`
}

type planBoardWire struct {
	ID          string           `json:"ID"`
	TaskID      string           `json:"TaskID"`
	TurnID      string           `json:"TurnID"`
	SessionID   string           `json:"SessionID"`
	Strategy    biz.PlanStrategy `json:"Strategy"`
	Status      biz.PlanStatus   `json:"Status"`
	Steps       []planStepWire   `json:"Steps"`
	StartedAt   time.Time        `json:"StartedAt"`
	CompletedAt *time.Time       `json:"CompletedAt"`
	Seq         int64            `json:"Seq"`
	Version     int64            `json:"Version"`
}

type graphNodeWire struct {
	ID           string              `json:"ID"`
	GraphStageID string              `json:"GraphStageID"`
	Label        string              `json:"Label"`
	DagNodeID    string              `json:"DagNodeID"`
	TeamStageID  string              `json:"TeamStageID"`
	Status       biz.GraphNodeStatus `json:"Status"`
	DependsOn    []string            `json:"DependsOn"`
}

type graphStageWire struct {
	ID          string               `json:"ID"`
	TaskID      string               `json:"TaskID"`
	TurnID      string               `json:"TurnID"`
	SessionID   string               `json:"SessionID"`
	PlanBoardID string               `json:"PlanBoardID"`
	Nodes       []graphNodeWire      `json:"Nodes"`
	Status      biz.GraphStageStatus `json:"Status"`
	StartedAt   time.Time            `json:"StartedAt"`
	CompletedAt *time.Time           `json:"CompletedAt"`
	Seq         int64                `json:"Seq"`
	Version     int64                `json:"Version"`
}

// === 事件 payload wire 类型（envelope.payload 的形状）===

type taskEventWire struct {
	Task taskWire `json:"Task"`
}

type turnEventWire struct {
	TurnID string   `json:"TurnID"`
	Turn   turnWire `json:"Turn"`
}

type stepEventWire struct {
	Step stepWire `json:"Step"`
}

type stepStreamingEventWire struct {
	StepID     string `json:"StepID"`
	DeltaField string `json:"DeltaField"`
	DeltaChunk string `json:"DeltaChunk"`
	// DeltaSeq 会话级单调序号（Sequencer flush 时分配），前端用于增量去重。
	DeltaSeq int64 `json:"DeltaSeq"`
}

type teamStageEventWire struct {
	TeamStage teamStageWire `json:"TeamStage"`
}

type teamRunEventWire struct {
	TeamRun teamRunWire `json:"TeamRun"`
}

type memberSessionEventWire struct {
	MemberSession memberSessionWire `json:"MemberSession"`
}

type planBoardEventWire struct {
	PlanBoard planBoardWire `json:"PlanBoard"`
}

type planStepEventWire struct {
	PlanStep planStepWire `json:"PlanStep"`
}

type planStepSkippedEventWire struct {
	PlanStep planStepWire `json:"PlanStep"`
	Reason   string       `json:"Reason"`
}

type graphStageEventWire struct {
	GraphStage graphStageWire `json:"GraphStage"`
}

type graphNodeEventWire struct {
	GraphNode graphNodeWire `json:"GraphNode"`
}

type runStatusEventWire struct {
	RunID  string         `json:"RunID"`
	Status string         `json:"Status"`
	Meta   map[string]any `json:"Meta"`
}

type heartbeatEventWire struct {
	Message string         `json:"Message"`
	Meta    map[string]any `json:"Meta"`
}

type systemNoticeEventWire struct {
	NoticeType string         `json:"NoticeType"`
	Message    string         `json:"Message"`
	Meta       map[string]any `json:"Meta"`
	Seq        int64          `json:"Seq"`
}

// activityBridgeEventWire 包装 v1 ActivityEvent。biz.ActivityEvent 自带
// snake_case json tag（本身即 wire 定义），故直接复用而非另建镜像类型。
type activityBridgeEventWire struct {
	Event biz.ActivityEvent `json:"Event"`
}
