package computeruse

import "context"

// 窄接口拆分（DB-N3：≤5 方法）；DeviceGateway 为组合接口仅供实现端一次性实现。

// DevicePerceiver 设备感知（免确认路径）。
// Stability:evolving
type DevicePerceiver interface {
	Info(ctx context.Context) (DeviceInfo, error)
	Snapshot(ctx context.Context, opts SnapshotOpts) (Snapshot, error)
	Screenshot(ctx context.Context, region *Rect, zoom float64) (Image, error)
}

// DeviceActor 设备动作注入。
// Stability:evolving
type DeviceActor interface {
	Invoke(ctx context.Context, ref string, generation int) error
	Click(ctx context.Context, p Point, button string, clickCount int) error
	TypeText(ctx context.Context, text string) error
	Key(ctx context.Context, combo string) error
}

// DevicePointer 指针级动作（滚轮/拖拽）。Wait 在 Usecase 内实现，不进 sidecar。
// Stability:evolving
type DevicePointer interface {
	Wheel(ctx context.Context, p Point, delta int) error
	Drag(ctx context.Context, from, to Point, durationMs int) error
}

// DeviceController 窗口/应用控制。
// Stability:evolving
type DeviceController interface {
	FocusWindow(ctx context.Context, titleRegex string) error
	Launch(ctx context.Context, target string, args string, workDir string) (pid int, err error)
	ListWindows(ctx context.Context) ([]WindowInfo, error)
}

// DeviceGateway 组合接口：internal/computeruse 的 sidecar gateway 一次性实现。
// biz 层字段一律使用上面的窄接口类型声明。
// Stability:evolving
type DeviceGateway interface {
	DevicePerceiver
	DeviceActor
	DevicePointer
	DeviceController
}

// VisionParser 视觉解析（OmniParser HTTP 服务）。M1.3 实现。
// Stability:evolving
type VisionParser interface {
	Available(ctx context.Context) bool
	Parse(ctx context.Context, img Image) ([]UIElement, error)
}

// VisionGrounder VLM 语义定位。M1.3 实现。
// Stability:evolving
type VisionGrounder interface {
	// Pick 从候选元素中为 target 选出最匹配者，返回其 ref；无法判断返回 ErrGroundingFailed。
	Pick(ctx context.Context, img Image, candidates []UIElement, target string) (ref string, err error)
	// PickCoordinate VLM 坐标直判（vlm_direct 路径）：返回目标在 img 上的图像素点；
	// 无法判断返回 ErrGroundingFailed。
	PickCoordinate(ctx context.Context, img Image, target string) (Point, error)
}

// AuditStore 审计持久化（data 层 Ent 实现，M1.4）。
// Stability:evolving
type AuditStore interface {
	RecordStep(ctx context.Context, entry AuditEntry) error
	ListSteps(ctx context.Context, sessionID string) ([]AuditEntry, error)
}

// StepEventPublisher computeruse.step 实时事件发布端口（service 层装配 MonitorBus 适配器）。
// Stability:evolving
type StepEventPublisher interface {
	PublishStep(ctx context.Context, step Step)
}
