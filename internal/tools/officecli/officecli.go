// Package officecli 实现 Office 文档操作工具集（officecli_read / officecli_write /
// officecli_render）。底层驱动 OfficeCLI 单二进制
//（https://github.com/iOfficeAI/OfficeCLI，无需安装 Office），业务层实现，
// 不动 vendored trpc 框架（FW-R1）。
//
// 安全模型：
//   - 动词白名单：read 仅 view/get/query/validate/dump/help；write 仅
//     create/add/set/remove/save/close/open；render 固定 view <file> <mode> -o <out>。
//     watch/install/mcp 等系统级命令一律不放行。
//   - 路径围栏：文件参数仅接受工作区相对路径，拒绝绝对路径/卷名/.. 越界/
//     符号链接逃逸；子进程 cwd 固定为工作区根。
//   - 写工具经种子 reqConfirm=true 接入 HITL 确认门禁。
//   - 子进程强制 OFFICECLI_RESIDENT_FLUSH=each：officecli 默认驻留内存、空闲
//     2-10s 才落盘；Agent 每次调用是独立进程，驻留进程可能在其被杀前未来得及
//     落盘（实测跨会话编辑丢失）。flush=each 让每条命令返回前落盘，牺牲少量 IO
//     换跨调用持久化（2026-08-15 端到端实测发现）。
//
// 配置经环境变量注入：
//
//	ARANEA_OFFICECLI_BIN          officecli 二进制路径，默认 PATH 查找 "officecli"
//	ARANEA_OFFICECLI_TIMEOUT_SEC  单次调用超时秒数，默认 120
package officecli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	artifactbiz "aranea-agents/internal/biz/artifact"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// 工具名（= 平台工具种子 tool_key，确认门禁按此精确匹配）。
const (
	ToolRead   = "officecli_read"
	ToolWrite  = "officecli_write"
	ToolRender = "officecli_render"
)

// 单次调用 stdout/stderr 各保留的最大字节数（超出截断并标注）。
const maxOutputBytes = 256 * 1024

// 动词白名单。help 无文件参数，其余动词文件参数固定在 args[1]。
var readVerbs = map[string]bool{
	"view": true, "get": true, "query": true,
	"validate": true, "dump": true, "help": true,
}

var writeVerbs = map[string]bool{
	"create": true, "add": true, "set": true, "remove": true,
	"save": true, "close": true, "open": true,
}

// renderModes 渲染模式 → 输出扩展名 + MIME。
var renderModes = map[string]struct {
	ext  string
	mime string
}{
	"html":       {"html", "text/html"},
	"screenshot": {"png", "image/png"},
	"svg":        {"svg", "image/svg+xml"},
	"pdf":        {"pdf", "application/pdf"},
}

// Config 是 officecli 二进制的调用配置。
type Config struct {
	Bin     string
	Timeout time.Duration
}

// ConfigFromEnv 从环境变量加载配置（缺省：PATH 查找 officecli，超时 120s）。
func ConfigFromEnv() Config {
	cfg := Config{
		Bin:     strings.TrimSpace(os.Getenv("ARANEA_OFFICECLI_BIN")),
		Timeout: 120 * time.Second,
	}
	if v := strings.TrimSpace(os.Getenv("ARANEA_OFFICECLI_TIMEOUT_SEC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Timeout = time.Duration(n) * time.Second
		}
	}
	return cfg
}

// execResult 统一输出容器。框架 NewFunctionTool 对输出类型反射生成 schema，
// 直接用 any 会在 reflect.TypeOf(nil) 处 panic（FW-R1 不改框架，业务侧具体类型规避）。
type execResult struct {
	Result any `json:"result" jsonschema:"description=执行结果对象：ok=是否成功，exit_code=进程退出码，stdout/stderr=进程输出（可能被截断），argv=实际执行参数，error=失败原因（ok=false 时）"`
}

// ---------- 输入结构 ----------

type commandInput struct {
	Args []string `json:"args" jsonschema:"description=officecli 命令参数数组（不含二进制名）。首元素为动词，第二元素为工作区相对文件路径（help 除外）。示例：[\"create\",\"deck.pptx\"]、[\"add\",\"deck.pptx\",\"/\",\"--type\",\"slide\",\"--prop\",\"title=Q4 Report\"]、[\"set\",\"doc.docx\",\"/body/p[1]\",\"--prop\",\"bold=true\"]、[\"get\",\"data.xlsx\",\"/Sheet1/A1\",\"--json\"],required"`
}

type renderInput struct {
	File      string   `json:"file" jsonschema:"description=要渲染的 Office 文件（工作区相对路径），如 deck.pptx,required"`
	Mode      string   `json:"mode" jsonschema:"description=渲染模式：html（静态 HTML 快照）| screenshot（PNG 截图，供多模态视觉校验）| svg（PPT 单页矢量）| pdf（PDF 导出）,required"`
	ExtraArgs []string `json:"extra_args,omitempty" jsonschema:"description=可选附加标志，如 [\"--grid\",\"3\"]、[\"--page\",\"2\"]；禁止 -o/--output（输出名由工具生成）与 --browser"`
}

// ---------- 工具构造函数 ----------

func newReadTool(cfg Config, dir string) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in commandInput) (execResult, error) {
		argv, err := buildReadArgv(in.Args)
		if err != nil {
			return execResult{}, err
		}
		return cfg.exec(ctx, dir, argv), nil
	},
		trpcfunction.WithName(ToolRead),
		trpcfunction.WithDescription("读取/检查 Office 文档（docx/xlsx/pptx），底层为 officecli CLI。可用动词：view（outline|stats|issues|text|annotated 模式查看文档）、get（按路径取节点，如 /slide[1]、/body/p[3]、/Sheet1/A1，--depth N 展开子节点）、query（CSS 选择器查询）、validate（OpenXML 规范校验）、dump（原始 XML）、help（查询元素可用属性，不确定属性名时先 help 不要猜）。文件为工作区相对路径；建议加 --json 获取结构化输出。禁止输出重定向（-o），需要渲染文件请用 officecli_render。"),
	)
}

func newWriteTool(cfg Config, dir string) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in commandInput) (execResult, error) {
		argv, err := buildWriteArgv(in.Args)
		if err != nil {
			return execResult{}, err
		}
		return cfg.exec(ctx, dir, argv), nil
	},
		trpcfunction.WithName(ToolWrite),
		trpcfunction.WithDescription("创建/编辑 Office 文档（docx/xlsx/pptx），底层为 officecli CLI。可用动词：create（按扩展名创建空白文档）、add（添加元素：add deck.pptx / --type slide --prop title=\"标题\"）、set（改属性：set doc.docx /body/p[1] --prop bold=true；查找替换：set f.docx / --find 草稿 --replace 终稿）、remove（删除元素）、save/close/open（驻留会话管理，多步编辑建议先 open 后 close）。元素用路径寻址（/slide[1]/shape[2]、/Sheet1/B2）；--prop 可重复。工作区内多步编辑自动享受 resident 模式免重复 IO。编辑前先 officecli_read 查看结构，不确定属性名先 officecli_read help。文件为工作区相对路径。"),
	)
}

func newRenderTool(cfg Config, dir string, artifacts artifactbiz.Saver) trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(func(ctx context.Context, in renderInput) (execResult, error) {
		argv, outName, mime, err := buildRenderArgv(in)
		if err != nil {
			return execResult{}, err
		}
		res := cfg.exec(ctx, dir, argv)
		m, _ := res.Result.(map[string]any)
		if m == nil {
			return res, nil
		}
		if ok, _ := m["ok"].(bool); !ok {
			return res, nil
		}
		return persistRenderOutput(ctx, m, dir, outName, mime, artifacts), nil
	},
		trpcfunction.WithName(ToolRender),
		trpcfunction.WithDescription("把 Office 文档渲染为可视化产物：html（静态 HTML 快照）/ screenshot（PNG 截图，排版视觉校验首选）/ svg（PPT 单页矢量）/ pdf。输出文件自动命名并保存在工作区，同时落盘为会话制品（返回 artifact:// URL 供用户下载）。生成/大改文档后应渲染截图检查排版，再迭代修正。"),
	)
}

// NewToolset 返回全部 3 个 officecli 工具。
func NewToolset(cfg Config, dir string, artifacts artifactbiz.Saver) []trpctool.Tool {
	return []trpctool.Tool{
		newReadTool(cfg, dir),
		newWriteTool(cfg, dir),
		newRenderTool(cfg, dir, artifacts),
	}
}

// EnabledTools 仅返回 effective key 集合中启用的工具（白名单最小授权，
// 与 twinops.EnabledTools 同构）。dir 为空（工作区未解析）时不挂载任何工具——
// 无围栏根目录宁可 fail-closed。
func EnabledTools(eff map[string]bool, cfg Config, dir string, artifacts artifactbiz.Saver) []trpctool.Tool {
	if len(eff) == 0 || strings.TrimSpace(dir) == "" {
		return nil
	}
	var out []trpctool.Tool
	for _, t := range NewToolset(cfg, dir, artifacts) {
		if d := t.Declaration(); d != nil && eff[d.Name] {
			out = append(out, t)
		}
	}
	return out
}

// AnyEnabled 报告 effective key 集合是否启用了任一 officecli 工具，
// 供装配层决定是否解析工作区目录。
func AnyEnabled(eff map[string]bool) bool {
	return eff[ToolRead] || eff[ToolWrite] || eff[ToolRender]
}

// ---------- 参数校验与路径围栏 ----------

// buildReadArgv 校验只读调用：动词白名单 + 文件围栏 + 禁止输出重定向/浏览器。
func buildReadArgv(args []string) ([]string, error) {
	return buildArgv(args, readVerbs, true)
}

func buildWriteArgv(args []string) ([]string, error) {
	return buildArgv(args, writeVerbs, false)
}

// buildArgv 统一校验 read/write 参数。readOnly=true 时额外拒绝 -o/--output
// 与 --browser（输出落盘归 render 工具）。
func buildArgv(args []string, verbs map[string]bool, readOnly bool) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("args 不能为空，首元素为动词（如 view/get/create/set）")
	}
	verb := strings.ToLower(strings.TrimSpace(args[0]))
	if !verbs[verb] {
		names := make([]string, 0, len(verbs))
		for v := range verbs {
			names = append(names, v)
		}
		return nil, fmt.Errorf("动词 %q 不在本工具白名单（允许: %s）。创建/编辑请用 officecli_write，渲染文件请用 officecli_render", verb, strings.Join(names, "/"))
	}
	for _, a := range args[1:] {
		f := strings.ToLower(strings.TrimSpace(a))
		if strings.HasPrefix(f, "--browser") {
			return nil, fmt.Errorf("服务端环境禁止 --browser 标志")
		}
		if readOnly && (f == "-o" || strings.HasPrefix(f, "-o=") || strings.HasPrefix(f, "--output")) {
			return nil, fmt.Errorf("本工具禁止输出重定向（-o/--output）；生成渲染文件请用 officecli_render")
		}
	}
	if verb == "help" {
		return append([]string(nil), args...), nil
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("缺少文件参数（工作区相对路径，如 deck.pptx）")
	}
	if strings.HasPrefix(strings.TrimSpace(args[1]), "-") {
		return nil, fmt.Errorf("文件参数 %q 形如命令行标志，疑似参数错位", args[1])
	}
	if _, err := jailPath(args[1]); err != nil {
		return nil, err
	}
	return append([]string(nil), args...), nil
}

// buildRenderArgv 组装 render 命令：view <file> <mode> -o <生成名> [extra...]。
// 输出名由工具生成（杜绝 LLM 控制 -o 越界），不含目录分量，天然落在
// 子进程 cwd（工作区根）。返回 argv、工作区相对输出名与输出 MIME。
func buildRenderArgv(in renderInput) ([]string, string, string, error) {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	info, ok := renderModes[mode]
	if !ok {
		return nil, "", "", fmt.Errorf("mode 仅支持 html/screenshot/svg/pdf，收到 %q", in.Mode)
	}
	if _, err := jailPath(in.File); err != nil {
		return nil, "", "", err
	}
	for _, a := range in.ExtraArgs {
		f := strings.ToLower(strings.TrimSpace(a))
		if f == "-o" || strings.HasPrefix(f, "-o=") || strings.HasPrefix(f, "--output") {
			return nil, "", "", fmt.Errorf("extra_args 禁止 -o/--output（输出名由工具自动生成）")
		}
		if strings.HasPrefix(f, "--browser") {
			return nil, "", "", fmt.Errorf("服务端环境禁止 --browser 标志")
		}
	}
	base := strings.TrimSuffix(filepath.Base(filepath.Clean(strings.TrimSpace(in.File))), filepath.Ext(strings.TrimSpace(in.File)))
	if base == "" || base == "." {
		base = "render"
	}
	outName := fmt.Sprintf("%s-%s.%s", base, time.Now().UTC().Format("20060102T150405Z"), info.ext)
	argv := []string{"view", filepath.Clean(strings.TrimSpace(in.File)), mode, "-o", outName}
	argv = append(argv, in.ExtraArgs...)
	return argv, outName, info.mime, nil
}

// jailPath 校验 p 为工作区内相对路径：拒绝绝对路径、卷名相对路径（C:foo）、
// .. 逃逸。清理后的相对路径原样返回（子进程 cwd=工作区根，相对路径即围栏内）。
func jailPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	if filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", fmt.Errorf("仅允许工作区相对路径，收到 %q", p)
	}
	cleaned := filepath.Clean(p)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("文件路径越界（.. 逃逸工作区）: %q", p)
	}
	return cleaned, nil
}

// ---------- 进程执行 ----------

// execCommand 是进程创建 seam（单测替换为 helper 进程，不改框架）。
var execCommand = exec.CommandContext

// exec 在工作区目录下执行 officecli 并返回结构化结果。进程级失败（不可达/
// 非零退出/超时）一律返回 ok=false 结构化对象（不返回 Go error），让 LLM 拿到
// stderr 作为诊断证据自行修正，而不是盲目重试（与 twinops 失败处理约定一致）。
func (c Config) exec(ctx context.Context, dir string, args []string) execResult {
	bin := c.Bin
	if bin == "" {
		bin = "officecli"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return errResult(args, "officecli 未安装或未加入 PATH（可设 ARANEA_OFFICECLI_BIN 指定二进制路径；安装见 https://officecli.ai/SKILL.md）: "+err.Error())
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := execCommand(cctx, path, args...)
	cmd.Dir = dir
	// 强制每条命令返回前落盘：默认驻留+空闲自动落盘模式在 Agent 多进程调用
	// 模型下可能丢编辑（驻留进程被杀前未 flush）。用户已设则尊重用户值。
	if os.Getenv("OFFICECLI_RESIDENT_FLUSH") == "" {
		cmd.Env = append(os.Environ(), "OFFICECLI_RESIDENT_FLUSH=each")
	}
	var stdout, stderr cappedBuffer
	stdout.max = maxOutputBytes
	stderr.max = maxOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	m := map[string]any{
		"ok":        runErr == nil && exitCode == 0,
		"exit_code": exitCode,
		"argv":      args,
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
	}
	if stdout.Truncated() || stderr.Truncated() {
		m["truncated"] = true
	}
	if cctx.Err() == context.DeadlineExceeded {
		m["ok"] = false
		m["error"] = fmt.Sprintf("执行超时（%ds），进程已终止；大文档操作建议先 open 驻留再分步执行", int(timeout.Seconds()))
	} else if runErr != nil {
		m["ok"] = false
		m["error"] = runErr.Error()
	}
	return execResult{Result: m}
}

func errResult(args []string, msg string) execResult {
	return execResult{Result: map[string]any{
		"ok":        false,
		"exit_code": -1,
		"argv":      args,
		"error":     msg,
	}}
}

// persistRenderOutput 渲染成功后：取产物字节 → 落盘会话制品（best-effort）→
// 补充制品字段。制品失败/不可用不翻转 ok，仅保留工作区文件路径。
// 产物来源：html/screenshot/pdf 由 officecli 写到 -o 文件；svg 模式 officecli
// 忽略 -o、只写 stdout（2026-08-15 实测），故文件缺失时回退取 stdout。
func persistRenderOutput(ctx context.Context, m map[string]any, dir, outName, mime string, artifacts artifactbiz.Saver) execResult {
	outPath := filepath.Join(dir, outName)
	data, err := os.ReadFile(outPath)
	if err != nil {
		if stdout, _ := m["stdout"].(string); strings.TrimSpace(stdout) != "" {
			data = []byte(stdout)
			m["source"] = "stdout" // svg：officecli 忽略 -o，产物在 stdout
		} else {
			m["ok"] = false
			m["error"] = "渲染命令成功但输出不可读（文件与 stdout 均为空）: " + err.Error()
			return execResult{Result: m}
		}
	}
	m["file"] = outName
	m["size_bytes"] = len(data)
	if artifacts == nil {
		m["note"] = "制品服务不可用，渲染文件保留在工作区"
		return execResult{Result: m}
	}
	sessionID := sessionIDFromCtx(ctx)
	if sessionID == "" {
		m["note"] = "上下文无会话 ID，渲染文件保留在工作区"
		return execResult{Result: m}
	}
	saved, err := artifacts.Save(ctx, sessionID, outName, mime, data)
	if err != nil {
		m["note"] = "制品落盘失败（文件保留在工作区）: " + err.Error()
		return execResult{Result: m}
	}
	m["artifact_id"] = saved.ID
	m["artifact_url"] = "artifact://" + saved.ID
	return execResult{Result: m}
}

// sessionIDFromCtx 从 Agent 调用上下文解析会话 ID（与 provider/media 同款）。
func sessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	return inv.Session.ID
}

// cappedBuffer 是有界输出缓冲：超过 max 后丢弃后续写入并记录截断。
type cappedBuffer struct {
	buf   bytes.Buffer
	max   int
	total int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	remain := b.max - b.buf.Len()
	if remain > 0 {
		if len(p) > remain {
			b.buf.Write(p[:remain])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	if b.Truncated() {
		return b.buf.String() + fmt.Sprintf("\n...[输出截断，共 %d 字节，仅保留前 %d 字节；可加 --json 或缩小范围重试]", b.total, b.max)
	}
	return b.buf.String()
}

func (b *cappedBuffer) Truncated() bool { return b.total > b.max }
