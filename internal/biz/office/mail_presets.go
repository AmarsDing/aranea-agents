package office

// 服务商预设与连接错误归类（09-邮件.md §3.2「添加邮箱向导」、§6 M1）。
// 只填邮箱地址 → 按域名认；认不出（企业邮常用自己的域名）再查 MX 记录认；
// 都认不出就让用户手动填服务器。授权码怎么生成的说明在前端文案里（按 Key 取）。

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"
)

// 连接错误类别（存进账号 last_error、返给向导；不含上游原文）。
const (
	MailErrKindDNS         = "dns"         // 域名解析不了
	MailErrKindUnreachable = "unreachable" // 连不上 / 超时 / 被拒
	MailErrKindTLS         = "tls"         // 证书或加密握手失败
	MailErrKindAuth        = "auth"        // 用户名或授权码不对 / 服务没开
	MailErrKindProtocol    = "protocol"    // 服务器回了看不懂或拒绝的响应
)

// MailProviderPreset 一家服务商的收发服务器。
type MailProviderPreset struct {
	Key       string // 前端文案键：qq / exmail / 163 / 126 / yeah / aliyun_qiye / aliyun / netease_qiye / sina / 139 / gmail / outlook
	Name      string
	IMAPHost  string
	IMAPPort  int
	IMAPTLS   string
	SMTPHost  string
	SMTPPort  int
	SMTPTLS   string
	Supported bool // false：要 OAuth 登录（P3），向导直接说明
	domains   []string
	mxHints   []string
}

var mailProviderPresets = []MailProviderPreset{
	{Key: "qq", Name: "QQ 邮箱", IMAPHost: "imap.qq.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.qq.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"qq.com", "foxmail.com", "vip.qq.com"}},
	{Key: "exmail", Name: "腾讯企业邮", IMAPHost: "imap.exmail.qq.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.exmail.qq.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		mxHints: []string{"exmail.qq.com", "mxbiz1.qq.com", "mxbiz2.qq.com"}},
	{Key: "163", Name: "网易 163 邮箱", IMAPHost: "imap.163.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.163.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"163.com"}},
	{Key: "126", Name: "网易 126 邮箱", IMAPHost: "imap.126.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.126.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"126.com"}},
	{Key: "yeah", Name: "网易 yeah.net", IMAPHost: "imap.yeah.net", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.yeah.net", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"yeah.net"}},
	{Key: "netease_qiye", Name: "网易企业邮", IMAPHost: "imap.qiye.163.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.qiye.163.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		mxHints: []string{"qiye.163.com", "qiye163mx"}},
	{Key: "aliyun_qiye", Name: "阿里企业邮", IMAPHost: "imap.qiye.aliyun.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.qiye.aliyun.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		mxHints: []string{"mxhichina.com", "qiye.aliyun.com"}},
	{Key: "aliyun", Name: "阿里邮箱（个人）", IMAPHost: "imap.aliyun.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.aliyun.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"aliyun.com"}},
	{Key: "sina", Name: "新浪邮箱", IMAPHost: "imap.sina.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.sina.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"sina.com", "sina.cn", "vip.sina.com"}},
	{Key: "139", Name: "139 邮箱", IMAPHost: "imap.139.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.139.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"139.com"}},
	{Key: "gmail", Name: "Gmail", IMAPHost: "imap.gmail.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp.gmail.com", SMTPPort: 465, SMTPTLS: MailTLSTLS, Supported: true,
		domains: []string{"gmail.com", "googlemail.com"}, mxHints: []string{"google.com", "googlemail.com"}},
	// 微软个人邮箱已停用密码 / 授权码登录 IMAP，只能 OAuth（P3）。
	{Key: "outlook", Name: "Outlook / Hotmail", IMAPHost: "outlook.office365.com", IMAPPort: 993, IMAPTLS: MailTLSTLS, SMTPHost: "smtp-mail.outlook.com", SMTPPort: 587, SMTPTLS: MailTLSStartTLS, Supported: false,
		domains: []string{"outlook.com", "hotmail.com", "live.com", "msn.com"}, mxHints: []string{"outlook.com", "protection.outlook.com"}},
}

// MailProviderPresets 全部预设（向导里「换一家」的下拉用）。
func MailProviderPresets() []MailProviderPreset { return mailProviderPresets }

// MailPresetByKey 按键取预设。
func MailPresetByKey(key string) (*MailProviderPreset, bool) {
	for i := range mailProviderPresets {
		if mailProviderPresets[i].Key == key {
			return &mailProviderPresets[i], true
		}
	}
	return nil, false
}

// MailMXLookup 查 MX（测试注入）。
type MailMXLookup func(ctx context.Context, domain string) ([]*net.MX, error)

// DetectMailProvider 先按域名认，认不出查 MX。返回 (预设, 怎么认出来的：domain | mx | "")。
func DetectMailProvider(ctx context.Context, email string, lookup MailMXLookup) (*MailProviderPreset, string) {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return nil, ""
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	for i := range mailProviderPresets {
		for _, d := range mailProviderPresets[i].domains {
			if d == domain {
				return &mailProviderPresets[i], "domain"
			}
		}
	}
	if lookup == nil {
		lookup = net.DefaultResolver.LookupMX
	}
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	mxs, err := lookup(cctx, domain)
	if err != nil {
		return nil, ""
	}
	for _, mx := range mxs {
		host := strings.ToLower(strings.TrimSuffix(mx.Host, "."))
		for i := range mailProviderPresets {
			for _, hint := range mailProviderPresets[i].mxHints {
				if strings.Contains(host, hint) {
					return &mailProviderPresets[i], "mx"
				}
			}
		}
	}
	return nil, ""
}

// classifyMailDialErr 建连阶段的错误归类。
func classifyMailDialErr(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return MailErrKindDNS
	}
	var certErr *tls.CertificateVerificationError
	var recErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &recErr) || strings.Contains(err.Error(), "tls:") {
		return MailErrKindTLS
	}
	var netErr net.Error
	var opErr *net.OpError
	if errors.As(err, &netErr) || errors.As(err, &opErr) {
		return MailErrKindUnreachable
	}
	return MailErrKindProtocol
}

// mailStatusForKind 错误类别 → 账号状态。
func mailStatusForKind(kind string) string {
	if kind == MailErrKindAuth {
		return MailStatusAuthFailed
	}
	return MailStatusUnreachable
}
