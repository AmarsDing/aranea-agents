package acp

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// SpawnOptions 描述一次子进程启动。
type SpawnOptions struct {
	Command string            // 可执行命令（经 exec.LookPath 解析）
	Args    []string          // 参数
	Env     map[string]string // 附加环境变量（叠加在进程环境之上）
	Dir     string            // 工作目录（空 = 继承）
}

// Process 是一个受管的 ACP agent 子进程。
// 生命周期：Spawn → Conn(stdin/stdout) → Kill/自然退出 → Done 关闭。
type Process struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	done    chan struct{} // 关闭即广播退出（所有等待者都能收到）
	exitErr error        // 退出结果（nil = 退出码 0），Done 关闭后可读
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
	cmd.Env = os.Environ()
	for k, v := range opt.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	setProcAttrs(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: stdout pipe: %w", err)
	}
	// stderr 丢弃：agent 诊断信息不进协议流（需要排查时由外层 wrap 获取）
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %q: %w", opt.Command, err)
	}

	p := &Process{cmd: cmd, stdin: stdin, stdout: stdout, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 0 {
			err = nil
		}
		p.exitErr = err
		close(p.done)
	}()
	return p, nil
}

// Stdin 返回协议写入端（NDJSON 帧）。
func (p *Process) Stdin() io.Writer { return p.stdin }

// Stdout 返回协议读取端（NDJSON 帧）。
func (p *Process) Stdout() io.Reader { return p.stdout }

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
