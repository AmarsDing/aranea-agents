package repl

import (
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

// SlashResult describes what should happen after a slash command.
type SlashResult struct {
	// Quit signals the REPL should exit.
	Quit bool
	// Handled means the command was processed internally (don't send to WS).
	Handled bool
}

// handleSlash processes /command inputs and returns a SlashResult.
// output is where help/info is printed.
func handleSlash(cmd string, r *REPL, output io.Writer) SlashResult {
	parts := strings.Fields(strings.TrimPrefix(cmd, "/"))
	if len(parts) == 0 {
		return SlashResult{Handled: true}
	}
	switch strings.ToLower(parts[0]) {
	case "quit", "exit", "q":
		return SlashResult{Quit: true, Handled: true}

	case "help", "h", "?":
		fmt.Fprintln(output, slashHelp)
		return SlashResult{Handled: true}

	case "session":
		if len(parts) == 1 {
			fmt.Fprintf(output, "当前会话: %s\n", r.sessionID)
			return SlashResult{Handled: true}
		}
		switch strings.ToLower(parts[1]) {
		case "new":
			r.sessionID = uuid.NewString()
			fmt.Fprintf(output, "已切换到新会话: %s（当前连接仍会在下次启动时生效；当前后端暂未暴露 REPL 内重连 API）\n", r.sessionID)
		case "list", "resume":
			fmt.Fprintf(output, "/session %s 暂不可用：CLI REPL 尚未接入会话列表/重连 API，请使用 aranea session 命令。\n", parts[1])
		default:
			fmt.Fprintf(output, "未知 /session 子命令：%s\n", parts[1])
		}
		return SlashResult{Handled: true}

	case "agent":
		if len(parts) > 1 {
			r.agentKey = parts[1]
			r.sessionID = uuid.NewString()
			fmt.Fprintf(output, "切换到 Agent: %s；已准备新会话 %s。当前连接不会重连，请退出后用 aranea chat --agent %s 继续。\n", r.agentKey, r.sessionID, r.agentKey)
		} else {
			fmt.Fprintf(output, "当前 Agent: %s\n", r.agentKey)
		}
		return SlashResult{Handled: true}

	case "dry-run":
		if len(parts) > 1 {
			switch strings.ToLower(parts[1]) {
			case "on", "true", "1":
				r.dryRun = true
			case "off", "false", "0":
				r.dryRun = false
			default:
				fmt.Fprintf(output, "用法：/dry-run [on|off]\n")
				return SlashResult{Handled: true}
			}
		} else if r.dryRun {
			r.dryRun = false
		} else {
			r.dryRun = true
		}
		if r.dryRun {
			fmt.Fprintln(output, "dry-run 模式：开启（消息不实际发送）")
		} else {
			fmt.Fprintln(output, "dry-run 模式：关闭")
		}
		return SlashResult{Handled: true}

	case "yes":
		fmt.Fprintln(output, "/yes 暂不可用：当前 WS 协议尚未提供会话级确认开关，请在工具确认提示中显式回复。")
		return SlashResult{Handled: true}

	case "tools":
		fmt.Fprintln(output, "/tools 暂不可用：后端尚未提供当前 REPL 会话工具列表 API。")
		return SlashResult{Handled: true}

	case "expand":
		fmt.Fprintln(output, "/expand 暂不可用：CLI 尚未保存上一条工具结果的可展开副本。")
		return SlashResult{Handled: true}

	case "copy":
		fmt.Fprintln(output, "/copy 暂不可用：当前 CLI 未集成跨平台剪贴板。")
		return SlashResult{Handled: true}

	case "cancel":
		if r.conn != nil {
			_ = r.conn.Send(r.ctx, buildEnvelope("cancel", nil))
		}
		fmt.Fprintln(output, "已发送 cancel 请求")
		return SlashResult{Handled: true}

	case "clear":
		fmt.Fprint(output, "\033[H\033[2J")
		return SlashResult{Handled: true}

	default:
		fmt.Fprintf(output, "未知命令：/%s  输入 /help 查看帮助\n", parts[0])
		return SlashResult{Handled: true}
	}
}

const slashHelp = `可用命令：
  /help          显示此帮助
  /session       显示当前会话 ID
  /session new   准备新会话（当前连接暂不自动重连）
  /session list|resume  暂请使用 aranea session 命令
  /agent [key]   显示或切换当前 Agent（提示新会话）
  /dry-run [on|off] 切换 dry-run 模式
  /yes           暂不可用：后端未提供会话级确认开关
  /tools         暂不可用：后端未提供当前会话工具列表 API
  /expand        暂不可用：尚未缓存上一条工具结果
  /copy          暂不可用：尚未集成剪贴板
  /cancel        取消当前正在运行的任务
  /clear         清屏
  /quit          退出 REPL`
