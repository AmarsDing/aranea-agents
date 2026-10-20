package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"aranea-agents/pkg/safego"
)

// SpawnOptions 描述一次子进程启动。
type SpawnOptions struct {
	Command string            // 可执行命令（经 exec.LookPath 解析）
	Args    []string          // 参数
	Env     map[string]string // 显式注入环境变量（API key 等经此注入；宿主环境只传白名单）
	Dir     string            // 工作目录（空 = 继承）
}

// spawnEnvWhitelist S2（M4 P0-2）：子进程环境最小继承白名单——只带系统/网络/
// 区域必需键与显式注入键，宿主密钥形态键（含 *_KEY/*_TOKEN/*_SECRET 等）不进子进程。
var spawnEnvWhitelist = []string{
	// Windows 系统必需（大小写形态经 EqualFold 合并去重）
	"PATH", "SYSTEMROOT", "SYSTEMDRIVE", "WINDIR", "COMSPEC", "PATHEXT", "OS",
	"PROCESSOR_ARCHITECTURE", "PROCESSOR_ARCHITEW6432", "NUMBER_OF_PROCESSORS",
	// 用户目录与配置基准
	"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "LOCALAPPDATA",
	"PUBLIC", "PROGRAMDATA", "TEMP", "TMP",
	// POSIX / 国际化 / 工具链默认路径
	"TERM", "LANG", "LC_ALL", "LC_CTYPE", "TZ", "SHELL",
	"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
	// 网络代理（CLI 访问远端 API 常需）
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
}

// buildChildEnv 组装子进程环境：宿主环境过白名单，再叠加显式注入键（后者覆盖同名键）。
func buildChildEnv(extra map[string]string) []string {
	seen := make(map[string]struct{}, len(spawnEnvWhitelist))
	env := make([]string, 0, len(spawnEnvWhitelist)+len(extra))
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		for _, w := range spawnEnvWhitelist {
			if strings.EqualFold(key, w) {
				up := strings.ToUpper(key)
				if _, dup := seen[up]; !dup {
					seen[up] = struct{}{}
					env = append(env, kv)
				}
				break
			}
		}
	}
	for k, v := range extra {
		kv := k + "=" + v
		up := strings.ToUpper(k)
		if _, dup := seen[up]; !dup {
			seen[up] = struct{}{}
		}
		env = append(env, kv)
	}
	return env
}

// stderrRing 有限尾部缓冲（S3）：stderr 只保留最近 cap 字节，供失败诊断取尾部摘要。
const stderrRingCap = 32 << 10 // 32 KiB

type stderrRing struct {
	mu  sync.Mutex
	buf []byte
}

func (r *stderrRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := len(p)
	if total >= stderrRingCap {
		r.buf = append(r.buf[:0], p[total-stderrRingCap:]...)
		return total, nil
	}
	if over := len(r.buf) + total - stderrRingCap; over > 0 {
		r.buf = append(r.buf[over:], p...)
	} else {
		r.buf = append(r.buf, p...)
	}
	return total, nil
}

func (r *stderrRing) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(append([]byte(nil), r.buf...))
}

// Process 是一个受管的 ACP agent 子进程。
// 生命周期：Spawn → Conn(stdin/stdout) → Kill/自然退出 → Done 关闭。
type Process struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderrRd io.ReadCloser
	stderr   *stderrRing
	done     chan struct{} // 关闭即广播退出（所有等待者都能收到）
	exitErr  error         // 退出结果（nil = 退出码 0），Done 关闭后可读
}

// Spawn 启动子进程。命令不存在时返回带命令名的错误。
// Windows 下子进程归属新进程组，Kill 时整组终止（覆盖 npx node 孙进程）。
func Spawn(ctx context.Context, opt SpawnOptions) (*Process, error) {
	if _, err := exec.LookPath(opt.Command); err != nil {
		return nil, fmt.Errorf("acp: command %q not found: %w", opt.Command, err)
	}
	cmd := exec.CommandContext(ctx, opt.Command, opt.Args...)
	if opt.Dir != "" {
		cmd.Dir = opt.Dir
	}
	cmd.Env = buildChildEnv(opt.Env)
	setProcAttrs(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	// S3：stderr 接尾部环形缓冲（诊断材料）；绝不进 stdout 协议流。
	stderrRd, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %q: %w", opt.Command, err)
	}

	p := &Process{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		stderrRd: stderrRd,
		stderr:   &stderrRing{},
		done:     make(chan struct{}),
	}
	// stderr 泵：进程退出后 pipe EOF 自动结束
	safego.GoBackground("acp.process.stderr", func() {
		_, _ = io.Copy(p.stderr, stderrRd)
	})
	// 75 review Y3：等待循环经 safego 托管，panic 可恢复并上报 hook
	safego.GoBackground("acp.process.wait", func() {
		err := cmd.Wait()
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 0 {
			err = nil
		}
		p.exitErr = err
		close(p.done)
	})
	return p, nil
}

// Stdin 返回协议写入端（NDJSON 帧）。
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout 返回协议读取端（NDJSON 帧）。
func (p *Process) Stdout() io.Reader { return p.stdout }

// StderrTail 返回 stderr 尾部（≤32KiB；S3：进程失败诊断材料）。
func (p *Process) StderrTail() string {
	if p.stderr == nil {
		return ""
	}
	return p.stderr.String()
}

// Done 返回进程退出信号通道（关闭即退出，可多方监听）。
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitErr 阻塞至进程退出并返回结果：nil = 退出码 0。
func (p *Process) ExitErr() error {
	<-p.done
	return p.exitErr
}

// PID 返回子进程 PID（启动恢复清理用）。
func (p *Process) PID() int { return p.cmd.Process.Pid }

// Kill 终止整个进程组并等待退出。幂等。
func (p *Process) Kill() {
	if p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done: // 已退出
		return
	default:
	}
	killProcessTree(p.cmd)
	<-p.done
}

// ProbeCommand 探测命令是否可用（派发前置检查）。
func ProbeCommand(command string) error {
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("command %q not found: %w", command, err)
	}
	return nil
}
