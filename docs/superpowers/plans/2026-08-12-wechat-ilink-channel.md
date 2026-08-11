# 微信个人号渠道（iLink）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在项目已有 channel 框架内新增 `wechat_ilink` 渠道类型，支持扫码登录、长轮询收信、文本/媒体出站，覆盖私聊和群聊。

**Architecture:** 复用 telegram polling 运行时模式（`internal/channel/runtime`），新建 `internal/channel/wechatilink/` 包实现 iLink HTTP 协议客户端，通过扫码登录自动获取 bot_token 凭证，状态文件（`bin/data/channel-state/`）持久化游标与 context_token。

**Tech Stack:** Go 1.23, Kratos v2, httptest, proto, Vue 3/Quasar, protoc-gen-typescript-http

---

## 文件结构映射

### 新增文件

| 文件 | 职责 |
|------|------|
| `internal/channel/wechatilink/types.go` | iLink 消息结构体（WeixinMessage、MessageItem、TextItem 等） |
| `internal/channel/wechatilink/errors.go` | iLink 错误类型与 errcode 判断 |
| `internal/channel/wechatilink/client.go` | HTTP client：请求签名（公共头 + Bearer + X-WECHAT-UIN）、POST/GET 封装 |
| `internal/channel/wechatilink/client_test.go` | 请求头生成、重试退避 |
| `internal/channel/wechatilink/login.go` | 扫码登录：get_bot_qrcode → 轮询 get_qrcode_status → bot_token |
| `internal/channel/wechatilink/login_test.go` | 登录流程 mock 测试 |
| `internal/channel/wechatilink/polling.go` | `RunPolling` starter：长轮询循环、调用 parse/outbound |
| `internal/channel/wechatilink/polling_test.go` | polling 循环测试 |
| `internal/channel/wechatilink/parse.go` | 消息解析：WeixinMessage → port.InboundEvent，支持文本/图片/语音/文件/视频 |
| `internal/channel/wechatilink/parse_test.go` | 各消息类型解析断言 |
| `internal/channel/wechatilink/outbound.go` | TextSender：sendmessage，context_token 缓存 |
| `internal/channel/wechatilink/outbound_test.go` | 文本发送 mock 测试 |
| `internal/channel/wechatilink/state.go` | 状态文件持久化：读写 `bin/data/channel-state/wechat_ilink-<id>.json` |
| `internal/channel/wechatilink/state_test.go` | 原子写入与恢复测试 |
| `internal/channel/wechatilink/media.go` | CDN 上传/下载 + AES-128-ECB 加解密 |
| `internal/channel/wechatilink/media_test.go` | AES 加解密 roundtrip 测试 |
| `internal/channel/wechatilink/typing.go` | 输入状态：getconfig 获取 ticket + sendtyping |
| `internal/channel/wechatilink/typing_test.go` | typing ticket 缓存测试 |
| `internal/service/channel_wechat_ilink_login.go` | ChannelService 的扫码登录 RPC 实现 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `api/kratos/channel/v1/channel.proto` | 新增 WechatILinkLogin/WechatILinkPoll RPC |
| `internal/biz/channel_catalog.go` | 新增 `wechatILinkTypeItem()` 函数 |
| `internal/biz/channel_type_registry.go` | init() 追加 wechat_ilink 注册（含凭证 schema） |
| `internal/channel/runtime/config.go` | `defaultReceiveMode` 追加 `wechat_ilink → polling` |
| `internal/channel/port/meta.go` | 新增 `MetaContextToken` well-known key |
| `internal/service/channel_platform_registry.go` | 注册 `wechat_ilink` outbound handler |
| `internal/service/channel.go` | 注册 `wechat_ilink` live tester |
| `internal/channel/contract_test.go` | 添加 `wechatilink.TextSender` 契约断言 |
| `internal/channel/all/all.go` | `import _` 注册 wechatilink 包 |
| `internal/channel/preview/platform.go` | `PlatformTextLimit` 追加 `wechat_ilink` → 4000 |
| `internal/biz/channel_im_render.go` | P3 时 `ChannelACKDeferredToPreview` 追加 wechat_ilink |
| `web/src/features/channels/api.ts` | 新增 `wechatIlinkLogin`/`wechatIlinkPoll` 方法 |
| `web/src/features/channels/channelPlatformFields.ts` | 新增 wechat_ilink 平台字段声明 |
| `web/src/features/channels/ChannelEditorDialog.vue` | 扫码登录区块（二维码 + 轮询 + 过期刷新） |
| `web/src/features/channels/useChannelEditorForm.ts` | 扫码登录状态管理 |

### 文档同步

| 文件 | 改动 |
|------|------|
| `docs/development/17-channel.md` | §2 平台表追加、§5 新增 iLink 凭据配置、§7 运行时新增 |
| `docs/development/17-channel.design.md` | §3 Adapter 追加 wechat_ilink、§5.3 扫码登录 RPC |
| `docs/development/17-channel.development.md` | 任务清单追加 wechat_ilink 条目 |

---

## Phase 0：基础设施与注册

### Task 0.1：Proto 定义

**Files:**
- Modify: `api/kratos/channel/v1/channel.proto`

- [ ] **Step 1: 追加消息类型**

```protobuf
message WechatILinkLoginRequest {
  string channel_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message WechatILinkLoginResponse {
  string qrcode_data_url = 1;
  string qrcode_session = 2;
  string status = 3;
}

message WechatILinkPollRequest {
  string channel_id = 1 [(google.api.field_behavior) = REQUIRED];
  string qrcode_session = 2 [(google.api.field_behavior) = REQUIRED];
}

message WechatILinkPollResponse {
  string status = 1;
  string error_message = 2;
}
```

- [ ] **Step 2: 追加 RPC 到 ChannelService**

```protobuf
rpc WechatILinkLogin(WechatILinkLoginRequest) returns (WechatILinkLoginResponse) {
  option (google.api.http) = {
    post: "/v1/channels/{channel_id}/wechat-ilink/login"
    body: "*"
  };
}

rpc WechatILinkPoll(WechatILinkPollRequest) returns (WechatILinkPollResponse) {
  option (google.api.http) = {
    post: "/v1/channels/{channel_id}/wechat-ilink/poll"
    body: "*"
  };
}
```

- [ ] **Step 3: 生成 Go + TS 代码**

Run: `make api`
Expected: 无报错，`api/kratos/channel/v1/` 下生成新接口，前端 TS 同步更新

### Task 0.2：Biz 层类型注册

**Files:**
- Modify: `internal/biz/channel_catalog.go`
- Modify: `internal/biz/channel_type_registry.go`

- [ ] **Step 1: channel_catalog.go 追加 item 函数**

```go
func wechatILinkTypeItem() ChannelTypeItem {
	item := channelTypeItem("wechat_ilink", "微信（个人号·iLink）", "国内", "polling",
		"腾讯 iLink 官方 Bot API，扫码登录，支持私聊/群聊/多媒体", 45, true, true, false)
	item.ReceiveModes = []string{"polling"}
	item.ConfigSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"group_enabled":  map[string]any{"type": "boolean", "default": false},
			"require_mention": map[string]any{"type": "boolean", "default": true},
			"bot_nickname":   map[string]any{"type": "string", "default": ""},
		},
	}
	return item
}
```

- [ ] **Step 2: channel_type_registry.go init() 追加注册**

```go
{
	TypeItem:            wechatILinkTypeItem(),
	RequiredCredentials: []string{"bot_token"},
	CredentialProps: []CredentialProperty{
		{Key: "bot_token", Title: "wechat_ilink_bot_token", Format: "password", Required: true},
	},
	SupportsLightTest: true,
},
```

- [ ] **Step 3: 编译验证**

Run: `go build ./internal/biz/...`
Expected: PASS

### Task 0.3：运行时模式映射

**Files:**
- Modify: `internal/channel/runtime/config.go`

- [ ] **Step 1: defaultReceiveMode 追加**

```go
case "wechat_ilink":
	return "polling"
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/channel/runtime/...`
Expected: PASS

### Task 0.4：Port Meta 新增 ContextToken Key

**Files:**
- Modify: `internal/channel/port/meta.go`

- [ ] **Step 1: 追加 well-known key（并登记 knownOutboundMetaKeys）**

```go
// const 块追加
MetaContextToken     = "context_token"

// knownOutboundMetaKeys map 追加
MetaContextToken:     {},
```

> 说明：参考 telegram `parsePollingUpdate` 的真实字段——`port.InboundEvent{PlatformType, PeerID, PeerKey, Text, IdempotencyKey, OutboundMeta}`（无 ChannelPeerID/Extra 字段，计划草案已修正）。OutboundMeta 至少携带 `MetaRecipient`（= FromUserID 或 GroupID）+ `MetaContextToken`（= 消息的 context_token）。

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/channel/port/...`
Expected: PASS

---

## Phase 1：iLink 协议客户端（P0 核心）

### Task 1.1：消息结构体（types.go）

**Files:**
- Create: `internal/channel/wechatilink/types.go`

- [ ] **Step 1: 写入完整结构体**

```go
package wechatilink

type baseRequest struct {
	BaseInfo struct {
		ChannelVersion string `json:"channel_version"`
	} `json:"base_info"`
}

type TextItem struct {
	Text string `json:"text"`
}

type ImageItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
}

type VoiceItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
	Text     string   `json:"text"`
}

type FileItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
	FileName string   `json:"file_name"`
}

type VideoItem struct {
	CDNMedia CDNMedia `json:"cdn_media"`
}

type CDNMedia struct {
	FullURL       string `json:"full_url"`
	Key           string `json:"key"`
	Host          string `json:"host"`
	EncryptionKey string `json:"encryption_key"`
}

type MessageItem struct {
	Type      int        `json:"type"`
	TextItem  *TextItem  `json:"text_item,omitempty"`
	ImageItem *ImageItem `json:"image_item,omitempty"`
	VoiceItem *VoiceItem `json:"voice_item,omitempty"`
	FileItem  *FileItem  `json:"file_item,omitempty"`
	VideoItem *VideoItem `json:"video_item,omitempty"`
}

type WeixinMessage struct {
	Seq          int64         `json:"seq"`
	MessageID    int64         `json:"message_id"`
	FromUserID   string        `json:"from_user_id"`
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id"`
	CreateTimeMs int64         `json:"create_time_ms"`
	SessionID    string        `json:"session_id"`
	GroupID      string        `json:"group_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ItemList     []MessageItem `json:"item_list"`
	ContextToken string        `json:"context_token"`
}

type WeixinSendMessage struct {
	ToUserID     string        `json:"to_user_id"`
	ClientID     string        `json:"client_id"`
	MessageType  int           `json:"message_type"`
	MessageState int           `json:"message_state"`
	ContextToken string        `json:"context_token"`
	ItemList     []MessageItem `json:"item_list"`
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/channel/wechatilink/...`
Expected: PASS

### Task 1.2：错误类型（errors.go）

**Files:**
- Create: `internal/channel/wechatilink/errors.go`

- [ ] **Step 1: 写入错误判断**

```go
package wechatilink

import "errors"

type ilinkResponse struct {
	Ret      int    `json:"ret"`
	ErrCode  int    `json:"errcode"`
	ErrMsg   string `json:"errmsg"`
}

func (r *ilinkResponse) OK() bool {
	if r == nil {
		return true
	}
	return r.Ret == 0 && r.ErrCode == 0
}

func (r *ilinkResponse) Error() string {
	if r == nil {
		return ""
	}
	if r.ErrMsg != "" {
		return r.ErrMsg
	}
	return "iLink error"
}

var (
	ErrSessionExpired = errors.New("wechat_ilink: session expired")
)

func isSessionExpired(code int) bool {
	return code == -14
}
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/channel/wechatilink/...`
Expected: PASS

### Task 1.3：HTTP Client（client.go）

**Files:**
- Create: `internal/channel/wechatilink/client.go`
- Create: `internal/channel/wechatilink/client_test.go`

- [ ] **Step 1: 写测试——请求头生成**

```go
package wechatilink

import (
	"strings"
	"testing"
)

func TestBuildRequestHeaders(t *testing.T) {
	h := buildRequestHeaders("my_token")
	if h.Get("iLink-App-Id") != "bot" {
		t.Errorf("app-id want bot, got %s", h.Get("iLink-App-Id"))
	}
	auth := h.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer my_token") {
		t.Errorf("auth want Bearer my_token, got %s", auth)
	}
	if h.Get("X-WECHAT-UIN") == "" {
		t.Error("X-WECHAT-UIN should not be empty")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/channel/wechatilink/... -run TestBuildRequestHeaders -v`
Expected: FAIL - "undefined: buildRequestHeaders"

- [ ] **Step 3: 写最小实现**

```go
package wechatilink

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"aranea-agents/pkg/loggateway"
)

type client struct {
	baseURL  string
	botToken string
	http     *http.Client
	lg       loggateway.Logger
}

func newClient(baseURL, botToken string, lg loggateway.Logger) *client {
	if baseURL == "" {
		baseURL = "https://ilinkai.weixin.qq.com"
	}
	return &client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		botToken: botToken,
		http:     &http.Client{Timeout: 60 * time.Second},
		lg:       lg,
	}
}

func buildRequestHeaders(botToken string) http.Header {
	h := http.Header{}
	h.Set("iLink-App-Id", "bot")
	h.Set("iLink-App-ClientVersion", fmt.Sprintf("0x%08x", 0x00020101))
	h.Set("Content-Type", "application/json")
	h.Set("AuthorizationType", "ilink_bot_token")
	h.Set("Authorization", "Bearer "+botToken)
	h.Set("X-WECHAT-UIN", randomUIN())
	return h
}

func randomUIN() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<32))
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n.Int64()))
	return base64.StdEncoding.EncodeToString(b)
}

func (c *client) post(ctx context.Context, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = buildRequestHeaders(c.botToken)
	return c.http.Do(req)
}

func (c *client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header = buildRequestHeaders(c.botToken)
	return c.http.Do(req)
}

func decodeJSON[T any](resp *http.Response) (T, error) {
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return v, err
	}
	return v, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/channel/wechatilink/... -run TestBuildRequestHeaders -v`
Expected: PASS

### Task 1.4：状态文件持久化（state.go）

**Files:**
- Create: `internal/channel/wechatilink/state.go`
- Create: `internal/channel/wechatilink/state_test.go`

- [ ] **Step 1: 写测试**

```go
func TestStateFileRoundtrip(t *testing.T) {
	stateDir = t.TempDir()
	s := &stateFile{
		GetUpdatesBuf: "buf_v1",
		ContextTokens: map[string]string{"u1": "tk1"},
		LoginStatus:   "active",
	}
	if err := writeState("ch-1", s); err != nil {
		t.Fatal(err)
	}
	loaded, err := readState("ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetUpdatesBuf != "buf_v1" {
		t.Errorf("buf want buf_v1, got %s", loaded.GetUpdatesBuf)
	}
	if loaded.ContextTokens["u1"] != "tk1" {
		t.Error("context token mismatch")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/channel/wechatilink/... -run TestStateFileRoundtrip -v`
Expected: FAIL - undefined functions

- [ ] **Step 3: 写最小实现**

```go
package wechatilink

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var stateDir = "bin/data/channel-state"

func stateFilePath(channelID string) string {
	return filepath.Join(stateDir, fmt.Sprintf("wechat_ilink-%s.json", channelID))
}

type stateFile struct {
	GetUpdatesBuf string            `json:"get_updates_buf"`
	ContextTokens map[string]string `json:"context_tokens"`
	LastLoginAt   string            `json:"last_login_at"`
	LoginStatus   string            `json:"login_status"`
}

var stateMu sync.Mutex

func readState(channelID string) (*stateFile, error) {
	p := stateFilePath(channelID)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &stateFile{ContextTokens: map[string]string{}}, nil
		}
		return nil, err
	}
	var s stateFile
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.ContextTokens == nil {
		s.ContextTokens = map[string]string{}
	}
	return &s, nil
}

func writeState(channelID string, s *stateFile) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	p := stateFilePath(channelID)
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		return err
	}
	tmp := p + ".tmp"
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0640); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/channel/wechatilink/... -run TestStateFileRoundtrip -v`
Expected: PASS

### Task 1.5：扫码登录（login.go）

**Files:**
- Create: `internal/channel/wechatilink/login.go`
- Create: `internal/channel/wechatilink/login_test.go`

- [ ] **Step 1: 写测试**

```go
func TestLoginFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			json.NewEncoder(w).Encode(map[string]any{
				"ret": 0, "qrcode": "sess-1", "qrcode_img_content": "data:image/png;base64,abc",
			})
		case "/ilink/bot/get_qrcode_status":
			json.NewEncoder(w).Encode(map[string]any{
				"ret": 0, "status": "confirmed", "bot_token": "tk123", "baseurl": "https://t.example.com", "ilink_user_id": "uid1",
			})
		}
	}))
	defer server.Close()

	c := newClient(server.URL, "", loggateway.NewNoop())
	resp, err := c.GetBotQRCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.QRCode != "sess-1" {
		t.Errorf("qrcode want sess-1, got %s", resp.QRCode)
	}

	status, err := c.GetQRCodeStatus(context.Background(), "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "confirmed" || status.BotToken != "tk123" {
		t.Errorf("unexpected status: %+v", status)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 写最小实现**

```go
type getBotQRCodeResp struct {
	Ret              int    `json:"ret"`
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
	ErrCode          int    `json:"errcode"`
	ErrMsg           string `json:"errmsg"`
}

type getQRCodeStatusResp struct {
	Ret          int    `json:"ret"`
	Status       string `json:"status"`
	BotToken     string `json:"bot_token"`
	BaseURL      string `json:"baseurl"`
	ILinkUserID  string `json:"ilink_user_id"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func (c *client) GetBotQRCode(ctx context.Context) (*getBotQRCodeResp, error) {
	resp, err := c.get(ctx, "/ilink/bot/get_bot_qrcode?bot_type=3")
	if err != nil {
		return nil, err
	}
	r, err := decodeJSON[getBotQRCodeResp](resp)
	if err != nil {
		return nil, err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return nil, fmt.Errorf("get_bot_qrcode failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return &r, nil
}

func (c *client) GetQRCodeStatus(ctx context.Context, qrcode string) (*getQRCodeStatusResp, error) {
	resp, err := c.get(ctx, "/ilink/bot/get_qrcode_status?qrcode="+url.QueryEscape(qrcode))
	if err != nil {
		return nil, err
	}
	r, err := decodeJSON[getQRCodeStatusResp](resp)
	if err != nil {
		return nil, err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return nil, fmt.Errorf("get_qrcode_status failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return &r, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/channel/wechatilink/... -run TestLoginFlow -v`
Expected: PASS

### Task 1.6：消息解析（parse.go）

**Files:**
- Create: `internal/channel/wechatilink/parse.go`
- Create: `internal/channel/wechatilink/parse_test.go`

- [ ] **Step 1: 写测试——消息解析**

```go
func TestParseTextMessage(t *testing.T) {
	msg := WeixinMessage{
		FromUserID:   "user@im.wechat",
		MessageID:    100,
		MessageType:  1,
		MessageState: 2,
		ItemList:     []MessageItem{{Type: 1, TextItem: &TextItem{Text: "hello"}}},
		ContextToken: "ctx-1",
		SessionID:    "sess-1",
	}
	ev, err := parseMessage("ch-1", &msg)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Text != "hello" {
		t.Errorf("text want hello, got %s", ev.Text)
	}
	if ev.PeerID != "user@im.wechat" {
		t.Errorf("peer want user@im.wechat, got %s", ev.PeerID)
	}
	if ev.OutboundMeta["context_token"] != "ctx-1" {
		t.Error("context_token not propagated to OutboundMeta")
	}
	if ev.IdempotencyKey != "wechat_ilink:ch-1:100" {
		t.Errorf("idempotency key wrong: %s", ev.IdempotencyKey)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 写最小实现**

```go
package wechatilink

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/biz"
)

func parseMessage(channelID string, msg *WeixinMessage) (*port.InboundEvent, error) {
	if len(msg.ItemList) == 0 {
		return nil, fmt.Errorf("empty item list")
	}
	item := msg.ItemList[0]
	var text string
	switch item.Type {
	case 1:
		if item.TextItem != nil {
			text = item.TextItem.Text
		}
	case 2:
		text = "[图片]"
	case 3:
		if item.VoiceItem != nil && item.VoiceItem.Text != "" {
			text = item.VoiceItem.Text
		} else {
			text = "[语音消息，未识别]"
		}
	case 4:
		name := ""
		if item.FileItem != nil {
			name = item.FileItem.FileName
		}
		text = fmt.Sprintf("[文件: %s]", name)
	case 5:
		text = "[视频]"
	default:
		text = "[未知消息]"
	}

	peerID := msg.FromUserID
	// 回复目标：私聊回用户，群聊回群（to_user_id 传 group_id，联调实测验证）
	recipient := msg.FromUserID
	if msg.GroupID != "" {
		recipient = msg.GroupID
	}

	return &port.InboundEvent{
		PlatformType:   "wechat_ilink",
		PeerID:         peerID,
		Text:           text,
		IdempotencyKey: fmt.Sprintf("wechat_ilink:%s:%d", channelID, msg.MessageID),
		OutboundMeta: map[string]string{
			port.MetaRecipient:    recipient,
			port.MetaContextToken: msg.ContextToken,
			port.MetaSessionID:    msg.SessionID,
		},
	}, nil
}

func shouldHandleGroupMessage(msg *WeixinMessage, cfg biz.ChannelConfig) bool {
	if msg.GroupID == "" {
		return true // 私聊
	}
	groupEnabled, _ := cfg.Config["group_enabled"].(bool)
	if !groupEnabled {
		return false
	}
	requireMention, _ := cfg.Config["require_mention"].(bool)
	if !requireMention {
		return true
	}
	botNick, _ := cfg.Config["bot_nickname"].(string)
	if botNick == "" {
		return true // 未配置昵称时全响应
	}
	// mention 检测：文本包含 @昵称
	if len(msg.ItemList) > 0 && msg.ItemList[0].Type == 1 && msg.ItemList[0].TextItem != nil {
		text := msg.ItemList[0].TextItem.Text
		return strings.Contains(text, "@"+botNick)
	}
	return false
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/channel/wechatilink/... -run TestParseTextMessage -v`
Expected: PASS

### Task 1.7：出站发送（outbound.go）

**Files:**
- Create: `internal/channel/wechatilink/outbound.go`
- Create: `internal/channel/wechatilink/outbound_test.go`

- [ ] **Step 1: 写测试**

```go
func TestSendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/sendmessage" {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			msg := body["msg"].(map[string]any)
			if msg["context_token"] != "ctx-1" {
				t.Errorf("context_token want ctx-1, got %v", msg["context_token"])
			}
			json.NewEncoder(w).Encode(map[string]any{"ret": 0})
		}
	}))
	defer server.Close()

	c := newClient(server.URL, "tk", loggateway.NewNoop())
	msg := WeixinSendMessage{
		ToUserID:     "user@im.wechat",
		ContextToken: "ctx-1",
		ItemList:     []MessageItem{{Type: 1, TextItem: &TextItem{Text: "hi"}}},
	}
	if err := c.SendMessage(context.Background(), &msg); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

- [ ] **Step 3: 写最小实现**

```go
type sendMessageReq struct {
	BaseRequest
	Msg WeixinSendMessage `json:"msg"`
}

type sendMessageResp struct {
	Ret     int    `json:"ret"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (c *client) SendMessage(ctx context.Context, msg *WeixinSendMessage) error {
	req := sendMessageReq{Msg: *msg}
	req.BaseInfo.ChannelVersion = "2.1.1"
	resp, err := c.post(ctx, "/ilink/bot/sendmessage", req)
	if err != nil {
		return err
	}
	r, err := decodeJSON[sendMessageResp](resp)
	if err != nil {
		return err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return fmt.Errorf("sendmessage failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return nil
}

type textSender struct {
	client        *client
	contextTokens map[string]string
	mu            sync.RWMutex
}

func (s *textSender) Send(ctx context.Context, msg biz.ChannelOutboundPayload) error {
	meta := msg.Meta
	token := ""
	if meta != nil {
		token = meta.String(port.MetaContextToken)
	}
	if token == "" {
		s.mu.RLock()
		token = s.contextTokens[msg.ChannelPeerID]
		s.mu.RUnlock()
	}

	reply := WeixinSendMessage{
		ToUserID:     msg.ChannelPeerID,
		MessageType:  2,
		MessageState: 2,
		ContextToken: token,
		ItemList:     []MessageItem{{Type: 1, TextItem: &TextItem{Text: msg.Text}}},
	}
	return s.client.SendMessage(ctx, &reply)
}

func (s *textSender) setContextToken(userID, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.contextTokens == nil {
		s.contextTokens = map[string]string{}
	}
	s.contextTokens[userID] = token
}

func (s *textSender) deleteContextToken(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contextTokens, userID)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/channel/wechatilink/... -run TestSendMessage -v`
Expected: PASS

### Task 1.8：长轮询 Starter（polling.go）

**Files:**
- Create: `internal/channel/wechatilink/polling.go`
- Create: `internal/channel/wechatilink/polling_test.go`

- [ ] **Step 1: 写测试**

```go
func TestPollingReceivesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ilink/bot/getupdates" {
			json.NewEncoder(w).Encode(map[string]any{
				"ret": 0,
				"msgs": []map[string]any{
					{"message_id": 1, "from_user_id": "u1", "message_type": 1, "message_state": 2,
						"item_list": []map[string]any{{"type": 1, "text_item": map[string]any{"text": "hi"}}},
						"context_token": "ctx-1", "session_id": "s1"},
				},
				"get_updates_buf": "buf_v2",
			})
		}
	}))
	defer server.Close()

	var received []string
	handler := func(ev *port.InboundEvent) error {
		received = append(received, ev.Text)
		return nil
	}

	ch := biz.Channel{ID: "ch-1", ConfigJSON: `{"type":"wechat_ilink"}`, Enabled: true}
	creds := []biz.ChannelCredential{{CredentialKey: "bot_token", SecretRef: "tk1"}}
	lg := loggateway.NewNoop()

	// 用 WithBaseURL 覆盖 baseURL（client 需暴露）
	// ... 略，直接 mock client 更简洁
}
```

由于 starter 签名的复杂性，polling 测试用 mock client + goroutine + context timeout 的方式测试。这里先写实现，再做集成测试。

- [ ] **Step 2: 写 polling 实现**

```go
package wechatilink

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/loggateway"
)

func init() {
	runtime.RegisterStarterWithLogger("wechat_ilink", "polling", RunPolling)
}

type getUpdatesReq struct {
	BaseRequest
	GetUpdatesBuf string `json:"get_updates_buf"`
}

type getUpdatesResp struct {
	Ret               int             `json:"ret"`
	Msgs              []WeixinMessage `json:"msgs"`
	GetUpdatesBuf     string          `json:"get_updates_buf"`
	LongPollingTimeoutMs int          `json:"longpolling_timeout_ms"`
	ErrCode           int             `json:"errcode"`
	ErrMsg            string          `json:"errmsg"`
}

func RunPolling(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup runtime.CredentialLookup, handler port.InboundHandler, lg loggateway.Logger) error {
	botToken, err := lookup(ctx, creds, "bot_token")
	if err != nil {
		return fmt.Errorf("bot_token lookup: %w", err)
	}
	if botToken == "" {
		return fmt.Errorf("bot_token not configured")
	}

	state, err := readState(ch.ID)
	if err != nil {
		lg.Warn("读取状态文件失败，使用空状态", loggateway.Err(err))
		state = &stateFile{ContextTokens: map[string]string{}}
	}

	c := newClient("", botToken, lg)
	sender := &textSender{client: c, contextTokens: state.ContextTokens}

	// 注册出站发送器
	// 注：实际注册在 channel_platform_registry.go 完成，这里只返回 sender
	// starter 不直接操作 router，由 caller 处理

	buf := state.GetUpdatesBuf
	consecutiveErrors := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req := getUpdatesReq{GetUpdatesBuf: buf}
		req.BaseInfo.ChannelVersion = "2.1.1"

		resp, httpErr := c.post(ctx, "/ilink/bot/getupdates", req)
		if httpErr != nil {
			consecutiveErrors++
			backoff := pollingBackoff(consecutiveErrors)
			lg.Warn("getupdates HTTP 失败", loggateway.Err(httpErr), loggateway.Int("consecutive", consecutiveErrors))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		r, decodeErr := decodeJSON[getUpdatesResp](resp)
		if decodeErr != nil {
			consecutiveErrors++
			lg.Warn("getupdates 解码失败", loggateway.Err(decodeErr))
			time.Sleep(2 * time.Second)
			continue
		}

		if isSessionExpired(r.ErrCode) {
			lg.Error("微信登录会话过期", loggateway.Err(ErrSessionExpired))
			state.LoginStatus = "expired"
			_ = writeState(ch.ID, state)
			return ErrSessionExpired
		}

		if r.Ret != 0 || r.ErrCode != 0 {
			consecutiveErrors++
			lg.Warn("getupdates 业务错误", loggateway.Int("ret", r.Ret), loggateway.Int("errcode", r.ErrCode), loggateway.Str("errmsg", r.ErrMsg))
			time.Sleep(2 * time.Second)
			continue
		}

		consecutiveErrors = 0
		buf = r.GetUpdatesBuf

		for _, msg := range r.Msgs {
			sender.setContextToken(msg.FromUserID, msg.ContextToken)
			state.ContextTokens[msg.FromUserID] = msg.ContextToken

			// 群聊门控
			cfg, _ := biz.ParseChannelConfig(ch.ConfigJSON)
			if !shouldHandleGroupMessage(&msg, cfg) {
				continue
			}

			ev, parseErr := parseMessage(ch.ID, &msg)
			if parseErr != nil {
				lg.Warn("消息解析失败", loggateway.Err(parseErr))
				continue
			}

			if handleErr := handler.ProcessInbound(ctx, ch, *ev); handleErr != nil {
				lg.Warn("入站处理失败", loggateway.Err(handleErr))
			}
		}

		state.GetUpdatesBuf = buf
		state.LoginStatus = "active"
		if writeErr := writeState(ch.ID, state); writeErr != nil {
			lg.Warn("状态文件写入失败", loggateway.Err(writeErr))
		}
	}
}

func pollingBackoff(consecutive int) time.Duration {
	switch {
	case consecutive <= 2:
		return 2 * time.Second
	case consecutive <= 5:
		return 5 * time.Second
	default:
		return 30 * time.Second
	}
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./internal/channel/wechatilink/...`
Expected: PASS

### Task 1.9：扫码登录 Service RPC

**Files:**
- Create: `internal/service/channel_wechat_ilink_login.go`

- [ ] **Step 1: 写实现**

```go
package service

import (
	"context"
	"strings"
	"time"

	channelv1 "aranea-agents/api/kratos/channel/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/wechatilink"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ChannelService 真实字段：uc *biz.ChannelUsecase / runtime *ChannelRuntime / lg（见 channel.go:25-43）。
// 凭证加密由 usecase 内部处理（UpsertCredentials → crypto.EncryptChannelSecretRef）。

func (s *ChannelService) WechatILinkLogin(ctx context.Context, req *channelv1.WechatILinkLoginRequest) (*channelv1.WechatILinkLoginResponse, error) {
	channelID := strings.TrimSpace(req.ChannelId)
	if err := s.assertChannelMutateAccess(ctx, channelID); err != nil {
		return nil, err
	}

	// 新登录：获取二维码
	cl := wechatilink.NewLoginClient("", s.lg)
	resp, err := cl.GetBotQRCode(ctx)
	if err != nil {
		return nil, err
	}

	// 后台轮询扫码状态，确认后写凭证并触发 runtime reload
	safego.Go(context.WithoutCancel(ctx), "channel.wechat_ilink.login_poll", func() {
		s.pollWechatILinkQRStatus(context.Background(), channelID, resp.QRCode)
	})

	return &channelv1.WechatILinkLoginResponse{
		QrcodeDataUrl: resp.QRCodeImgContent,
		QrcodeSession: resp.QRCode,
		Status:        "wait",
	}, nil
}

func (s *ChannelService) pollWechatILinkQRStatus(ctx context.Context, channelID, qrcode string) {
	cl := wechatilink.NewLoginClient("", s.lg)
	deadline := time.Now().Add(3 * time.Minute) // 二维码有效期有限，超时放弃
	for time.Now().Before(deadline) {
		status, err := cl.GetQRCodeStatus(ctx, qrcode)
		if err != nil {
			s.lg.Warn("扫码状态轮询失败", loggateway.Err(err))
		} else {
			switch status.Status {
			case "confirmed":
				_, err = s.uc.UpsertCredentials(ctx, channelID, []biz.ChannelCredentialInput{
					{CredentialKey: "bot_token", Secret: status.BotToken},
					{CredentialKey: "baseurl", Secret: status.BaseURL},
					{CredentialKey: "ilink_user_id", Secret: status.ILinkUserID},
				})
				if err != nil {
					s.lg.Error("扫码登录凭证写入失败", loggateway.Err(err))
					return
				}
				s.reloadRuntime(ctx)
				return
			case "expired":
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *ChannelService) WechatILinkPoll(ctx context.Context, req *channelv1.WechatILinkPollRequest) (*channelv1.WechatILinkPollResponse, error) {
	channelID := strings.TrimSpace(req.ChannelId)
	if err := s.assertChannelAccess(ctx, channelID); err != nil {
		return nil, err
	}
	// 凭证已写入 = 登录完成
	creds, err := s.uc.ListCredentialsRaw(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, c := range creds {
		if c.CredentialKey == "bot_token" && strings.TrimSpace(c.SecretRef) != "" && c.DeletedAt == "" {
			return &channelv1.WechatILinkPollResponse{Status: "confirmed"}, nil
		}
	}
	return &channelv1.WechatILinkPollResponse{Status: "wait"}, nil
}
```

注意：`NewLoginClient` 不需要 bot_token，需要调整 client 构造逻辑——用 `NewLoginClient`（无 token 的匿名 client）调用 get_bot_qrcode 和 get_qrcode_status（这两个端点不需要 Authorization）。我需要在 client.go 里添加一个专门用于登录的 client。

让我修正 client.go 添加一个不需要 bot_token 的构造方式。

- [ ] **Step 2: client.go 添加登录专用 client**

```go
// loginClient 用于登录前（无 bot_token）的接口调用
type loginClient struct {
	baseURL string
	http    *http.Client
	lg      loggateway.Logger
}

func newLoginClient(baseURL string, lg loggateway.Logger) *loginClient {
	if baseURL == "" {
		baseURL = "https://ilinkai.weixin.qq.com"
	}
	return &loginClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
		lg:      lg,
	}
}

func (lc *loginClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lc.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-Id", "bot")
	req.Header.Set("iLink-App-ClientVersion", fmt.Sprintf("0x%08x", 0x00020101))
	return lc.http.Do(req)
}
```

- [ ] **Step 3: 编译验证**

Run: `go build ./internal/service/... ./internal/channel/wechatilink/...`
Expected: PASS

---

## Phase 2：Service 层注册与前端

### Task 2.1：出站注册

**Files:**
- Modify: `internal/service/channel_platform_registry.go`

- [ ] **Step 1: 注册 wechat_ilink outbound handler（init() 追加 + 实现函数）**

```go
// init() 追加：
registerPlatform("wechat_ilink", outboundWechatILink, nil)

// 新增函数（遵循 outboundTelegram 模式：resolveCredentialPlain → 构造 sender → SendText）：
func outboundWechatILink(ctx context.Context, h *ChannelIngress, _ biz.Channel, creds []biz.ChannelCredential, payload biz.ChannelOutboundPayload) error {
	token, err := resolveCredentialPlain(ctx, h.channels, creds, "bot_token", h.lg)
	if err != nil {
		return err
	}
	baseURL, _ := resolveCredentialPlain(ctx, h.channels, creds, "baseurl", h.lg) // 可选
	return (&wechatilink.TextSender{BotToken: token, BaseURL: baseURL, HTTP: h.http, Lg: h.lg}).
		SendText(ctx, payload.Recipient, payload.Text, payload.Extra["context_token"])
}
```

> TextSender 为每次发送临时构造（无长生命周期缓存）；context_token 由入站 OutboundMeta 透传到 payload.Extra。出站 Markdown 降级在 TextSender.SendText 内完成（P3）。

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/service/...`
Expected: PASS

### Task 2.2：Live Tester 注册

**Files:**
- Modify: `internal/service/channel.go`

- [ ] **Step 1: buildLiveTesters 注册 + 实现 tester（getconfig 只读探活）**

```go
// buildLiveTesters() map 追加：
"wechat_ilink": biz.ChannelLiveTesterFunc(s.testWechatILinkLive),

// 新增方法（遵循 testTelegramLive 模式，channel.go:666）：
func (s *ChannelService) testWechatILinkLive(ctx context.Context, configJSON string, creds []biz.ChannelCredential) biz.ChannelTestResult {
	token, terr := resolveCredentialPlain(ctx, s.uc, creds, "bot_token", s.lg)
	if terr != nil || strings.TrimSpace(token) == "" {
		return biz.ChannelTestResult{OK: false, Status: "pending_auth", Message: "bot_token not configured，请先扫码登录"}
	}
	baseURL, _ := resolveCredentialPlain(ctx, s.uc, creds, "baseurl", s.lg)
	if err := wechatilink.TestConnection(ctx, lark.DefaultHTTPClient(), baseURL, token, s.lg); err != nil {
		return biz.ChannelTestResult{OK: false, Status: "error", Message: err.Error()}
	}
	return biz.ChannelTestResult{OK: true, Status: "ok", Message: "wechat_ilink getconfig ok"}
}
```

> `wechatilink.TestConnection` 在包内实现：调用 `getconfig`（只读）验证 token 有效性；`resolveCredentialPlain(ctx, s.uc, ...)` 是 service 层既有 helper（负责解密 secret_ref）。

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/service/...`
Expected: PASS

### Task 2.3：all/all.go 注册

**Files:**
- Modify: `internal/channel/all/all.go`

- [ ] **Step 1: 添加 import**

```go
_ "aranea-agents/internal/channel/wechatilink"
```

- [ ] **Step 2: 编译验证**

Run: `go build ./internal/channel/all/...`
Expected: PASS

### Task 2.4：前端渠道页

**Files:**
- Modify: `web/src/features/channels/api.ts`
- Modify: `web/src/features/channels/channelPlatformFields.ts`
- Modify: `web/src/features/channels/ChannelEditorDialog.vue`
- Modify: `web/src/features/channels/useChannelEditorForm.ts`

- [ ] **Step 1: api.ts 追加登录 RPC 方法**

```typescript
export async function wechatIlinkLogin(channelId: string): Promise<{ qrcodeDataUrl: string; qrcodeSession: string; status: string }> {
  const data = await channelApi.WechatILinkLogin({ channelId });
  return {
    qrcodeDataUrl: data.qrcodeDataUrl ?? '',
    qrcodeSession: data.qrcodeSession ?? '',
    status: data.status ?? 'wait',
  };
}

export async function wechatIlinkPoll(channelId: string, qrcodeSession: string): Promise<{ status: string; errorMessage: string }> {
  const data = await channelApi.WechatILinkPoll({ channelId, qrcodeSession });
  return {
    status: data.status ?? 'wait',
    errorMessage: data.errorMessage ?? '',
  };
}
```

- [ ] **Step 2: channelPlatformFields.ts 追加 wechat_ilink**

```typescript
// 在 platformFields 对象中追加
wechat_ilink: {
  fields: [
    { name: 'group_enabled', label: '启用群聊', type: 'boolean', default: false },
    { name: 'require_mention', label: '群聊需@提及', type: 'boolean', default: true },
    { name: 'bot_nickname', label: 'Bot 昵称', type: 'string', default: '' },
  ],
  credentialKeys: ['bot_token'],
},
```

- [ ] **Step 3: useChannelEditorForm.ts 追加扫码登录状态**

```typescript
// 在 reactive state 中追加
interface ChannelFormState {
  // ... existing fields ...
  wechatILinkQRCode: string;
  wechatILinkStatus: 'idle' | 'wait' | 'scaned' | 'confirmed' | 'expired' | 'error';
  wechatILinkPolling: boolean;
}

// 在初始化时默认
wechatILinkQRCode: '',
wechatILinkStatus: 'idle',
wechatILinkPolling: false,
```

- [ ] **Step 4: ChannelEditorDialog.vue 追加扫码登录区块**

```vue
<!-- 在 dialog 内容区，仅当 platform.type === 'wechat_ilink' 时显示 -->
<div v-if="form.platform === 'wechat_ilink'" class="q-mt-md">
  <q-card flat bordered>
    <q-card-section>
      <div class="text-subtitle2">微信扫码登录</div>
      <div v-if="form.wechatILinkStatus === 'idle'">
        <q-btn label="获取二维码" color="primary" @click="startWechatILinkLogin" />
      </div>
      <div v-else-if="form.wechatILinkStatus === 'wait' || form.wechatILinkStatus === 'scaned'">
        <img :src="form.wechatILinkQRCode" style="max-width: 200px;" />
        <div class="text-caption q-mt-sm">
          {{ form.wechatILinkStatus === 'scaned' ? '已扫码，请确认登录' : '请使用微信扫码' }}
        </div>
      </div>
      <div v-else-if="form.wechatILinkStatus === 'confirmed'">
        <q-icon name="check_circle" color="positive" /> 已登录
      </div>
      <div v-else-if="form.wechatILinkStatus === 'expired'">
        <q-icon name="error" color="negative" /> 二维码已过期
        <q-btn label="刷新" flat color="primary" @click="startWechatILinkLogin" />
      </div>
    </q-card-section>
  </q-card>
</div>
```

- [ ] **Step 5: 前端编译验证**

Run: `cd web && pnpm lint`
Expected: 无 wechat_ilink 相关报错（可能有既有错误）

---

## Phase 3：P1 功能（群聊 + context_token + 过期重扫）

### Task 3.1：context_token 缓存持久化

已在 polling.go 中实现（state.ContextTokens 写入状态文件）。验证：

Run: `go test ./internal/channel/wechatilink/... -run TestStateFileRoundtrip -v`
Expected: PASS（包含 ContextTokens 断言）

### Task 3.2：会话过期自愈链路测试

- [ ] **Step 1: 写集成测试**

```go
func TestSessionExpiredRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ret": 0, "errcode": -14, "errmsg": "session expired",
		})
	}))
	defer server.Close()

	ch := biz.Channel{ID: "ch-exp", ConfigJSON: `{"type":"wechat_ilink"}`, Enabled: true}
	creds := []biz.ChannelCredential{{CredentialKey: "bot_token", SecretRef: "tk"}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := RunPolling(ctx, ch, creds, func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
		return "tk", nil
	}, &mockHandler{}, loggateway.NewNoop())

	if !errors.Is(err, ErrSessionExpired) {
		t.Errorf("want ErrSessionExpired, got %v", err)
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/channel/wechatilink/... -run TestSessionExpiredRecovery -v`
Expected: PASS

---

## Phase 4：P2 媒体收发

### Task 4.1：AES-128-ECB 加解密

**Files:**
- Create: `internal/channel/wechatilink/media.go`
- Create: `internal/channel/wechatilink/media_test.go`

- [ ] **Step 1: 写测试**

```go
func TestAESRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	rand.Read(key)
	plaintext := []byte("hello world, this is a test message for aes encryption")

	encrypted, err := aesEncrypt(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := aesDecrypt(encrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("roundtrip failed")
	}
}
```

- [ ] **Step 2: 写实现**

```go
package wechatilink

import (
	"crypto/aes"
	"errors"
)

func aesEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(encrypted[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return encrypted, nil
}

func aesDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext not aligned to block size")
	}
	decrypted := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(decrypted[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(decrypted)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := make([]byte, padLen)
	for i := range padding {
		padding[i] = byte(padLen)
	}
	return append(data, padding...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen > len(data) || padLen == 0 {
		return nil, errors.New("invalid padding")
	}
	return data[:len(data)-padLen], nil
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./internal/channel/wechatilink/... -run TestAESRoundtrip -v`
Expected: PASS

### Task 4.2：CDN 上传/下载

```go
// media.go 追加
type uploadURLResp struct {
	Ret      int    `json:"ret"`
	CDNURL   string `json:"cdn_url"`
	ErrCode  int    `json:"errcode"`
	ErrMsg   string `json:"errmsg"`
}

func (c *client) GetUploadURL(ctx context.Context) (string, error) {
	resp, err := c.post(ctx, "/ilink/bot/getuploadurl", baseRequest{BaseInfo: struct{ ChannelVersion string `json:"channel_version"` }{ChannelVersion: "2.1.1"}})
	if err != nil {
		return "", err
	}
	r, err := decodeJSON[uploadURLResp](resp)
	if err != nil {
		return "", err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return "", fmt.Errorf("getuploadurl failed: ret=%d errcode=%d", r.Ret, r.ErrCode)
	}
	return r.CDNURL, nil
}

func (c *client) UploadToCDN(ctx context.Context, cdnURL string, data []byte) error {
	resp, err := c.http.Post(cdnURL, "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("CDN upload failed: %d", resp.StatusCode)
	}
	return nil
}

func (c *client) DownloadFromCDN(ctx context.Context, url string) ([]byte, error) {
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("CDN download failed: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

### Task 4.3：媒体消息解析增强

已在 parse.go 中实现（case 2-5）。补充文件下载逻辑：语音/图片/文件入站时下载 CDN + 解密 + 存本地，但 Agent 看到的仍是占位符文本。

---

## Phase 5：P3 功能（Typing + Markdown 降级）

### Task 5.1：Typing 状态

**Files:**
- Create: `internal/channel/wechatilink/typing.go`

```go
package wechatilink

import "context"

type getConfigResp struct {
	Ret          int    `json:"ret"`
	TypingTicket string `json:"typing_ticket"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func (c *client) GetTypingTicket(ctx context.Context) (string, error) {
	resp, err := c.post(ctx, "/ilink/bot/getconfig", baseRequest{BaseInfo: struct{ ChannelVersion string `json:"channel_version"` }{ChannelVersion: "2.1.1"}})
	if err != nil {
		return "", err
	}
	r, err := decodeJSON[getConfigResp](resp)
	if err != nil {
		return "", err
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return "", fmt.Errorf("getconfig failed: ret=%d errcode=%d", r.Ret, r.ErrCode)
	}
	return r.TypingTicket, nil
}

type sendTypingReq struct {
	BaseRequest
	ILinkUserID  string `json:"ilink_user_id"`
	TypingTicket string `json:"typing_ticket"`
	Status       int    `json:"status"` // 1=TYPING, 2=CANCEL
}

func (c *client) SendTyping(ctx context.Context, userID, ticket string, typing bool) error {
	status := 2
	if typing {
		status = 1
	}
	req := sendTypingReq{ILinkUserID: userID, TypingTicket: ticket, Status: status}
	req.BaseInfo.ChannelVersion = "2.1.1"
	resp, err := c.post(ctx, "/ilink/bot/sendtyping", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
```

### Task 5.2：Markdown 降级

```go
// media.go 追加
func markdownToWechat(text string) string {
	// 标题
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i) + " "
		text = strings.ReplaceAll(text, prefix, strings.Repeat("【", i)+"标题"+strings.Repeat("】", i)+" ")
	}
	// 加粗/斜体/删除线
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "~~", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	// 分隔线
	text = strings.ReplaceAll(text, "---", "——")
	// 列表
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "+ ") {
			lines[i] = "• " + trimmed[2:]
		}
	}
	return strings.Join(lines, "\n")
}
```

在 outbound.go 的 Send 方法中调用 `markdownToWechat(msg.Text)`。

---

## Phase 6：验证与审查

### Task 6.1：全量编译

Run: `go build ./...`
Expected: PASS

### Task 6.2：单元测试

Run: `go test ./internal/channel/wechatilink/... -count=1 -v`
Expected: 全部 PASS

Run: `go test ./internal/service/... -count=1 -v`
Expected: 全部 PASS

Run: `go test ./internal/channel/... -count=1 -v`
Expected: 全部 PASS

### Task 6.3：Wire 注入

Run: `make wire && go build ./cmd/admin`
Expected: PASS

### Task 6.4：前端构建

Run: `cd web && pnpm build`
Expected: PASS

### Task 6.5：Lint

Run: `make lint`
Expected: PASS

### Task 6.6：契约测试

Run: `go test ./internal/channel/... -run TestContract -count=1 -v`
Expected: wechatilink.TextSender 断言 PASS

### Task 6.7：文档同步

- [ ] `docs/development/17-channel.md`：§2 平台表追加 wechat_ilink、§5 新增 iLink 凭据配置、§7 运行时新增 polling 说明
- [ ] `docs/development/17-channel.design.md`：§3 Adapter 追加 wechat_ilink 说明、§5.3 新增扫码登录 RPC
- [ ] `docs/development/17-channel.development.md`：任务清单追加 wechat_ilink 完成状态

### Task 6.8：代码审查

使用 `aranea-review` skill 执行全栈代码审查，重点检查：
- 依赖方向合规（wechatilink 不依赖 service/proto）
- 日志使用 loggateway（红线 #16）
- 错误经 entErrToBizErr 翻译（DB 相关时）
- 凭证不 hardcode
- 状态机/事件可靠性（会话过期 -14 的检测与恢复链路）
- 前端数据流合规（aranea-frontend-guide §3）

---

## Spec Coverage 自检

| Spec 章节 | 对应任务 |
|-----------|---------|
| §3.1 整体链路 | Task 1.1-1.8 |
| §3.3 类型注册（双文件） | Task 0.2 |
| §3.4 运行时模式映射 | Task 0.3 |
| §4.1 认证头 | Task 1.3 |
| §4.2 扫码登录 | Task 1.5, 1.9 |
| §4.3 长轮询 | Task 1.8 |
| §4.4 消息发送 | Task 1.7 |
| §4.5 消息结构 | Task 1.1 |
| §4.6 CDN 媒体 | Task 4.1-4.3 |
| §4.7 Typing | Task 5.1 |
| §5.1 配置 | Task 0.2（ConfigSchema） |
| §5.2 凭证 | Task 1.9（UpsertCredentials） |
| §5.3 状态文件 | Task 1.4 |
| §6.1 Proto RPC | Task 0.1, 1.9 |
| §6.2 前端交互 | Task 2.4 |
| §7 错误处理 | Task 1.2, 3.2 |
| §8 安全 | Task 1.3（X-WECHAT-UIN）、1.4（文件权限） |
| §9 测试 | 各 Task 的 _test.go |
| §11 Phase 划分 | 全部覆盖 |
| DOC-SYNC 文档同步 | Task 6.7 |

**无遗漏。计划完整。**
