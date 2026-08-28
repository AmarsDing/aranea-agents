package intent

import (
	"strings"
	"testing"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// TestScanInputRisk_L3BypassVectors 对账 L3 tool_param_rules 20261265/267 已防的
// rm 族绕过变形：输入级扫描必须全部命中（防双表漂移的护栏——L3 再加固新变形时，
// 同步把向量加入本表）。
func TestScanInputRisk_L3BypassVectors(t *testing.T) {
	vectors := []string{
		"rm -rf /",
		"rm -fr /",                    // flags 顺序变形
		"rm -r -f /",                  // 拆分 flags
		"rm -f -r /",                  // 拆分 flags 逆序
		"rm --recursive --force /",    // 长选项
		"rm -rf --no-preserve-root /", // GNU 真实删根形态
		"rm -rfv /",                   // 组合 flags
		"rm -r --force /",             // 长短混排
		"$(rm -rf /)",                 // 命令替换
		"`rm -rf /`",                  // 反引号
		"(rm -rf /)",                  // 子 shell
		"sudo rm -rf /",               // sudo 包装
		"sudo -S rm -rf /",            // sudo 带 flag
		"/bin/rm -rf /",               // 绝对路径
		"sh -c \"rm -rf /\"",          // shell 包装（引号前缀）
		"rm -rf ~",                    // 家目录
		"rm -rf $HOME",                // 环境变量
		"rm -rf *",                    // 通配
		"echo done; rm -rf /",         // 分号串联
	}
	for _, v := range vectors {
		if got := ScanInputRisk(v); len(got) == 0 {
			t.Errorf("ScanInputRisk(%q) = %v, want destructive hit", v, got)
		}
	}
}

// TestScanInputRisk_Keywords 保留关键词回归（沿用 BUG-MON-A 兜底词表语义）。
func TestScanInputRisk_Keywords(t *testing.T) {
	hits := []string{
		"请对 sw1 执行 fault_inject",
		"调用 gns3_fault_inject 注入端口 down",
		"注入故障到交换机 eth1",
		"对 sw1 注入故障",
		"drop table users",
		"truncate table sessions",
		"delete from audit_log",
		"直接 drop database prod",
		"他要删库",
		"删除数据库再重建",
		"格式化磁盘 /dev/sda",
		"format disk D:",
		"把核心交换机 core-sw1 的 BGP 邻居断了模拟故障。",
		"对核心交换机做 BGP 故障注入",
		// 新增正则族：mkfs / dd
		"mkfs /dev/sda",
		"mkfs.ext4 /dev/sda1",
		"sudo mkfs /dev/sdb",
		"dd if=/dev/zero of=/dev/sda",
		"sudo dd of=/dev/sda bs=1M",
	}
	for _, v := range hits {
		if got := ScanInputRisk(v); len(got) == 0 {
			t.Errorf("ScanInputRisk(%q) = %v, want destructive hit", v, got)
		}
	}
}

// TestScanInputRisk_NoFalsePositive 误报纪律：讨论性/正常运维输入不得命中。
func TestScanInputRisk_NoFalsePositive(t *testing.T) {
	misses := []string{
		"如何防范 rm -rf 误删",             // CJK target：讨论性输入
		"rm -rf 命令的原理是什么",            // 无路径 target
		"帮我写一个 landing page",         // 正常请求
		"delete the unused variable", // 英文普通 delete
		"重启服务器",                      // 可逆系统管理操作（不入表，L3 兜底）
		"reboot the server",          // 同上
		"shutdown 和 poweroff 的区别",    // 讨论性
		"rm 命令怎么用",                   // 无递归标志无 target
		"帮我清理一下磁盘空间",                 // 正常运维
		"",
	}
	for _, v := range misses {
		if got := ScanInputRisk(v); len(got) != 0 {
			t.Errorf("ScanInputRisk(%q) = %v, want no hit", v, got)
		}
	}
}

// TestScanInputRisk_WideDetection 宽检测语义：target 为普通路径（L3 不 deny 的
// /tmp 场景）输入级仍打标——分层差异：宽检测（审计/澄清门）→ 窄拦截（L3）。
func TestScanInputRisk_WideDetection(t *testing.T) {
	hits := []string{
		"rm -rf /tmp/data",    // L3 deny 不命中（target 非根），输入级打标
		"rm -rf node_modules", // 裸词 target（非 CJK 起头）
		"rm -rf ./dist",
	}
	for _, v := range hits {
		if got := ScanInputRisk(v); len(got) == 0 {
			t.Errorf("ScanInputRisk(%q) = %v, want destructive hit (wide detection)", v, got)
		}
	}
}

// TestRunOptionInjectInputRisk 降级注入（S3-2）：flags 非空注入一条含警示头的
// system 消息；flags 为空为 no-op。
func TestRunOptionInjectInputRisk(t *testing.T) {
	var opts trpcagent.RunOptions
	RunOptionInjectInputRisk([]string{"destructive"})(&opts)
	if len(opts.InjectedContextMessages) != 1 {
		t.Fatalf("InjectedContextMessages len = %d, want 1", len(opts.InjectedContextMessages))
	}
	content := opts.InjectedContextMessages[0].Content
	if !strings.Contains(content, inputRiskNoticeHeader) || !strings.Contains(content, "destructive") {
		t.Errorf("injected content missing notice header/flags: %q", content)
	}

	var empty trpcagent.RunOptions
	RunOptionInjectInputRisk(nil)(&empty)
	if len(empty.InjectedContextMessages) != 0 {
		t.Errorf("empty flags should be no-op, got %d messages", len(empty.InjectedContextMessages))
	}
}

func TestScanInputRisk_ShadowNearMiss(t *testing.T) {
	soft := "把 core-sw1 的 BGP 邻居断了做演练"
	if got := ScanInputRisk(soft); len(got) != 0 {
		t.Fatalf("hard scan must miss %q, got %v", soft, got)
	}
	shadow := ScanInputRiskShadowHits(soft)
	if len(shadow) == 0 {
		t.Fatalf("shadow scan must hit BGP/邻居断 near-miss, got %v", shadow)
	}
	if hits := ScanInputRiskShadowHits("对 sw1 执行故障注入"); len(hits) != 0 {
		t.Fatalf("hard-hit must not also shadow, got %v", hits)
	}
}
