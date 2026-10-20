// probe.go M4 P4-2（76 设计「五工具接入扩展」§G，AC-12）：ProbeAgent 分级探测。
//
//	L1 not_installed         未安装（LookPath miss）
//	L2 spawn_failed          启动失败（spawn 失败，或进程在 initialize 应答前退出）
//	L3 handshake_failed      握手失败（initialize 超时无应答，或对端回 JSON-RPC 错误）
//	L4 version_incompatible  版本不兼容（protocolVersion ≠ 客户端支持版本）
//	L5 warming               预热中（全部通过但首轮 initialize 超过 warm 阈值，
//	                         npx 首跑下载属正常，agent 仍视为可派发）
//
// 级别经 last_probe_error 以 "<level>: <中文引导>" 形态落库（设计「probe_status
// 现有字段扩枚举字符串」的零 schema 变更读法——独立 probe_status 列入 backlog：
// ent regen 存在爆炸半径前科，且 proto/前端列变更分属其他会话泳道）。
package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProbeLevel 探测分级（语义见文件头 L1–L5）。
type ProbeLevel string

const (
	ProbeOK                  ProbeLevel = "ok"
	ProbeNotInstalled        ProbeLevel = "not_installed"
	ProbeSpawnFailed         ProbeLevel = "spawn_failed"
	ProbeHandshakeFailed     ProbeLevel = "handshake_failed"
	ProbeVersionIncompatible ProbeLevel = "version_incompatible"
	ProbeWarming             ProbeLevel = "warming"
)

// OKish 报告该级别下 agent 是否可派发（ok 或 warming 均可）。
func (l ProbeLevel) OKish() bool { return l == ProbeOK || l == ProbeWarming }

// DefaultProbeWarmTimeout 设计 §G 默认预热阈值：首轮 initialize 超过即 L5。
const DefaultProbeWarmTimeout = 60 * time.Second

// probeHandshakeBudget 握手总预算：预热阈值的四倍、下限 2 分钟。
// 预算必须显著大于 warm 阈值，否则 L5 永不可达（慢于阈值即先超时误判 L3）；
// 这即设计 §G「npx 首跑长超时档」。测试中可换短以加速断言。
var probeHandshakeBudget = func(warm time.Duration) time.Duration {
	b := warm * 4
	if b < 2*time.Minute {
		b = 2 * time.Minute
	}
	return b
}

// Probe 探测一次 agent 命令并分级返回人类可读引导（含 stderr 尾部等诊断材料，
// 无敏感信息——host env 已经 buildChildEnv 白名单过滤，detail 不引 env 内容）。
// level.OKish()==true 时 agent 可派发；detail 仅作展示，判级只用 level。
func Probe(ctx context.Context, opt SpawnOptions, warmTimeout time.Duration) (ProbeLevel, string) {
	if warmTimeout <= 0 {
		warmTimeout = DefaultProbeWarmTimeout
	}
	// L1
	if _, err := exec.LookPath(opt.Command); err != nil {
		return ProbeNotInstalled, fmt.Sprintf("未检测到 %s 命令，请先安装对应编程工具或检查 PATH", opt.Command)
	}
	budget := probeHandshakeBudget(warmTimeout)
	hsCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// L2（spawn 失败）
	proc, err := Spawn(hsCtx, opt)
	if err != nil {
		return ProbeSpawnFailed, fmt.Sprintf("启动失败：%v", err)
	}
	defer proc.Kill()

	// 探针仅握手，不建会话：入站消息丢弃（NewConn 容忍 nil handler）。
	conn := NewConn(proc.Stdout(), proc.Stdin(), nil, nil)
	defer conn.Close()

	started := time.Now()
	res, err := conn.Call(hsCtx, MethodInitialize, InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientCapabilities: ClientCapabilities{
			FS:       FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
			Terminal: false,
		},
	})
	if err != nil {
		if hsCtx.Err() != nil {
			// L3：预算耗尽仍无 initialize 应答（对端不是 ACP agent 或握手挂死）。
			return ProbeHandshakeFailed, fmt.Sprintf("ACP 握手超时（%s 内无 initialize 应答），请确认该命令是 ACP agent", budget.Round(time.Second))
		}
		// 连接已断但 Done 可能尚未广播（wait 循环瞬间即到）——给一拍以区分
		// 「进程已自杀」与「进程存活但协议层断开」。
		select {
		case <-proc.Done():
		case <-time.After(150 * time.Millisecond):
		}
		select {
		case <-proc.Done():
			// L2：进程在握手完成前自行退出（含缺依赖/参数错等首屏即死场景）。
			if tail := strings.TrimSpace(proc.StderrTail()); tail != "" {
				return ProbeSpawnFailed, fmt.Sprintf("启动失败（进程在 initialize 前退出）：%s", tail)
			}
			return ProbeSpawnFailed, fmt.Sprintf("启动失败（进程在 initialize 前退出）：%v", err)
		default:
		}
		// L3：进程存活但 initialize 回了 JSON-RPC 错误或连接异常断开。
		return ProbeHandshakeFailed, fmt.Sprintf("ACP 握手失败：%v", err)
	}
	var ir InitializeResult
	if uerr := json.Unmarshal(res, &ir); uerr != nil {
		return ProbeHandshakeFailed, fmt.Sprintf("ACP 握手失败：initialize 响应无法解析（%v）", uerr)
	}
	// L4
	if ir.ProtocolVersion != ProtocolVersion {
		return ProbeVersionIncompatible, fmt.Sprintf("ACP 版本不兼容：对端 protocolVersion=%d（仅支持 %d），请升级工具或回退版本", ir.ProtocolVersion, ProtocolVersion)
	}
	// L5
	if elapsed := time.Since(started); elapsed >= warmTimeout {
		return ProbeWarming, fmt.Sprintf("首次握手耗时 %s（冷启动下载依赖属正常，预热后可正常派发）", elapsed.Round(time.Second))
	}
	return ProbeOK, ""
}
