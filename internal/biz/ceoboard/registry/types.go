// Package registry 实现工作流注册表:声明加载、schema 校验、[待补] 显式跳过。
// 字段定义对齐方案 v0.2 §14.1;校验规则对齐 §14.3 三道闸之 schema 校验。
package registry

import (
	"time"

	"gopkg.in/yaml.v3"
)

// PendingMarker 是声明中"待补"的机器可识别标记;含该标记的元素加载期显式跳过。
const PendingMarker = "[待补"

// Duration 支持 YAML "30s" 形式的时长。
type Duration time.Duration

// UnmarshalYAML 解析时长标量。
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// D 返回标准库时长。
func (d Duration) D() time.Duration { return time.Duration(d) }

// Workflow 是注册单元(一条业务工作流的完整声明)。
type Workflow struct {
	APIVersion  string   `yaml:"api_version"`
	Kind        string   `yaml:"kind"`
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	BizLine     string   `yaml:"biz_line"`
	Owner       string   `yaml:"owner"`
	Version     int      `yaml:"version"`
	Permissions []string `yaml:"permissions"`
	Changelog   []Change `yaml:"changelog"`
	Probe       *Probe   `yaml:"probe"`
	Nodes       []Node   `yaml:"nodes"`

	File string `yaml:"-"` // 来源文件(加载期填充)
}

// Change 是版本演进记录(§14.3 要求节点增删必须升 version 并说明)。
type Change struct {
	Version int    `yaml:"version"`
	Date    string `yaml:"date"`
	Note    string `yaml:"note"`
}

// Probe 是健康探针声明;stale_after 即陈旧阈值。
type Probe struct {
	Type       string   `yaml:"type"` // rest | nats | ws
	Path       string   `yaml:"path"`
	Interval   Duration `yaml:"interval"`
	StaleAfter Duration `yaml:"stale_after"`
}

// Node 是流程节点;enter/exit 为"事件或状态条件"表达式。
type Node struct {
	ID        string   `yaml:"id"`
	Name      string   `yaml:"name"`
	Order     int      `yaml:"order"`
	Enter     string   `yaml:"enter"`
	Exit      string   `yaml:"exit"`
	BranchOut []Branch `yaml:"branch_out"`
	Source    *Source  `yaml:"source"`
	Metrics   []Metric `yaml:"metrics"`
	Drill     []Drill  `yaml:"drill"`
}

// Branch 是条件分支(R3-Q2:必须支持分支与 terminal 节点)。
type Branch struct {
	When string `yaml:"when"`
	To   string `yaml:"to"`
}

// Source 是数据源绑定;Skipped/SkipReason 由加载器填充(含 [待补] 标记时)。
type Source struct {
	Adapter  string `yaml:"adapter"`
	Endpoint string `yaml:"endpoint"`
	Stream   string `yaml:"stream"`
	// StreamOptional:true 表示该流在源平台是"条件创建"(如 HCF MM_PUB 仅 MAP worker 启用时建流,
	// 见 HotCraftFlow smoke.sh:52-63);探测缺失时标灰"未启用"而非标红"声明漂移"。
	StreamOptional bool              `yaml:"stream_optional"`
	Subjects       []string          `yaml:"subjects"`
	RestPath       string            `yaml:"rest_path"`
	SSEPath        string            `yaml:"sse_path"`
	DedupeKey      string            `yaml:"dedupe_key"`
	FieldMap       map[string]string `yaml:"field_map"`

	Skipped    bool   `yaml:"-"`
	SkipReason string `yaml:"-"`
}

// Metric 是指标声明;算子仅 count/ratio/latency/gauge 四种(§14.1)。
type Metric struct {
	ID         string            `yaml:"id"`
	Unit       string            `yaml:"unit"`
	Window     string            `yaml:"window"`
	Expr       string            `yaml:"expr"`
	Thresholds map[string]string `yaml:"thresholds"`
	Precompute string            `yaml:"precompute"`
	// Join 是 latency(A,B) 类指标的配对键(payload 点路径,如 payload.signal_id);缺省回退节点 field_map.entity_id。
	Join    string `yaml:"join"`
	Note    string `yaml:"note"`
	Pending string `yaml:"pending"`
}

// Drill 是下钻定义;fields 白名单控制展示与权限裁剪。
type Drill struct {
	Label      string            `yaml:"label"`
	Type       string            `yaml:"type"` // rest | sse | console_url
	Path       string            `yaml:"path"`
	Params     map[string]string `yaml:"params"`
	Fields     []string          `yaml:"fields"`
	EmptyState string            `yaml:"empty_state"`
	Note       string            `yaml:"note"`
	Permission string            `yaml:"permission"`
}

// SkipEntry 记录一条被显式跳过的元素([待补])。
type SkipEntry struct {
	Workflow string `json:"workflow"`
	Node     string `json:"node,omitempty"`
	Metric   string `json:"metric,omitempty"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
}

// FailEntry 记录一条加载失败的工作流(隔离失败,不影响其余)。
type FailEntry struct {
	File   string   `json:"file"`
	Errors []string `json:"errors"`
}

// Report 是注册表加载报告(校验行为对外可见)。
type Report struct {
	Dir      string      `json:"dir"`
	LoadedAt time.Time   `json:"loaded_at"`
	Loaded   []string    `json:"loaded"`
	Failed   []FailEntry `json:"failed"`
	Skipped  []SkipEntry `json:"skipped"`
}
