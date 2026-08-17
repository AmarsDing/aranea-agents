# 19-cli CLI 测试结果（2026-08-17）

> 二进制 `bin/aranea.exe`（测前按当前源码重新构建，go build ./cmd/aranea）。
> 结果：**8 PASS / 1 FAIL**（CLI-08 为真实缺陷：非法命令静默退出）。

## 结果汇总

| ID | 用例 | 结果 | 数据 |
|----|------|------|------|
| CLI-01 | version | PASS | dev（ldflags 未注入版本，构建信息待发布流程注入） |
| CLI-02 | --help 顶级命令 | PASS | 15 个命令组 |
| CLI-03 | agent ls -o json | PASS | 20 条/页，exit 0，113ms |
| CLI-04 | session ls -o json | PASS | 20 条/页，109ms |
| CLI-05 | tool ls -o json | PASS | 20 条/页，179ms |
| CLI-06 | monitor events -o json | PASS | 7KB 事件数据，274ms |
| CLI-07 | session send -y（真实 LLM 链路） | PASS | Spirit 会话 6c0ec1f0 发送成功，端到端 8.9s（含 LLM 推理） |
| CLI-08 | 非法命令错误提示 | **FAIL** | exit=3 但**零输出** |
| CLI-09 | system info -o json | PASS | 275B，115ms |

## 缺陷与解决方案

### BUG-CLI-01（真实缺陷·建议 P3 修复）：非法命令静默退出
- **现象**：`aranea nosuchcmd` 退出码 3，stdout/stderr 均无任何输出，用户得不到「命令不存在」提示。
- **原因**：[main.go](file:///f:/myproject/aranea-agents/cmd/aranea/main.go#L78-L79) root 命令设置 `SilenceUsage: true, SilenceErrors: true` 抑制 cobra 打印；同时 `main()` 拿到 err 后仅 `os.Exit(cli.ExitCodeOf(err))` 从不打印错误文本，两相叠加导致静默。
- **解决方案**：main() 退出前补一行 stderr 打印：
  ```go
  if err := execute(ctx, bi); err != nil {
      fmt.Fprintln(os.Stderr, "aranea:", err.Error())
      os.Exit(cli.ExitCodeOf(err))
  }
  ```
  注意先排查现有错误路径是否已有打印（避免重复输出）；该修复同时改善所有命令失败的可见性。

## 踩坑记录（测试过程问题）

1. **bin/aranea.exe 是陈旧构建**（2026-08-13，源码 2026-08-15 已变）：旧二进制不识别 cobra 子命令，任何参数都落进 REPL（banner + 本地会话 ID）。已重新构建（28574208B）。**教训：bin/ 产物不进版本控制则容易过期，测试前先重建。**
2. **测试脚本踩 `$args` 自动变量坑**（2026-08-12 已立规的同一坑）：Cli-Run 形参命名 `$Args` 导致实参静默丢失，所有调用变裸 `aranea` 进 REPL。已改 `$CmdArgs` 重跑。
3. REPL 空启动不产生服务端会话（stdin EOF 即退，会话 ID 仅本地打印），已核实 sessions/sessions_v2/trpc_session_states 三表无残留。

## 清理

- 无 DB 残留；证据文件留存 evidence/（cli01~cli09）。
- CLI-07 向 Spirit 会话 6c0ec1f0 发送了一条真实消息（"Reply with exactly one word: OK"），属该会话正常历史。
