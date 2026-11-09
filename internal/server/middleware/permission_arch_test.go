package middleware_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	servermw "aranea-agents/internal/server/middleware"
)

// P1-6 敏感域清单（P0 详设 02 §2），与其 proto 目录名一致。
// P2 并入 ceoboard（整合方案 §5.1：经营驾驶舱域全量收严 view/ops 两档）。
var sensitiveDomains = []string{
	"admin",
	"ceoboard",
	"computeruse",
	"cron",
	"llm_provider_model",
	"monitor",
	"sandbox",
	"self_improvement",
	"system_setting",
	"tool",
	"usage",
}

// 显式豁免：这些 rpc 按设计"默认登录可见"，不要求登记权限码。
//
//	admin：Login/Logout/Current 属登录前或自助路径；
//	tool：仅 TestTool/List*Audit/Get*Params 收严，其余为工具自服务面。
var exemptRPC = map[string]bool{
	"/kratos.admin.v1.AdminService/Login":   true,
	"/kratos.admin.v1.AdminService/Logout":  true,
	"/kratos.admin.v1.AdminService/Current": true,
}

var (
	serviceRe = regexp.MustCompile(`^service\s+(\w+)\s*\{`)
	rpcRe     = regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(`)
	bindRe    = regexp.MustCompile(`\b(get|post|put|delete|patch)\s*:\s*"([^"]+)"`)
)

// rpcInfo 是 proto 中一个 rpc 的规范化信息。
type rpcInfo struct {
	FullMethod string   // /kratos.<域>.v1.<Svc>/<Method>
	HTTPKeys   []string // "GET /v1/x"（无绑定则为空）
}

// scanDomainRPCs 从 api/kratos/<domain>/v1/*.proto 抽取全部 rpc 及其 HTTP 绑定。
func scanDomainRPCs(t *testing.T, domain string) []rpcInfo {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "api", "kratos", domain, "v1")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read proto dir %s: %v", dir, err)
	}
	var out []rpcInfo
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".proto" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read proto %s: %v", e.Name(), err)
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		svc := ""
		for i := 0; i < len(lines); i++ {
			line := stripComment(lines[i])
			if m := serviceRe.FindStringSubmatch(line); m != nil {
				svc = m[1]
				continue
			}
			m := rpcRe.FindStringSubmatch(line)
			if m == nil || svc == "" {
				continue
			}
			// 收齐该 rpc 的块（花括号配平），以捕获 option 中的 http 绑定。
			depth := strings.Count(line, "{") - strings.Count(line, "}")
			block := line
			for j := i; depth > 0 && j+1 < len(lines); {
				j++
				block += "\n" + stripComment(lines[j])
				depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				i = j
			}
			info := rpcInfo{FullMethod: "/kratos." + domain + ".v1." + svc + "/" + m[1]}
			for _, b := range bindRe.FindAllStringSubmatch(block, -1) {
				info.HTTPKeys = append(info.HTTPKeys, servermw.HTTPKey(strings.ToUpper(b[1]), b[2]))
			}
			out = append(out, info)
		}
	}
	return out
}

func stripComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		return line[:i]
	}
	return line
}

// TestSensitiveDomainsFullyRegistered 是 P1-6 验收的架构 fitness function：
// 10 个敏感域的每个 rpc（除显式豁免）都必须在默认登记表中出现，
// 新增 rpc 若未登记会在本测试失败——防止权限面悄悄漏保。
func TestSensitiveDomainsFullyRegistered(t *testing.T) {
	reg := servermw.DefaultPermissionRegistry()
	if reg == nil {
		t.Fatal("DefaultPermissionRegistry() is nil")
	}
	total, covered := 0, 0
	var missing []string
	for _, domain := range sensitiveDomains {
		rpcs := scanDomainRPCs(t, domain)
		if len(rpcs) == 0 {
			t.Fatalf("sensitive domain %s yielded 0 rpc — proto scan broken", domain)
		}
		for _, info := range rpcs {
			total++
			if exemptRPC[info.FullMethod] {
				continue
			}
			if _, ok := reg.CodeFor(info.FullMethod); ok {
				covered++
				continue
			}
			missing = append(missing, info.FullMethod)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("sensitive-domain rpc missing permission registration (%d):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	t.Logf("sensitive-domain coverage: %d/%d non-exempt rpc registered", covered, total)
}

// TestExemptionsMatchProto 双向钉住 public 豁免集:架构测试的 exemptRPC(运行期
// 豁免判定依据)必须等于 proto 中显式标注 public 的 rpc 集(生成器产出
// generatedPublicRPCs)。防两类静默漂移:proto 标了 public 但测试未豁免
// (覆盖测试误报缺失);测试豁免但 proto 标了权限码(如误给 Login 加码,
// 登记了却被覆盖测试跳过,登录页被权限门锁死)。
func TestExemptionsMatchProto(t *testing.T) {
	gen := servermw.GeneratedPublicRPCs()
	if len(gen) != len(exemptRPC) {
		t.Fatalf("public 豁免集漂移: proto 标注 %d 条 vs exemptRPC %d 条\nproto: %v", len(gen), len(exemptRPC), gen)
	}
	for _, full := range gen {
		if !exemptRPC[full] {
			t.Fatalf("proto 标注 public 但 exemptRPC 未豁免: %s", full)
		}
	}
}

// TestRegisteredKeysExistInProto 反向校验：登记表的每个键（gRPC 全方法 + HTTP
// 方法/路径）都必须在对应 proto 中真实存在，防止 proto 改名/删除后残留死键。
func TestRegisteredKeysExistInProto(t *testing.T) {
	reg := servermw.DefaultPermissionRegistry()
	grpcSet := map[string]bool{}
	httpSet := map[string]bool{}
	for _, domain := range sensitiveDomains {
		for _, info := range scanDomainRPCs(t, domain) {
			grpcSet[info.FullMethod] = true
			for _, k := range info.HTTPKeys {
				httpSet[k] = true
			}
		}
	}
	var stale []string
	for op := range reg.CoverageSnapshot() {
		switch {
		case strings.HasPrefix(op, "/kratos."):
			if !grpcSet[op] {
				stale = append(stale, op)
			}
		case strings.Contains(op, " "):
			if !httpSet[op] {
				stale = append(stale, op)
			}
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("registry has keys not present in proto (%d):\n  %s",
			len(stale), strings.Join(stale, "\n  "))
	}
}
