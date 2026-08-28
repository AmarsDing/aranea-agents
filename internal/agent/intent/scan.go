package intent

import (
	"regexp"
	"strings"
)

// scan.go：输入级确定性安全扫描（2026-08-28 方案② S1/S2，根修缺口 A/B）。
//
// 背景：此前 destructive 兜底（ForceDestructiveFlag）只能在 intent pass LLM
// parse 成功后运行——LLM 失败（skipped_llm/skipped_parse）时输入级安全扫描
// 完全不运行，而 pass 失败恰恰是降级继续执行的时刻（缺口 A）；且 L2 词表经
// 三轮 L3 加固（20261264/265/267）后已漂移，rm 族在 L2 仍是朴素子串，变形全
// 绕过（缺口 B）。
//
// 本文件把确定性扫描独立为 turn 入口可用的纯函数，不依赖 LLM 成败：
//   - pass 成功：结果合并进 artifact.RiskFlags（ForceDestructiveFlag 已改为消费本函数）
//   - pass 失败/跳过：service 层直接用本结果发 gate 事件 + 降级注入风险提示
//
// 定位【宽检测、窄拦截】：本层只标记+审计（gate 事件），硬拦截保持在 L3
// ParamRuleGate 参数级（那里有工具上下文，判断更准）。target 集合因此比 L3
// deny 宽（如 rm -rf /tmp/data 也打标）——标记的代价仅是审计记录与澄清门不
// 自动代答，不会阻断流程；误报纪律：shutdown/reboot/poweroff/halt 等可逆系统
// 管理操作【不】入本表（正常运维请求被标高风险会误挂澄清门，L3 已 deny 兜底）。
//
// 维护纪律：改 L3 deny 词表（tool_param_rules seed 迁移）时须同步评估本表；
// scan_test.go 以对账向量防再次漂移。

// inputRiskKeywords 高置信关键词（子串，小写比对）：沿用 BUG-MON-A 兜底词表，
// 补 drop database。"rm -rf" 已由下方 rm 族正则覆盖（更强），不再单列。
var inputRiskKeywords = []string{
	"fault_inject", "fault inject", "故障注入", "注入故障", "gns3_fault_inject",
	"模拟故障", "故障模拟",
	"drop table", "truncate table", "delete from ", "drop database",
	"删库", "删除数据库",
	"格式化磁盘", "format disk",
}

// inputRiskSoftKeywords 是词表未命中时的近误影子标记（只记录、不 flag）。
// S14 自然语言「把 BGP 邻居断了模拟故障」原先漏标；补「模拟故障」后硬扫描
// 已覆盖。影子表用于尚未入硬表的语义近邻，避免误报直接生效。
var inputRiskSoftKeywords = []string{
	"bgp", "邻居断", "断邻居", "port down", "link down",
}

// inputRiskSep 是命令/路径前缀分隔符类（对齐 L3 20261267 口径 + 自然语言空格）。
const inputRiskSep = `(^|[;&|/\s"'($` + "`" + `])`

// inputRiskPatterns 正则族（对齐 L3 tool_param_rules 20261267 加固标准）。
var inputRiskPatterns = []*regexp.Regexp{
	// rm 递归删除：任意 flags 排列（-fr/-r -f/-rfv/--recursive）、长选项、
	// sudo 包装、绝对路径（/bin/rm）、命令替换/子 shell/反引号前缀；
	// target 首字符限定路径特征（/ ~ . * $ 字母数字 _ -），CJK 起头的
	// target 不命中（防「如何防范 rm -rf 误删」类讨论误报）。
	regexp.MustCompile(inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?(?:/[\w.-]+)*/?rm(?:\s+(?:-{1,2}[\w=-]+|--))*\s+(?:-[a-z]*r[a-z]*|--recursive)(?:\s+(?:-{1,2}[\w=-]+|--))*\s+[/~.*$\w-]\S*`),
	// mkfs 格式化块设备（mkfs /dev/... 或 mkfs.ext4 ...）。
	regexp.MustCompile(`(?i)` + inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?(?:/[\w.-]+)*/?mkfs(?:\.[\w]+)?\s`),
	// dd 写块设备（of=/dev/ 为危险特征）。
	regexp.MustCompile(`(?i)` + inputRiskSep + `(?:sudo\s+(?:-\S+\s+)*)?dd\s+[^;&|]*\bof=/dev/`),
}

// ScanInputRisk 对原始用户输入做确定性风险扫描，返回命中的风险标记集合
// （当前仅 "destructive"，按集合设计便于后续扩展 ask 级标记）。纯函数、无
// 外部依赖、微秒级，可在 turn 入口对每条用户输入无条件运行。
func ScanInputRisk(userText string) []string {
	if strings.TrimSpace(userText) == "" {
		return nil
	}
	lower := strings.ToLower(userText)
	for _, kw := range inputRiskKeywords {
		if strings.Contains(lower, kw) {
			return []string{"destructive"}
		}
	}
	for _, re := range inputRiskPatterns {
		if re.MatchString(userText) {
			return []string{"destructive"}
		}
	}
	return nil
}

// ScanInputRiskShadowHits returns soft near-miss tokens when the hard scan
// missed. Callers log-only (shadow mode); do not treat as a flag.
func ScanInputRiskShadowHits(userText string) []string {
	if len(ScanInputRisk(userText)) > 0 {
		return nil
	}
	lower := strings.ToLower(userText)
	var hits []string
	for _, kw := range inputRiskSoftKeywords {
		if strings.Contains(lower, kw) {
			hits = append(hits, kw)
		}
	}
	return hits
}
