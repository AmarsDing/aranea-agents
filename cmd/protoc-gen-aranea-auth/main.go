// protoc-gen-aranea-auth 从 rpc 上的 (aranea.auth.v1.requirement) 自定义选项
// 生成权限登记表 permission_registry_gen.go（P1 任务分解 §4 收口:proto option
// codegen 替换 permission_codes.go 手工登记表）。
//
// 语义与约束:
//   - 仅处理 domains 参数列出的敏感域(proto 包 kratos.<域>.v1);域内每个 rpc
//     必须携带 requirement 选项——缺失即生成失败(覆盖门禁前移到生成期,
//     架构测试 permission_arch_test.go 仍作运行期复核,双保险);
//   - 键空间与判定语义不变:gRPC=FullMethod,HTTP=`<大写方法> <pathTemplate>`,
//     由 permission_codes.go 的 requireRPC/requireAuthenticatedRPC 装配;
//   - public 豁免的 rpc 不登记,只生成注释留痕 + generatedPublicRPCs 清单
//     (架构测试断言其与豁免集一致,防"测试豁免但 proto 加码"或反向漂移);
//   - additional_bindings 暂不支持(当前 11 域无用法),出现即报错而非静默丢键;
//   - 输出为单一聚合文件,按 域/service/rpc 排序,确定性输出(diff 友好)。
//
// 用法(见 Makefile api 目标):
//
//	protoc --plugin=protoc-gen-aranea-auth=<bin> \
//	  --aranea-auth_out=domains=admin,ceoboard,...:./internal/server/middleware \
//	  <全部 api protos>
package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	authv1 "aranea-agents/api/aranea/auth/v1"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// registration 是一个 rpc 的规范化登记项。
type registration struct {
	domain string
	svc    string
	method string
	httpM  string // HTTP 方法(大写;无绑定为空)
	httpP  string // pathTemplate(无绑定为空)
	code   string // permission 码;authenticated 为空
	public bool   // 显式豁免
}

// defaultSensitiveDomains 是默认敏感域清单(P0 详设 02 §2 的 11 域 +
// 评审 2026-11-09 H4 补录 agentbridge——该域在 P1-6 普查范围外,管理/派发
// 是主机命令执行面;E 阶段(2026-11-09 需求文档)补录 approval/credential/
// github/office/review/secops 六域——凭据/审批/安全运营均为收严面;
// 与 permission_arch_test.go sensitiveDomains 同源两份:
// 插件漏配→架构测试红;架构测试漏配→生成期不报但测试也不查——改动域清单
// 时两处同步)。2026-11-18 补录 fleet(Harness 多任务运行台:起终端即主机
// 命令执行面)。2026-11-19 补录 Harness › 连接四域:a2a(含联邦)、channel、gateway(出站推送)、
// model_catalog——此前全部默认「登录即可」,任何登录用户能增删改通道 / 推送 / 远程 Agent。
// protoc 参数以逗号分隔键值对,故 domains 覆盖参数用分号分隔,默认清单免参。
var defaultSensitiveDomains = []string{
	"a2a", "admin", "agentbridge", "approval", "ceoboard", "channel", "computeruse", "credential",
	"cron", "fleet", "gateway", "github", "llm_provider_model", "model_catalog", "monitor", "office", "review",
	"sandbox", "secops", "self_improvement", "system_setting", "tool", "usage",
}

func main() {
	var flags flag.FlagSet
	domainsOpt := flags.String("domains", "", "分号分隔的敏感域覆盖清单(默认用内置清单;显式给出时,清单内域必须全部出现在本次文件集中)")
	outName := flags.String("out", "permission_registry_gen.go", "输出文件名(相对 --aranea-auth_out 目录)")
	protogen.Options{ParamFunc: flags.Set}.Run(func(gen *protogen.Plugin) error {
		gen.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		domainSet := map[string]bool{}
		enforcePresence := false
		if *domainsOpt != "" {
			enforcePresence = true
			for _, d := range strings.Split(*domainsOpt, ";") {
				if d = strings.TrimSpace(d); d != "" {
					domainSet[d] = true
				}
			}
		} else {
			for _, d := range defaultSensitiveDomains {
				domainSet[d] = true
			}
		}
		if len(domainSet) == 0 {
			return fmt.Errorf("domains 参数为空:敏感域清单缺省会静默跳过全部文件,必须显式给出")
		}

		var regs []registration
		seenDomains := map[string]bool{}
		var problems []string
		var anchor *protogen.File
		for _, f := range gen.Files {
			if !f.Generate {
				continue
			}
			parts := strings.Split(string(f.Desc.Package()), ".")
			if len(parts) != 3 || parts[0] != "kratos" || parts[2] != "v1" {
				continue // 非 kratos.<域>.v1 包:非敏感域文件,跳过
			}
			domain := parts[1]
			if !domainSet[domain] {
				continue
			}
			seenDomains[domain] = true
			if anchor == nil {
				anchor = f
			}
			for _, svc := range f.Services {
				for _, m := range svc.Methods {
					reg, err := extractRegistration(domain, string(svc.Desc.Name()), m)
					if err != nil {
						problems = append(problems, fmt.Sprintf("%s: %v", f.Desc.Path(), err))
						continue
					}
					regs = append(regs, reg)
				}
			}
		}
		if enforcePresence {
			for d := range domainSet {
				if !seenDomains[d] {
					problems = append(problems, fmt.Sprintf("敏感域 %q 未在本次 protoc 调用的文件集中出现(domains 拼写错误或文件漏传)", d))
				}
			}
		}
		if len(problems) > 0 {
			sort.Strings(problems)
			return fmt.Errorf("requirement 选项校验失败:\n  %s", strings.Join(problems, "\n  "))
		}
		if anchor == nil {
			return fmt.Errorf("未匹配到任何敏感域文件(domains=%q)", *domainsOpt)
		}

		sort.Slice(regs, func(i, j int) bool {
			a, b := regs[i], regs[j]
			if a.domain != b.domain {
				return a.domain < b.domain
			}
			if a.svc != b.svc {
				return a.svc < b.svc
			}
			return a.method < b.method
		})

		g := gen.NewGeneratedFile(*outName, anchor.GoImportPath)
		emitRegistry(g, regs)
		return nil
	})
}

// extractRegistration 读取一个 rpc 的 requirement 选项与 HTTP 绑定。
// 缺选项、oneof 未置位、additional_bindings 均视为错误(调用方聚合后整体失败)。
func extractRegistration(domain, svcName string, m *protogen.Method) (registration, error) {
	full := "/" + string(m.Desc.ParentFile().Package()) + "." + svcName + "/" + string(m.Desc.Name())
	reg := registration{domain: domain, svc: svcName, method: string(m.Desc.Name())}

	opts, _ := m.Desc.Options().(*descriptorpb.MethodOptions)
	if opts == nil || !proto.HasExtension(opts, authv1.E_Requirement) {
		return reg, fmt.Errorf("rpc %s 缺少 (aranea.auth.v1.requirement) 选项——敏感域每个 rpc 必须显式声明 permission/authenticated/public", full)
	}
	req := proto.GetExtension(opts, authv1.E_Requirement).(*authv1.AuthRequirement)
	switch kind := req.Kind.(type) {
	case *authv1.AuthRequirement_Permission:
		if strings.TrimSpace(kind.Permission) == "" {
			return reg, fmt.Errorf("rpc %s 的 permission 为空串", full)
		}
		reg.code = kind.Permission
	case *authv1.AuthRequirement_Authenticated:
		// code 留空
	case *authv1.AuthRequirement_Public:
		reg.public = true
	default:
		return reg, fmt.Errorf("rpc %s 的 requirement oneof 未置位", full)
	}

	if proto.HasExtension(opts, annotations.E_Http) {
		rule := proto.GetExtension(opts, annotations.E_Http).(*annotations.HttpRule)
		switch p := rule.Pattern.(type) {
		case *annotations.HttpRule_Get:
			reg.httpM, reg.httpP = "GET", p.Get
		case *annotations.HttpRule_Post:
			reg.httpM, reg.httpP = "POST", p.Post
		case *annotations.HttpRule_Put:
			reg.httpM, reg.httpP = "PUT", p.Put
		case *annotations.HttpRule_Delete:
			reg.httpM, reg.httpP = "DELETE", p.Delete
		case *annotations.HttpRule_Patch:
			reg.httpM, reg.httpP = "PATCH", p.Patch
		}
		if len(rule.AdditionalBindings) > 0 {
			return reg, fmt.Errorf("rpc %s 使用 additional_bindings——插件暂不支持(需扩展为每绑定一条登记),请勿静默丢键", full)
		}
	}
	return reg, nil
}

// emitRegistry 输出聚合登记文件。内容只调用 permission_codes.go 的包内装配
// 辅助函数,自身不含判定逻辑。
func emitRegistry(g *protogen.GeneratedFile, regs []registration) {
	const (
		header = `// Code generated by protoc-gen-aranea-auth from (aranea.auth.v1.requirement) method options. DO NOT EDIT.
//
// 来源:api/kratos/<域>/v1/*.proto 的 rpc 选项;重新生成:make api(见 Makefile
// 与 docs/aranea-agents/guide/代码生成工具链.md)。登记语义、码位分配规则与
// 键空间见 permission_codes.go;运行期判定见 permission.go;覆盖校验见
// permission_arch_test.go。

package middleware

// registerGeneratedPermissions 由生成器装配:敏感域(P0 详设 02 §2)每个 rpc 的
// 认证/授权要求。按 域/service/rpc 字典序排列(确定性输出)。
func registerGeneratedPermissions(r *PermissionRegistry) {
`
	)
	g.P(header)

	var publics []string
	coded, authed := 0, 0
	lastDomain := ""
	for _, reg := range regs {
		if reg.domain != lastDomain {
			if lastDomain != "" {
				g.P()
			}
			g.P("\t// ── ", reg.domain, " ──")
			lastDomain = reg.domain
		}
		full := "/kratos." + reg.domain + ".v1." + reg.svc + "/" + reg.method
		switch {
		case reg.public:
			publics = append(publics, full)
			g.P("\t// public 豁免(不登记,留痕;架构测试断言与 exemptRPC 一致): ", full)
		case reg.code == "":
			authed++
			g.P("\trequireAuthenticatedRPC(r, ", quote(reg.domain), ", ", quote(reg.svc), ", ", quote(reg.method), ", ", quote(reg.httpM), ", ", quote(reg.httpP), ")")
		default:
			coded++
			g.P("\trequireRPC(r, ", quote(reg.domain), ", ", quote(reg.svc), ", ", quote(reg.method), ", ", quote(reg.httpM), ", ", quote(reg.httpP), ", ", quote(reg.code), ")")
		}
	}
	g.P("}")
	g.P()

	sort.Strings(publics)
	g.P("// generatedPublicRPCs 是 proto 中显式标注 public 豁免的 rpc 全方法名清单。")
	g.P("// permission_arch_test.go 断言其与 exemptRPC 集合一致——两侧任一改动的漂移即测试红。")
	g.P("var generatedPublicRPCs = []string{")
	for _, p := range publics {
		g.P("\t", quote(p), ",")
	}
	g.P("}")
	g.P()
	g.P("// 统计:permission=", coded, " authenticated=", authed, " public=", len(publics))
}

func quote(s string) string { return fmt.Sprintf("%q", s) }
