// Package service — twin_openapi_webhook.go
//
// Webhook 回调 SSRF 防护（P0 底座安全前置，2026-11-02，三仓联动）。
// 原状：handleCreateRun 接受任意 webhook_url，postEvent 直接发起 POST——
// 持有机器 token 的调用方可令底座向内网任意地址投递 HMAC 签名负载
// （a2a 包有 SSRF 防护但 twin 路径未接线）。
//
// 双闸设计：
//  1. 提交闸（submit-time）：webhook_url 按白名单策略校验，不合规 400；
//  2. 拨号闸（dial-time）：投递所用 http.Client 的 DialContext 拦截
//     出口 IP——非白名单公网/链路本地/元数据地址一律拒连，封堵
//     DNS 重绑定 TOCTOU（提交时解析的 IP 与投递时不一致）。
//
// 策略档位（ARANEA_TWINOPENAPI_WEBHOOK_ALLOW，逗号分隔）：
//   - 未设置：默认「内网可用」档——提交闸放行，拨号闸仅允许私有/环回
//     地址（存量 twinmonitor 同机回调不受影响；公网与云元数据被拒）；
//   - 条目列表：严格白名单档——提交闸要求 host 精确命中（host 或
//     host:port）或 IP 命中 CIDR 条目；拨号闸同默认档；
//   - 含 "*"：显式放开（仅供特殊运维场景，启动记 warn）。
package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TwinOpenAPIWebhookAllowEnv 回调白名单环境变量名。
const TwinOpenAPIWebhookAllowEnv = "ARANEA_TWINOPENAPI_WEBHOOK_ALLOW"

// webhookAllowPolicy 回调地址白名单策略。
type webhookAllowPolicy struct {
	// strict 为 true 时提交闸执行 hosts/cidrs 白名单。
	strict bool
	// allowAll 为 true 时两闸全放（显式 "*" 条目）。
	allowAll bool
	hosts    map[string]bool
	cidrs    []*net.IPNet
}

// loadWebhookAllowPolicy 从环境加载策略；未设置返回默认「内网可用」档。
func loadWebhookAllowPolicy(lg loggateway.Logger) *webhookAllowPolicy {
	raw := strings.TrimSpace(os.Getenv(TwinOpenAPIWebhookAllowEnv))
	p := &webhookAllowPolicy{hosts: map[string]bool{}}
	if raw == "" {
		return p
	}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if item == "*" {
			p.allowAll = true
			lg.Warn("twinopenapi webhook 白名单被显式放开（*），回调地址不受限",
				loggateway.StepID("twinopenapi.webhook_allow_all"))
			continue
		}
		if _, cidr, err := net.ParseCIDR(item); err == nil {
			p.cidrs = append(p.cidrs, cidr)
			continue
		}
		if ip := net.ParseIP(item); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			p.cidrs = append(p.cidrs, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		p.hosts[item] = true
	}
	if len(p.hosts) > 0 || len(p.cidrs) > 0 {
		p.strict = true
	}
	return p
}

// allows 提交闸校验 webhook_url。不合规返回非 nil error（门面 400）。
func (p *webhookAllowPolicy) allows(rawURL string) error {
	if p == nil || p.allowAll {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return fmt.Errorf("invalid webhook_url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook_url scheme %q not allowed (http/https only)", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("webhook_url host is empty")
	}
	if !p.strict {
		// 默认档：提交闸放行，出口由拨号闸兜底（内网限定）。
		return nil
	}
	if p.hosts[host] || p.hosts[strings.ToLower(u.Host)] {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		for _, cidr := range p.cidrs {
			if cidr.Contains(ip) {
				return nil
			}
		}
	}
	return fmt.Errorf("webhook_url host %q not in allowlist (%s)", host, TwinOpenAPIWebhookAllowEnv)
}

// dialAllowed 拨号闸出口判定：仅放行私有/环回地址；allowAll 时全放。
// 公网、链路本地（含 169.254.169.254 云元数据）、组播、未指定地址一律拒绝。
func (p *webhookAllowPolicy) dialAllowed(ip net.IP) bool {
	if p == nil || p.allowAll {
		return true
	}
	if ip == nil {
		return false
	}
	if ip.IsPrivate() || ip.IsLoopback() {
		return true
	}
	return false
}

// httpClient 构造带拨号闸的投递客户端：DialContext 在连接建立前按
// 解析到的真实 IP 判定（封堵 DNS 重绑定）；禁用重定向（回调语义
// 不需要跟跳，跟跳会绕过提交闸把负载带到任意地址）。
func (p *webhookAllowPolicy) httpClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return nil, fmt.Errorf("webhook dial: bad address %q: %w", address, err)
				}
				ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
				if err != nil {
					return nil, fmt.Errorf("webhook dial: resolve %q: %w", host, err)
				}
				for _, ip := range ips {
					if p.dialAllowed(ip) {
						return dialer.DialContext(ctx, network, address)
					}
				}
				return nil, fmt.Errorf("webhook dial: %s resolves to non-intranet address, blocked", host)
			},
		},
	}
}
