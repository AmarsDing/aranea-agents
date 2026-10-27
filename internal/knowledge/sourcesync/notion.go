// notion.go —— G-38 W6 notion walker（设计 ②「search API 页级指纹＋429 尊重
// Retry-After＋cursor-invalid 回退清 cursor 全量 walk」× ④ 凭据铁律链运行时侧）。
//
// W6 as-built 登记（design ⑭ 条同款体裁）：
//   - **walk 域＝集成共享面全量出席**：search API filter object=page，不带 root
//     子树过滤（Notion 集成只看得见显式共享的页/库，共享模型是天然边界；
//     逐页拉 parent 链判祖先违反 ② 成本模型钉「禁 walk 期逐页拉」）。每轮
//     全量出席——tombstone 对账（⑥-3 active∧缺席→missing）以 item 表为唯一
//     全集，任何「只 emit 增量页」的省费优化都会把未变页误判 missing，拒做。
//   - **指纹＝last_edited_time 逐字串**（页级；runner needsWork 指纹漂移判定
//     直接复用）；cursor 列恒空串（Notion 无跨轮增量游标——next_cursor 只在
//     轮内分页用；设计「cursor-invalid 回退」实现为**轮内** next_cursor 失效
//     →清游标从头重 walk 一次，二次失效即 abort walk_failed）。
//   - **429/Retry-After**：每次 429 读 Retry-After 头（缺省 1s，单跳上限 60s），
//     轮内累计上限 8 跳——尊重但不无限烧租约（租约 TTL 30min 与心跳续期在
//     runner 侧）；超限返回普错归 walk_failed 入 2^n 分钟退避梯（⑤ 分级表）。
//   - **auth 铁律（④ 运行时侧）**：token 决议＝config credential_key → 凭据行
//     GetCredential → secret_ref env: 方案读环境变量 / enc: 信封
//     DecryptChannelSecretRef；缺行→credential_missing、解密失败→
//     credential_decrypt_failed、401/403→auth_failed（全经 AuthError 包装，
//     runner 整轮闸收 auth_paused 不重试烧配额）。
//   - **ConfirmDeleted＝pages.retrieve 三态**：404/archived=true→Confirmed；
//     200 活性→StillExists；401/403/429/5xx/网络病→Unknown 保守不删（令牌
//     吊销返 401 非 404，Unknown 即误删防线——设计⑥-4 头号事故模式同宗）。
//   - **Fetch＝page retrieve（归档闸）+blocks.children 全分页组装 markdown**；
//     单页体上限 512KiB 截断（轮预算 max_fetch_bytes_per_run 由 runner 另闸）；
//     MIME=text/markdown 直读通道（connector 无 frontmatter 语义，W3 同口径）。
//   - **external_id＝Notion page UUID**（API 自证形状，出入闸拒非 UUID——
//     防手改 items 行/畸形响应把路径段注进 URL）。
package sourcesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
)

const (
	notionAPIVersion     = "2022-06-28"
	notionDefaultBaseURL = "https://api.notion.com"
	notionPageSize       = 100
	notionMaxBodyBytes   = 512 << 10 // 单页组装体上限（截断保真，轮预算另闸）
	notionMax429Waits    = 8         // 轮内 429 累计等待跳数上限
	notionMax429Wait     = 60 * time.Second
	notionDefault429Wait = time.Second
)

// notionSleepFn 429 等待缝（测试注入记账不真睡；nil→time.Timer 实等）。
type notionSleepFn func(ctx context.Context, d time.Duration) error

// NotionWalkerDeps walker 装配面（Credentials/Crypto 必填——无凭据面即无法注密）。
type NotionWalkerDeps struct {
	Log         loggateway.Logger
	Credentials bizsync.CredentialRepo
	Crypto      *biz.CredentialCrypto
	HTTP        *http.Client // nil→outboundguard.NewClient(30s)（SSRF 护栏同 webresearch）
	BaseURL     string       // ""→https://api.notion.com（测试注入 httptest）
	Sleep       notionSleepFn
}

// NotionWalker notion 唯一 Walker 实现（无内部轮态——并发安全）。
type NotionWalker struct {
	d   NotionWalkerDeps
	lg  loggateway.Logger
	hc  *http.Client
	url string
}

// NewNotionWalker 构造（缺凭据面/Crypto 即早错——静默 nil 会在运行时把
// credential_missing 变成 panic）。
func NewNotionWalker(d NotionWalkerDeps) (*NotionWalker, error) {
	lg := d.Log
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if d.Credentials == nil || d.Crypto == nil {
		return nil, fmt.Errorf("sourcesync: notion walker requires Credentials and Crypto")
	}
	hc := d.HTTP
	if hc == nil {
		hc = outboundguard.NewClient(30 * time.Second)
	}
	base := strings.TrimRight(strings.TrimSpace(d.BaseURL), "/")
	if base == "" {
		base = notionDefaultBaseURL
	}
	return &NotionWalker{d: d, lg: lg.With(loggateway.Domain("knowledge/sourcesync")), hc: hc, url: base}, nil
}

// Kind 注册键（runner Deps.Walkers 用同键）。
func (w *NotionWalker) Kind() bizsync.Kind { return bizsync.KindNotion }

// ── 凭据决议（④ 运行时侧：行表 → env:/enc: → Bearer） ──

func (w *NotionWalker) resolveToken(ctx context.Context, conn bizsync.Connector) (string, error) {
	cfg, err := bizsync.ParseNotionConfig(conn.ConfigJSON)
	if err != nil {
		return "", err // 配置缺陷归 walk_failed（非 auth 家族）
	}
	cred, found, err := w.d.Credentials.GetCredential(ctx, conn.ID, cfg.CredentialKey)
	if err != nil {
		return "", fmt.Errorf("sourcesync: notion credential read: %w", err)
	}
	if !found {
		return "", NewAuthError(bizsync.ErrCodeCredentialMissing,
			fmt.Errorf("credential_key %q not configured", cfg.CredentialKey))
	}
	ref := strings.TrimSpace(cred.SecretRef)
	if bizsync.IsEnvSecretRef(ref) {
		if err := bizsync.ValidateEnvSecretRef(ref); err != nil {
			return "", err
		}
		tok := strings.TrimSpace(os.Getenv(strings.TrimPrefix(ref, "env:")))
		if tok == "" {
			return "", NewAuthError(bizsync.ErrCodeCredentialMissing,
				fmt.Errorf("env secret %q unset", ref))
		}
		return tok, nil
	}
	tok, err := w.d.Crypto.DecryptChannelSecretRef(ctx, ref)
	if err != nil {
		return "", NewAuthError(bizsync.ErrCodeCredentialDecryptFailed, err)
	}
	if strings.TrimSpace(tok) == "" {
		return "", NewAuthError(bizsync.ErrCodeCredentialDecryptFailed,
			fmt.Errorf("decrypted credential %q is empty", cfg.CredentialKey))
	}
	return strings.TrimSpace(tok), nil
}

// ── HTTP 原语（429/auth/错误体归一） ──

// notionErrorBody Notion 错误信封（object=error）。
type notionErrorBody struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// notionCursorInvalid 判定 400 validation_error 是否游标失效（重启全量信号）。
func notionCursorInvalid(status int, code, message string) bool {
	if status != http.StatusBadRequest {
		return false
	}
	if code != "" && code != "validation_error" {
		return false
	}
	return strings.Contains(strings.ToLower(message), "cursor")
}

// doJSON 单次调用（不做 429 重试——重试决策在调用面，持 Retry-After 语义需要
// 返回头）。auth/状态归一在此：401/403→AuthError，429→caller 读头等待重试。
func (w *NotionWalker) doJSON(ctx context.Context, method, path, token string, body any, out any) (retryAfter time.Duration, is429 bool, err error) {
	var rdr io.Reader
	if body != nil {
		raw, mErr := json.Marshal(body)
		if mErr != nil {
			return 0, false, fmt.Errorf("sourcesync: notion marshal: %w", mErr)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, w.url+path, rdr)
	if err != nil {
		return 0, false, fmt.Errorf("sourcesync: notion request build: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Notion-Version", notionAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := w.hc.Do(req)
	if err != nil {
		return 0, false, fmt.Errorf("sourcesync: notion %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, false, fmt.Errorf("sourcesync: notion read body: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if out != nil {
			if err := json.Unmarshal(raw, out); err != nil {
				return 0, false, fmt.Errorf("sourcesync: notion decode %s: %w", path, err)
			}
		}
		return 0, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return 0, false, NewAuthError(bizsync.ErrCodeAuthFailed,
			fmt.Errorf("notion %s: http %d", path, resp.StatusCode))
	case http.StatusTooManyRequests:
		wait := notionDefault429Wait
		if h := strings.TrimSpace(resp.Header.Get("Retry-After")); h != "" {
			if secs, perr := strconv.Atoi(h); perr == nil && secs >= 0 {
				wait = time.Duration(secs) * time.Second
			}
		}
		if wait > notionMax429Wait {
			wait = notionMax429Wait
		}
		return wait, true, nil
	}
	// 其余状态：解错误信封留 message 供观测（仍归 walk_failed 家族）。
	var eb notionErrorBody
	if jerr := json.Unmarshal(raw, &eb); jerr == nil && eb.Message != "" {
		return 0, false, &notionHTTPError{status: resp.StatusCode, code: eb.Code, message: eb.Message}
	}
	return 0, false, &notionHTTPError{status: resp.StatusCode}
}

// notionHTTPError 非 429/auth 的 HTTP 失败（带 status/code 供 cursor-invalid 判定）。
type notionHTTPError struct {
	status  int
	code    string
	message string
}

func (e *notionHTTPError) Error() string {
	if e.message != "" {
		return fmt.Sprintf("sourcesync: notion http %d (%s): %s", e.status, e.code, e.message)
	}
	return fmt.Sprintf("sourcesync: notion http %d", e.status)
}

// wait429 尊重 Retry-After（轮内累计跳数由调用面闸；ctx 取消即醒）。
func (w *NotionWalker) wait429(ctx context.Context, wait time.Duration) error {
	if w.d.Sleep != nil {
		return w.d.Sleep(ctx, wait)
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ── search 分页形状 ──

type notionRichText struct {
	PlainText string `json:"plain_text"`
}

type notionPage struct {
	ID             string                         `json:"id"`
	Archived       bool                           `json:"archived"`
	LastEditedTime string                         `json:"last_edited_time"`
	Properties     map[string]notionPropertyValue `json:"properties"`
}

type notionPropertyValue struct {
	Type  string           `json:"type"`
	Title []notionRichText `json:"title"`
}

func (p notionPage) title() string {
	for _, pv := range p.Properties {
		if pv.Type != "title" {
			continue
		}
		var sb strings.Builder
		for _, rt := range pv.Title {
			sb.WriteString(rt.PlainText)
		}
		if s := strings.TrimSpace(sb.String()); s != "" {
			return s
		}
	}
	return "untitled"
}

type notionSearchResponse struct {
	Results    []notionPage `json:"results"`
	HasMore    bool         `json:"has_more"`
	NextCursor string       `json:"next_cursor"`
}

// notionIDShape UUID 形状闸（32 hex 或 8-4-4-4-12 带横杠；防路径注入）。
func notionIDShape(id string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(id), "-", "")
	if len(compact) != 32 {
		return false
	}
	for _, r := range compact {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// ── Walker 三法 ──

// Walk ⑥-2：search 全量分页出席（页级指纹=last_edited_time；archived 页跳过
// 不出席——缺席走 missing→ConfirmDeleted(archived=Confirmed) 级联正道）。
// 游标列恒空串：轮内 next_cursor 失效（validation_error 含 cursor）即清游标
// 从头重 walk 一次（设计② cursor-invalid 回退），二次失效 abort。
func (w *NotionWalker) Walk(ctx context.Context, conn bizsync.Connector, prev CursorState, emit func(Snapshot) error) (CursorState, error) {
	token, err := w.resolveToken(ctx, conn)
	if err != nil {
		return prev, err
	}
	cfg, err := bizsync.ParseNotionConfig(conn.ConfigJSON)
	if err != nil {
		return prev, err
	}
	for restart := 0; ; restart++ {
		_, _, werr := w.walkOnce(ctx, token, cfg.Query, emit)
		if werr == nil {
			return "", nil
		}
		var he *notionHTTPError
		if restart == 0 && errors.As(werr, &he) && notionCursorInvalid(he.status, he.code, he.message) {
			w.lg.Warn("sourcesync: notion cursor invalid mid-walk, restart full walk",
				loggateway.Str("connector_id", conn.ID))
			continue
		}
		return prev, werr
	}
}

// walkOnce 单趟 search 分页（emit 出错原样返回——walker 契约：游标不动由
// Walk 包 prev 兜底）。返回已 emit 条数供观测。
func (w *NotionWalker) walkOnce(ctx context.Context, token, query string, emit func(Snapshot) error) (lastCursor string, emitted int, err error) {
	seen := map[string]struct{}{}
	waits429 := 0
	cursor := ""
	for {
		if ctx.Err() != nil {
			return cursor, emitted, ctx.Err()
		}
		reqBody := map[string]any{
			"page_size": notionPageSize,
			"filter":    map[string]string{"property": "object", "value": "page"},
		}
		if q := strings.TrimSpace(query); q != "" {
			reqBody["query"] = q
		}
		if cursor != "" {
			reqBody["start_cursor"] = cursor
		}
		var out notionSearchResponse
		wait, is429, callErr := w.doJSON(ctx, http.MethodPost, "/v1/search", token, reqBody, &out)
		if is429 {
			waits429++
			if waits429 > notionMax429Waits {
				return cursor, emitted, fmt.Errorf("sourcesync: notion search rate-limited beyond %d waits", notionMax429Waits)
			}
			if err := w.wait429(ctx, wait); err != nil {
				return cursor, emitted, err
			}
			continue
		}
		if callErr != nil {
			return cursor, emitted, callErr
		}
		for _, p := range out.Results {
			if p.Archived {
				continue // 归档页不出席（检索面已排除，防御性跳过重入保险）
			}
			if !notionIDShape(p.ID) {
				w.lg.Warn("sourcesync: notion skip malformed page id",
					loggateway.Str("page_id", p.ID))
				continue
			}
			if _, dup := seen[p.ID]; dup {
				continue // 分页间漂移去重（search 非稳定快照——同页两窗出现只出席一次）
			}
			seen[p.ID] = struct{}{}
			if err := emit(Snapshot{
				ExternalID:  p.ID,
				Fingerprint: p.LastEditedTime,
				DisplayPath: p.title(),
			}); err != nil {
				return cursor, emitted, err
			}
			emitted++
		}
		if !out.HasMore || out.NextCursor == "" {
			return cursor, emitted, nil
		}
		cursor = out.NextCursor
	}
}

// Fetch ⑥ 提交面：retrieve 验活性（archived→普错进 fetch_failed 桶，下轮缺席
// 走 ConfirmDeleted 正道）+ blocks.children 全分页组装 markdown（512KiB 截断）。
func (w *NotionWalker) Fetch(ctx context.Context, conn bizsync.Connector, externalID string) (*Payload, error) {
	if !notionIDShape(externalID) {
		return nil, fmt.Errorf("sourcesync: notion fetch %q rejected: not a page UUID", externalID)
	}
	token, err := w.resolveToken(ctx, conn)
	if err != nil {
		return nil, err
	}
	page, err := w.retrievePage(ctx, token, externalID)
	if err != nil {
		return nil, err
	}
	if page.Archived {
		return nil, fmt.Errorf("sourcesync: notion page %s archived between walk and fetch", externalID)
	}
	var sb strings.Builder
	if err := w.appendBlocks(ctx, token, externalID, &sb); err != nil {
		return nil, err
	}
	body := sb.String()
	if len(body) > notionMaxBodyBytes {
		body = body[:notionMaxBodyBytes] + "\n\n…(truncated)"
	}
	return &Payload{
		Body:      io.NopCloser(strings.NewReader(body)),
		MIME:      "text/markdown",
		SizeBytes: int64(len(body)),
	}, nil
}

// retrievePage GET /v1/pages/{id}（429 同 walk 纪律；auth 归 AuthError）。
func (w *NotionWalker) retrievePage(ctx context.Context, token, pageID string) (notionPage, error) {
	var page notionPage
	waits429 := 0
	for {
		if ctx.Err() != nil {
			return page, ctx.Err()
		}
		wait, is429, err := w.doJSON(ctx, http.MethodGet, "/v1/pages/"+pageID, token, nil, &page)
		if is429 {
			waits429++
			if waits429 > notionMax429Waits {
				return page, fmt.Errorf("sourcesync: notion page retrieve rate-limited beyond %d waits", notionMax429Waits)
			}
			if werr := w.wait429(ctx, wait); werr != nil {
				return page, werr
			}
			continue
		}
		if err != nil {
			return page, err
		}
		return page, nil
	}
}

// notionBlock 渲染所需最小投影（未知 type 跳过不杀全页）。
type notionBlock struct {
	Type string `json:"type"`

	Paragraph        *notionBlockText `json:"paragraph"`
	Heading1         *notionBlockText `json:"heading_1"`
	Heading2         *notionBlockText `json:"heading_2"`
	Heading3         *notionBlockText `json:"heading_3"`
	BulletedListItem *notionBlockText `json:"bulleted_list_item"`
	NumberedListItem *notionBlockText `json:"numbered_list_item"`
	ToDo             *notionBlockToDo `json:"to_do"`
	Toggle           *notionBlockText `json:"toggle"`
	Quote            *notionBlockText `json:"quote"`
	Callout          *notionBlockText `json:"callout"`
	Code             *notionBlockCode `json:"code"`
	ChildPage        *notionChildPage `json:"child_page"`
}

type notionBlockText struct {
	RichText []notionRichText `json:"rich_text"`
}

func (t *notionBlockText) plain() string {
	if t == nil {
		return ""
	}
	var sb strings.Builder
	for _, rt := range t.RichText {
		sb.WriteString(rt.PlainText)
	}
	return sb.String()
}

type notionBlockToDo struct {
	RichText []notionRichText `json:"rich_text"`
	Checked  bool             `json:"checked"`
}

type notionBlockCode struct {
	RichText []notionRichText `json:"rich_text"`
	Language string           `json:"language"`
}

func (c *notionBlockCode) plain() string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, rt := range c.RichText {
		sb.WriteString(rt.PlainText)
	}
	return sb.String()
}

type notionChildPage struct {
	Title string `json:"title"`
}

type notionChildrenResponse struct {
	Results    []notionBlock `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor"`
}

// renderBlock 单块→markdown 行（未知类型空串=跳过）。
func renderBlock(b notionBlock) string {
	switch b.Type {
	case "paragraph":
		return b.Paragraph.plain() + "\n\n"
	case "heading_1":
		return "# " + b.Heading1.plain() + "\n\n"
	case "heading_2":
		return "## " + b.Heading2.plain() + "\n\n"
	case "heading_3":
		return "### " + b.Heading3.plain() + "\n\n"
	case "bulleted_list_item":
		return "- " + b.BulletedListItem.plain() + "\n"
	case "numbered_list_item":
		return "1. " + b.NumberedListItem.plain() + "\n"
	case "to_do":
		mark := "[ ]"
		if b.ToDo != nil && b.ToDo.Checked {
			mark = "[x]"
		}
		return "- " + mark + " " + b.ToDo.plain() + "\n"
	case "toggle":
		return b.Toggle.plain() + "\n\n"
	case "quote":
		return "> " + b.Quote.plain() + "\n\n"
	case "callout":
		return "> " + b.Callout.plain() + "\n\n"
	case "code":
		lang := ""
		if b.Code != nil {
			lang = b.Code.Language
		}
		return "```" + lang + "\n" + b.Code.plain() + "\n```\n\n"
	case "child_page":
		if b.ChildPage != nil {
			return "# " + b.ChildPage.Title + "\n\n"
		}
		return ""
	case "divider":
		return "---\n\n"
	default:
		return ""
	}
}

func (t *notionBlockToDo) plain() string {
	if t == nil {
		return ""
	}
	var sb strings.Builder
	for _, rt := range t.RichText {
		sb.WriteString(rt.PlainText)
	}
	return sb.String()
}

// appendBlocks blocks.children 全分页组装（429/auth 纪律同 retrievePage；
// 体达上限即停分页——截断保真不重拉）。
func (w *NotionWalker) appendBlocks(ctx context.Context, token, pageID string, sb *strings.Builder) error {
	cursor := ""
	waits429 := 0
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		path := "/v1/blocks/" + pageID + "/children?page_size=" + strconv.Itoa(notionPageSize)
		if cursor != "" {
			path += "&start_cursor=" + cursor
		}
		var out notionChildrenResponse
		wait, is429, err := w.doJSON(ctx, http.MethodGet, path, token, nil, &out)
		if is429 {
			waits429++
			if waits429 > notionMax429Waits {
				return fmt.Errorf("sourcesync: notion blocks rate-limited beyond %d waits", notionMax429Waits)
			}
			if werr := w.wait429(ctx, wait); werr != nil {
				return werr
			}
			continue
		}
		if err != nil {
			return err
		}
		for _, b := range out.Results {
			sb.WriteString(renderBlock(b))
			if sb.Len() > notionMaxBodyBytes {
				return nil // 截断保真：体上限即停（Fetch 侧落截断标记）
			}
		}
		if !out.HasMore || out.NextCursor == "" {
			return nil
		}
		cursor = out.NextCursor
	}
}

// ConfirmDeleted ⑥-4 三态＝pages.retrieve：404/archived→Confirmed；活性
// →StillExists；401/403/429/5xx/网络病→Unknown（保守不删）。
func (w *NotionWalker) ConfirmDeleted(ctx context.Context, conn bizsync.Connector, externalID string) bizsync.DeletionVerdict {
	if !notionIDShape(externalID) {
		return bizsync.VerdictUnknown
	}
	token, err := w.resolveToken(ctx, conn)
	if err != nil {
		return bizsync.VerdictUnknown
	}
	page, err := w.retrievePage(ctx, token, externalID)
	if err != nil {
		var he *notionHTTPError
		if errors.As(err, &he) && he.status == http.StatusNotFound {
			return bizsync.VerdictConfirmed
		}
		return bizsync.VerdictUnknown
	}
	if page.Archived {
		return bizsync.VerdictConfirmed
	}
	return bizsync.VerdictStillExists
}
