// Package computeruse 提供桌面 GUI 自动化（Computer Use）的领域模型与用例编排。
// a11y 优先、视觉兜底的混合 grounding；安全策略（敏感词/禁区/预算/干跑/急停）在本层强制。
package computeruse

import "time"

// GroundingPath 标识一次动作的 grounding 来源路径。
type GroundingPath string

const (
	// PathA11y 无障碍树直接命中（毫秒级、元素级精确）。
	PathA11y GroundingPath = "a11y"
	// PathVision a11y 未命中，经 OmniParser/SoM/VLM 视觉兜底命中。
	PathVision GroundingPath = "vision"
	// PathVLMDirect 视觉组件全部不可用时 VLM 纯坐标直判（最低精度降级路径）。
	PathVLMDirect GroundingPath = "vlm_direct"
)

// ActionType 语义动作类型。
type ActionType string

const (
	ActionInvoke   ActionType = "invoke" // 元素级直调（无坐标，最精确）
	ActionClick    ActionType = "click"
	ActionTypeText ActionType = "type"
	ActionKey      ActionType = "key"
	ActionWheel    ActionType = "wheel"
	ActionDrag     ActionType = "drag"
	ActionLaunch   ActionType = "launch"
	ActionFocus    ActionType = "focus"
)

// SessionStatus 会话状态（状态机见 session_state_machine.go）。
type SessionStatus string

const (
	SessionIdle            SessionStatus = "idle"
	SessionObserving       SessionStatus = "observing"
	SessionGrounding       SessionStatus = "grounding"
	SessionActing          SessionStatus = "acting"
	SessionAwaitingConfirm SessionStatus = "awaiting_confirm"
	SessionDone            SessionStatus = "done"
	SessionFailed          SessionStatus = "failed"
	SessionCancelled       SessionStatus = "cancelled"
)

// Budget 会话预算：步数上限与截止时间。
type Budget struct {
	MaxSteps int       `json:"max_steps"` // 0 = 不限
	Deadline time.Time `json:"deadline"`  // 零值 = 不限
}

// Session 一次 GUI 自动化会话：预算与审计的聚合单位。
type Session struct {
	ID        string        `json:"id"`
	AgentKey  string        `json:"agent_key"`
	Status    SessionStatus `json:"status"`
	Budget    Budget        `json:"budget"`
	StepsUsed int           `json:"steps_used"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Point 物理像素坐标。
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Rect 物理像素包围盒。
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Center 返回包围盒中心点。
func (r Rect) Center() Point { return Point{X: r.X + r.W/2, Y: r.Y + r.H/2} }

// UIElement 统一元素模型（CDP §2.3）。bbox 一律物理像素。
type UIElement struct {
	Ref           string `json:"ref"`           // g{generation}.e{index}，仅同代有效
	Type          string `json:"type"`          // button|edit|menuitem|text|icon|...
	Name          string `json:"name"`          // 可访问名称
	BBox          Rect   `json:"bbox"`          // 物理像素
	Interactivity bool   `json:"interactivity"` // 是否可交互
	Source        string `json:"source"`        // uia|atspi|wda|vision
	AppName       string `json:"app_name"`
	Enabled       bool   `json:"enabled"`
	Generation    int    `json:"generation"` // 所属 snapshot 代
}

// SnapshotOpts 快照选项。
type SnapshotOpts struct {
	WindowTitle       string `json:"window_title,omitempty"`
	IncludeScreenshot bool   `json:"include_screenshot,omitempty"`
	MaxElements       int    `json:"max_elements,omitempty"`
}

// Snapshot 一次感知结果。
type Snapshot struct {
	Elements   []UIElement `json:"elements"`
	Generation int         `json:"generation"`
	// Screenshot 仅 IncludeScreenshot=true 时由 sidecar 内联返回（可空）。
	Screenshot *Image `json:"screenshot,omitempty"`
}

// Image 截图（PNG 字节流 + 元数据）。
type Image struct {
	PNG         []byte  `json:"-"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	ScaleFactor float64 `json:"scale_factor"`
}

// DeviceInfo 设备信息。
type DeviceInfo struct {
	Platform    string  `json:"platform"`
	ScreenW     int     `json:"screen_w"`
	ScreenH     int     `json:"screen_h"`
	ScaleFactor float64 `json:"scale_factor"`
}

// WindowInfo 窗口信息（禁区判定用）。
type WindowInfo struct {
	Hwnd         int64  `json:"hwnd"`
	Title        string `json:"title"`
	ProcessName  string `json:"process_name"`
	IsForeground bool   `json:"is_foreground"`
}

// StepResult 动作结果。
type StepResult string

const (
	StepOK        StepResult = "ok"
	StepRetry     StepResult = "retry"
	StepFailed    StepResult = "failed"
	StepCancelled StepResult = "cancelled"
	StepDryRun    StepResult = "dry_run"
)

// Step 一步动作记录（内存态 + 审计落库载荷）。
type Step struct {
	ID          int64          `json:"id"`
	SessionID   string         `json:"session_id"`
	AgentKey    string         `json:"agent_key"`
	Index       int            `json:"index"`
	Target      string         `json:"target"`
	Path        GroundingPath  `json:"path"`
	Action      ActionType     `json:"action"`
	Params      map[string]any `json:"params,omitempty"`
	Result      StepResult     `json:"result"`
	Error       string         `json:"error,omitempty"`
	DurationMs  int64          `json:"duration_ms"`
	ConfirmedBy string         `json:"confirmed_by,omitempty"`
	Danger      bool           `json:"danger"`
	CreatedAt   time.Time      `json:"created_at"`
}

// AuditEntry 审计落库记录（= Step 持久化形态）。
type AuditEntry = Step

// ObserveRequest 感知请求。
type ObserveRequest struct {
	AgentKey          string
	WindowTitle       string
	IncludeScreenshot bool
	MaxElements       int
}

// ObserveResult 感知结果（LLM 可读摘要 + 原始元素）。
type ObserveResult struct {
	Summary    string      `json:"summary"` // LLM 可读文本摘要
	Elements   []UIElement `json:"elements"`
	Generation int         `json:"generation"`
	Info       DeviceInfo  `json:"info"`
}

// ActRequest 动作请求。
type ActRequest struct {
	AgentKey    string
	SessionID   string // 可空：空时复用/自动创建该 Agent 活跃会话
	Target      string // 目标语义描述（如 "保存菜单项"）；坐标动作可空
	Action      ActionType
	Args        map[string]any // text/combo/x/y/button/delta/from/to 等
	DryRun      bool           // 干跑：只 grounding + 返回计划，不注入
	ConfirmedBy string         // 确认门通过后的确认人标识（审计用）
}

// ActResult 动作结果。
type ActResult struct {
	Step    Step        `json:"step"`
	Plan    *DryRunPlan `json:"plan,omitempty"` // 干跑时的执行计划
	Element *UIElement  `json:"element,omitempty"`
}

// DryRunPlan 干跑产出的执行计划。
type DryRunPlan struct {
	ResolvedRef  string        `json:"resolved_ref,omitempty"`
	ResolvedName string        `json:"resolved_name,omitempty"`
	Path         GroundingPath `json:"path"`
	WillDo       string        `json:"will_do"` // 人类可读的动作计划描述
}
