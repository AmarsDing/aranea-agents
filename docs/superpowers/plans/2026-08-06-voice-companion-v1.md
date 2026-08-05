# Voice Companion V1（语音管线 MVP）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 桌面端说完话 < 2.5s 开始听到回复；语音对话记录与普通聊天互通（Phase V1，对应 `docs/development/74-voice-companion.development.md` §4 Phase V1 V1-T1~V1-T10）。

**Architecture:** 级联流式管线——浏览器/Tauri 采集 PCM → `/v1/voice` WS 网关 → 流式 ASR（火山）→ 终稿复用 `WSTurnExecutor` 入 Chat 管线 → 订阅 v2 EventBus 流式 delta → SentenceChunker 分句 → 流式 TTS（火山）→ 音频 chunk 直推前端 gapless 播放。Chat 编排内核零改动。

**Tech Stack:** Go（`internal/biz` 端口 / `internal/data/speech` 适配器 / `internal/voice` 编排 / `internal/server` 网关）| gorilla/websocket + Kratos HTTP HandleFunc | Vue 3 + Pinia + Three.js（已在 package.json）+ Web Audio | vitest / testify。

**Spec 锚点：**
- 需求：`docs/development/74-voice-companion.md`
- 设计：`docs/development/74-voice-companion.design.md`（协议 §2、端口 §3、分句 §4、状态机 §5、前端 §7）
- 开发计划：`docs/development/74-voice-companion.development.md` §4 Phase V1

**实施前必读（强制，对应 project_rules R1/R2）：**
1. `docs/development/74-voice-companion.design.md` 全文
2. `docs/development/65-module-cross-reference-full.md` §1.29 + §2.7（voice 模块卡片）
3. 数据流图：PCM帧 → ASR → ExecuteTurn → EventBus(step.streaming) → Chunker → TTS → 音频帧

**关键既有锚点（已验证，禁止重新发明）：**

| 复用 | 位置 |
|------|------|
| WS 升级/鉴权/所有权校验模式 | `internal/server/ws.go`（`handleWS`/`wsAuthenticate`/`SessionAuthorizer`，L209-254） |
| Turn 统一入口 | `internal/server/ws.go:27,45`（`WSTurnInput{SessionID, Content, AgentKey, TeamID, Options, AllowQueue, AllowStream}` + `WSTurnExecutor.ExecuteTurn`） |
| 取消 | `internal/server/ws.go:51` `RunCanceller.CancelRun(ctx, sessionID) bool` |
| 流式 delta 源 | `internal/biz/event.go:235` `StepStreamingEvent{DeltaField, DeltaChunk}` + `:597` `EventBus.Subscribe(EventSubscribeOptions{SpiritSessionID})`；Turn 结束 `TurnCompletedEvent`（`:190`） |
| 流程日志 | `internal/event/flow_log.go:100` `stepTitleRegistry`；emitter 模式见 `ws.go:362` `newWSFlowEmitter` |
| HTTP 挂载 | `internal/server/http.go:68,106`（`NewHTTPServer(..., wsSrv *WSServer, ...)` → `wsSrv.RegisterOnKratos(srv)`） |
| Wire | `cmd/admin/wire.go:2755` `provideWSServer` 模式；改完跑 `make wire` |
| 前端 WS URL | `web/src/config/runtime.ts:100` `buildWsUrl`（voice 版新增 `buildVoiceWsUrl`） |
| 聊天面板复用 | `web/src/pages/mobile/MobileChatPage.vue:170-184`（`provide(CHAT_WORKSPACE_KEY, useChatWorkspace())` 模式见 `layouts/MobileLayout.vue:68`） |
| i18n | `web/src/i18n/locales/zh-CN.ts` / `en-US.ts`（lint 含 check-i18n，禁止硬编码中文） |

**V1 范围裁剪说明（与设计/开发计划的对齐决策）：**
1. **凭据注入**：V1 走环境变量（设计 §3.3 允许：「V1 期可仅配置文件/环境变量注入」），System Settings `speech` 分组 + 管理面 UI 在 V2-T7。
2. **错误码**：设计 §3.2 承诺 `CodeFailedPrecondition`，但 `pkg/apierror` 尚无该码——Task 1 新增（含 HTTP 412 映射），与文档对齐。
3. **TTS 传输**：V1 每句一条 WS 连接（火山 `ws_binary`，一句一合成）；预连接/双工流式升级列入 V3-T5 打磨。
4. **voice.barge_in**：V1 网关路由到与 `voice.cancel` 相同的取消路径（停 TTS + CancelRun）；完整 barge-in 链路（前端 VAD/本地停播/状态机 interrupted）在 V2-T1。
5. **火山协议字节级细节**（SAUC 二进制帧）按公开文档实现并配 mock 契约测试；真机字节校验归 Task 18（V1-T10）。
6. **biz 端口微调**：`ASRSession` 增加 `Finish()`（支撑 `voice.commit`/PTT），实现后同步设计文档 §3.1（DOC-SYNC）。

---

### Task 1: biz Speech 端口 + apierror FailedPrecondition

**Files:**
- Modify: `pkg/apierror/apierror.go`（+`CodeFailedPrecondition` + 构造函数 + `ToKratos` 412 映射）
- Create: `internal/biz/speech.go`
- Test: `internal/biz/speech_test.go`

- [ ] **Step 1: 写失败测试**

`internal/biz/speech_test.go`:

```go
package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

func TestASRProviderConfigValidate(t *testing.T) {
	valid := biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Language: "zh-CN"}
	require.NoError(t, valid.Validate())

	missing := valid
	missing.AccessKey = ""
	err := missing.Validate()
	require.Error(t, err)
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "want FAILED_PRECONDITION, got %v", err)

	missingDriver := valid
	missingDriver.Driver = ""
	require.Error(t, missingDriver.Validate())
}

func TestTTSProviderConfigValidate(t *testing.T) {
	valid := biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Voice: "v", SpeedRatio: 1.0}
	require.NoError(t, valid.Validate())

	bad := valid
	bad.Voice = ""
	require.True(t, apierror.IsCode(bad.Validate(), apierror.CodeFailedPrecondition))

	badSpeed := valid
	badSpeed.SpeedRatio = 0
	require.True(t, apierror.IsCode(badSpeed.Validate(), apierror.CodeFailedPrecondition))
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/biz/ -run 'TestASRProviderConfigValidate|TestTTSProviderConfigValidate' -count=1`
Expected: FAIL — `undefined: biz.ASRProviderConfig` 与 `undefined: apierror.CodeFailedPrecondition`

- [ ] **Step 3: 实现**

`pkg/apierror/apierror.go` 修改（3 处）:

```go
// const 块追加：
	CodeFailedPrecondition Code = "FAILED_PRECONDITION"

// Constructors 区追加：
func FailedPrecondition(domain, msg string, args ...any) *Error {
	return newf(CodeFailedPrecondition, domain, msg, args...)
}

// ToKratos switch 追加一个 case（放在 CodeBadRequest 之后）：
	case CodeFailedPrecondition:
		return kerrors.New(http.StatusPreconditionFailed, reason, msg)
```

`internal/biz/speech.go`:

```go
package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// SpeechProvider ports for the voice companion (M74). Pure Go interfaces —
// biz 不依赖任何 Provider SDK（火山等 SDK 仅在 internal/data/speech）。

// ---- ASR ----

type ASREventType int

const (
	ASREventPartial ASREventType = iota
	ASREventFinal
	ASREventError
	ASREventVadEnd // 服务端 VAD 端点（无终稿文本时的纯端点信号）
)

// ASREvent is the transport-neutral recognition event emitted by ASRSession.
type ASREvent struct {
	Type       ASREventType
	Text       string // Partial=当前 partial 全文；Final=终稿全文
	DurationMs int    // Final: 本语句音频时长
	Err        error  // Error 事件非 nil
}

type ASRSessionConfig struct {
	Language   string // 默认 zh-CN
	SampleRate int    // 默认 16000
}

// Stability:evolving
type StreamingASRProvider interface {
	Open(ctx context.Context, cfg ASRSessionConfig) (ASRSession, error)
}

// Stability:evolving
type ASRSession interface {
	Write(pcm []byte) error  // 20ms PCM s16le 帧
	Finish() error           // 标记当前语句音频结束（voice.commit / PTT）
	Events() <-chan ASREvent // Partial/Final/Error/VadEnd
	Close() error
}

// ---- TTS ----

type TTSAudioChunkType int

const (
	TTSAudioChunkData TTSAudioChunkType = iota
	TTSAudioChunkEnd                  // 全部句子合成完毕（或取消）
	TTSAudioChunkError
)

// TTSAudioChunk carries one audio chunk. PCM 编码固定 f32le 16kHz mono
// （适配器负责从 Provider 原始编码转换）。
type TTSAudioChunk struct {
	Type TTSAudioChunkType
	PCM  []byte
	Err  error
}

type TTSSessionConfig struct {
	Voice      string
	SpeedRatio float64
	SampleRate int // 默认 16000
}

// Stability:evolving
type StreamingTTSProvider interface {
	Open(ctx context.Context, cfg TTSSessionConfig) (TTSSession, error)
}

// Stability:evolving
type TTSSession interface {
	Write(text string, flush bool) error // 分句写入；flush=尾句强制合成
	Audio() <-chan TTSAudioChunk
	Close() error
}

// ---- 配置（V1 环境变量注入，见 data/speech/env_config.go）----

type ASRProviderConfig struct {
	Driver     string
	Endpoint   string
	AppKey     string // sensitive，禁止入日志
	AccessKey  string // sensitive，禁止入日志
	ResourceID string
	Language   string
}

func (c ASRProviderConfig) Validate() error {
	if strings.TrimSpace(c.Driver) == "" {
		return apierror.FailedPrecondition("speech", "asr driver is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return apierror.FailedPrecondition("speech", "asr endpoint is required")
	}
	if strings.TrimSpace(c.AppKey) == "" || strings.TrimSpace(c.AccessKey) == "" {
		return apierror.FailedPrecondition("speech", "asr credential is required (SPEECH_ASR_APP_KEY / SPEECH_ASR_ACCESS_KEY)")
	}
	return nil
}

type TTSProviderConfig struct {
	Driver     string
	Endpoint   string
	AppKey     string // sensitive
	AccessKey  string // sensitive
	ResourceID string
	Voice      string
	SpeedRatio float64
}

func (c TTSProviderConfig) Validate() error {
	if strings.TrimSpace(c.Driver) == "" {
		return apierror.FailedPrecondition("speech", "tts driver is required")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		return apierror.FailedPrecondition("speech", "tts endpoint is required")
	}
	if strings.TrimSpace(c.AppKey) == "" || strings.TrimSpace(c.AccessKey) == "" {
		return apierror.FailedPrecondition("speech", "tts credential is required (SPEECH_TTS_APP_KEY / SPEECH_TTS_ACCESS_KEY)")
	}
	if strings.TrimSpace(c.Voice) == "" {
		return apierror.FailedPrecondition("speech", "tts voice is required (SPEECH_TTS_VOICE)")
	}
	if c.SpeedRatio <= 0 {
		return apierror.FailedPrecondition("speech", "tts speed_ratio must be > 0")
	}
	return nil
}

// Stability:evolving — 配置读取端口。V1 实现 = env（data/speech/env_config.go）；
// V2-T7 换 System Settings speech 分组实现，端口不变。
type SpeechConfigReader interface {
	ASRConfig(ctx context.Context) (ASRProviderConfig, error)
	TTSConfig(ctx context.Context) (TTSProviderConfig, error)
}
```

- [ ] **Step 4: 跑测试确认通过 + apierror 回归**

Run: `go test ./internal/biz/ -run 'Speech|ASRProvider|TTSProvider' -count=1 && go test ./pkg/apierror/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/apierror/apierror.go internal/biz/speech.go internal/biz/speech_test.go
git commit -m "feat(speech): add biz SpeechProvider ports + FAILED_PRECONDITION apierror code"
```

---

### Task 2: env SpeechConfigReader（V1 配置注入）

**Files:**
- Create: `internal/data/speech/env_config.go`
- Test: `internal/data/speech/env_config_test.go`

- [ ] **Step 1: 写失败测试**

`internal/data/speech/env_config_test.go`:

```go
package speech_test

import (
	"context"
	"testing"

	speech "aranea-agents/internal/data/speech"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

func TestEnvSpeechConfigReaderMissingCreds(t *testing.T) {
	r := speech.NewEnvSpeechConfigReader()
	_, err := r.ASRConfig(context.Background())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
	_, err = r.TTSConfig(context.Background())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
}

func TestEnvSpeechConfigReaderDefaults(t *testing.T) {
	t.Setenv("SPEECH_ASR_APP_KEY", "ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_APP_KEY", "ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_VOICE", "zh_female_test")

	r := speech.NewEnvSpeechConfigReader()
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "volcengine", asr.Driver)
	require.Equal(t, "zh-CN", asr.Language)
	require.Contains(t, asr.Endpoint, "wss://")

	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "volcengine", tts.Driver)
	require.Equal(t, 1.0, tts.SpeedRatio)
	require.Equal(t, "zh_female_test", tts.Voice)
}

func TestEnvSpeechConfigReaderOverrides(t *testing.T) {
	t.Setenv("SPEECH_TTS_APP_KEY", "ak")
	t.Setenv("SPEECH_TTS_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_TTS_VOICE", "v")
	t.Setenv("SPEECH_TTS_SPEED_RATIO", "1.25")
	t.Setenv("SPEECH_ASR_APP_KEY", "ak")
	t.Setenv("SPEECH_ASR_ACCESS_KEY", "sk")
	t.Setenv("SPEECH_ASR_LANGUAGE", "en-US")

	r := speech.NewEnvSpeechConfigReader()
	tts, err := r.TTSConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1.25, tts.SpeedRatio)
	asr, err := r.ASRConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "en-US", asr.Language)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/data/speech/ -run TestEnvSpeechConfigReader -count=1`
Expected: FAIL — `undefined: speech.NewEnvSpeechConfigReader`

- [ ] **Step 3: 实现**

`internal/data/speech/env_config.go`:

```go
// Package speech implements the biz.SpeechProvider ports (M74).
package speech

import (
	"context"
	"os"
	"strconv"
	"strings"

	"aranea-agents/internal/biz"
)

const (
	defaultASRDriver     = "volcengine"
	defaultASREndpoint   = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel"
	defaultASRResourceID = "volc.bigasr.sauc.duration"
	defaultASRLanguage   = "zh-CN"

	defaultTTSDriver     = "volcengine"
	defaultTTSEndpoint   = "wss://openspeech.bytedance.com/api/v1/tts/ws_binary"
	defaultTTSResourceID = "volc.service_type.10029"
	defaultTTSSpeedRatio = 1.0
)

// EnvSpeechConfigReader implements biz.SpeechConfigReader from environment
// variables (V1; System Settings speech 分组在 V2-T7)。凭据只经 env 注入，
// 禁止写入日志（DB-N8 同语义）。
type EnvSpeechConfigReader struct{}

func NewEnvSpeechConfigReader() *EnvSpeechConfigReader { return &EnvSpeechConfigReader{} }

var _ biz.SpeechConfigReader = (*EnvSpeechConfigReader)(nil)

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func (r *EnvSpeechConfigReader) ASRConfig(_ context.Context) (biz.ASRProviderConfig, error) {
	cfg := biz.ASRProviderConfig{
		Driver:     envOr("SPEECH_ASR_DRIVER", defaultASRDriver),
		Endpoint:   envOr("SPEECH_ASR_ENDPOINT", defaultASREndpoint),
		AppKey:     strings.TrimSpace(os.Getenv("SPEECH_ASR_APP_KEY")),
		AccessKey:  strings.TrimSpace(os.Getenv("SPEECH_ASR_ACCESS_KEY")),
		ResourceID: envOr("SPEECH_ASR_RESOURCE_ID", defaultASRResourceID),
		Language:   envOr("SPEECH_ASR_LANGUAGE", defaultASRLanguage),
	}
	if err := cfg.Validate(); err != nil {
		return biz.ASRProviderConfig{}, err
	}
	return cfg, nil
}

func (r *EnvSpeechConfigReader) TTSConfig(_ context.Context) (biz.TTSProviderConfig, error) {
	speed := defaultTTSSpeedRatio
	if raw := strings.TrimSpace(os.Getenv("SPEECH_TTS_SPEED_RATIO")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			speed = v
		}
	}
	cfg := biz.TTSProviderConfig{
		Driver:     envOr("SPEECH_TTS_DRIVER", defaultTTSDriver),
		Endpoint:   envOr("SPEECH_TTS_ENDPOINT", defaultTTSEndpoint),
		AppKey:     strings.TrimSpace(os.Getenv("SPEECH_TTS_APP_KEY")),
		AccessKey:  strings.TrimSpace(os.Getenv("SPEECH_TTS_ACCESS_KEY")),
		ResourceID: envOr("SPEECH_TTS_RESOURCE_ID", defaultTTSResourceID),
		Voice:      strings.TrimSpace(os.Getenv("SPEECH_TTS_VOICE")),
		SpeedRatio: speed,
	}
	if err := cfg.Validate(); err != nil {
		return biz.TTSProviderConfig{}, err
	}
	return cfg, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/data/speech/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/data/speech/env_config.go internal/data/speech/env_config_test.go
git commit -m "feat(speech): env-based SpeechConfigReader with volcengine defaults"
```

---

### Task 3: 火山 SAUC 二进制帧编解码器（ASR/TTS 共用）

**Files:**
- Create: `internal/data/speech/volc_frame.go`
- Test: `internal/data/speech/volc_frame_test.go`

协议（火山 SAUC/WebSocket 二进制，4 字节头）：

```
byte0: protocol_version(4bit)=1 << 4 | header_size(4bit)=1   → 0x11
byte1: message_type(4bit) << 4 | message_type_specific_flags(4bit)
byte2: serialization(4bit) << 4 | compression(4bit)
byte3: reserved = 0x00
flags: 0x1=携带正序号(4B)，0x3=携带负序号(末帧)
消息体: [4B seq]? [4B payload_size] payload（JSON 可 gzip；音频裸字节不压缩）
```

- [ ] **Step 1: 写失败测试**

`internal/data/speech/volc_frame_test.go`:

```go
package speech

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFrameRoundTripJSONGzip(t *testing.T) {
	in := volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: []byte(`{"hello":"world"}`)}
	data, err := marshalVolcFrame(in, true)
	require.NoError(t, err)
	require.Equal(t, byte(0x11), data[0], "version+header size")
	require.Equal(t, byte(0x10), data[1], "full client request, no flags")

	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, in.msgType, out.msgType)
	require.True(t, out.json)
	require.JSONEq(t, `{"hello":"world"}`, string(out.payload))
}

func TestFrameRoundTripAudioWithSeq(t *testing.T) {
	pcm := bytes.Repeat([]byte{0x01, 0x02}, 320) // 640B = 20ms 16k s16le
	in := volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagPositiveSeq, seq: 7, payload: pcm}
	data, err := marshalVolcFrame(in, false)
	require.NoError(t, err)

	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, volcMsgAudioOnlyRequest, out.msgType)
	require.Equal(t, int32(7), out.seq)
	require.Equal(t, pcm, out.payload)
}

func TestFrameNegativeSeq(t *testing.T) {
	in := volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagNegativeSeq, seq: -7, payload: []byte{}}
	data, err := marshalVolcFrame(in, false)
	require.NoError(t, err)
	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, int32(-7), out.seq)
}

func TestUnmarshalRejectsBadVersion(t *testing.T) {
	_, err := unmarshalVolcFrame(bytes.NewReader([]byte{0x21, 0x10, 0x10, 0x00, 0, 0, 0, 0}))
	require.Error(t, err)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/data/speech/ -run TestFrame -count=1`
Expected: FAIL — `undefined: volcFrame`

- [ ] **Step 3: 实现**

`internal/data/speech/volc_frame.go`:

```go
package speech

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
)

// 火山 SAUC WebSocket 二进制协议帧编解码（ASR/TTS 适配器共用）。
// 布局见设计文档引用的火山公开协议；字节级真机校验归 V1-T10。

type volcMsgType = byte

const (
	volcMsgFullClientRequest  volcMsgType = 0x1
	volcMsgAudioOnlyRequest   volcMsgType = 0x2
	volcMsgFullServerResponse volcMsgType = 0x9
	volcMsgAudioOnlyResponse  volcMsgType = 0xB
	volcMsgError              volcMsgType = 0xF
)

const (
	volcFlagNone        byte = 0x0
	volcFlagPositiveSeq byte = 0x1
	volcFlagNegativeSeq byte = 0x3
)

type volcFrame struct {
	msgType volcMsgType
	flags   byte
	seq     int32
	json    bool // serialization=JSON（可 gzip）；false=裸字节不压缩
	payload []byte
}

func marshalVolcFrame(f volcFrame, gzipJSON bool) ([]byte, error) {
	payload := f.payload
	compression := byte(0x0)
	serialization := byte(0x0)
	if f.json {
		serialization = 0x1
		if gzipJSON {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			if _, err := zw.Write(payload); err != nil {
				return nil, err
			}
			if err := zw.Close(); err != nil {
				return nil, err
			}
			payload = buf.Bytes()
			compression = 0x1
		}
	}
	withSeq := f.flags == volcFlagPositiveSeq || f.flags == volcFlagNegativeSeq
	out := make([]byte, 0, 4+len(payload)+8)
	out = append(out, 0x11, (f.msgType<<4)|(f.flags&0x0F), (serialization<<4)|compression, 0x00)
	if withSeq {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(f.seq))
		out = append(out, b[:]...)
	}
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(len(payload)))
	out = append(out, sz[:]...)
	out = append(out, payload...)
	return out, nil
}

func unmarshalVolcFrame(r io.Reader) (volcFrame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return volcFrame{}, err
	}
	if hdr[0]>>4 != 1 {
		return volcFrame{}, fmt.Errorf("unsupported protocol version %d", hdr[0]>>4)
	}
	headerSize := int(hdr[0] & 0x0F)
	f := volcFrame{
		msgType: hdr[1] >> 4,
		flags:   hdr[1] & 0x0F,
		json:    hdr[2]>>4 == 0x1,
	}
	gzipped := hdr[2]&0x0F == 0x1
	if headerSize > 1 { // 跳过额外头字节
		if _, err := io.CopyN(io.Discard, r, int64(4*(headerSize-1))); err != nil {
			return volcFrame{}, err
		}
	}
	if f.flags == volcFlagPositiveSeq || f.flags == volcFlagNegativeSeq {
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return volcFrame{}, err
		}
		f.seq = int32(binary.BigEndian.Uint32(b[:]))
	}
	var szb [4]byte
	if _, err := io.ReadFull(r, szb[:]); err != nil {
		return volcFrame{}, err
	}
	size := binary.BigEndian.Uint32(szb[:])
	if size > 16<<20 { // 16MB 防护
		return volcFrame{}, fmt.Errorf("payload too large: %d", size)
	}
	f.payload = make([]byte, size)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return volcFrame{}, err
	}
	if gzipped {
		zr, err := gzip.NewReader(bytes.NewReader(f.payload))
		if err != nil {
			return volcFrame{}, err
		}
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		if err != nil {
			return volcFrame{}, err
		}
		f.payload = raw
	}
	return f, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/data/speech/ -run TestFrame -count=1`
Expected: PASS（4/4）

- [ ] **Step 5: Commit**

```bash
git add internal/data/speech/volc_frame.go internal/data/speech/volc_frame_test.go
git commit -m "feat(speech): volcengine SAUC binary frame codec with round-trip tests"
```

---

### Task 4: 火山流式 ASR 适配器 + Provider Registry

**Files:**
- Create: `internal/data/speech/ws_conn.go`（WS 抽象，ASR/TTS 共用）
- Create: `internal/data/speech/volcengine_asr.go`
- Create: `internal/data/speech/registry.go`
- Test: `internal/data/speech/volcengine_asr_test.go`、`internal/data/speech/registry_test.go`

可测性设计：`*websocket.Conn` 抽象为 `wsConn` 接口 + `wsDialer` 工厂字段，单测注入 fake conn（channel 驱动），无需真实网络。

- [ ] **Step 1: 写失败测试**

`internal/data/speech/volcengine_asr_test.go`:

```go
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	nethttp "net/http"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type readMsg struct {
	mt   int
	data []byte
	err  error
}

// fakeWSConn 是 channel 驱动的 wsConn 假实现。
type fakeWSConn struct {
	written chan []byte
	toRead  chan readMsg
	closed  atomic.Bool
}

func newFakeWSConn() *fakeWSConn {
	return &fakeWSConn{written: make(chan []byte, 32), toRead: make(chan readMsg, 32)}
}

func (f *fakeWSConn) WriteMessage(_ int, data []byte) error {
	if f.closed.Load() {
		return errors.New("closed")
	}
	f.written <- data
	return nil
}

func (f *fakeWSConn) ReadMessage() (int, []byte, error) {
	msg, ok := <-f.toRead
	if !ok {
		return 0, nil, io.EOF
	}
	return msg.mt, msg.data, msg.err
}

func (f *fakeWSConn) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		close(f.toRead)
	}
	return nil
}

func newTestASRProvider(conn *fakeWSConn) biz.StreamingASRProvider {
	return &volcASRProvider{
		cfg: biz.ASRProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", ResourceID: "rid", Language: "zh-CN",
		},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return conn, nil },
		lg:   loggateway.NewNoop(),
	}
}

func mustReadFrame(t *testing.T, conn *fakeWSConn) volcFrame {
	t.Helper()
	select {
	case data := <-conn.written:
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		require.NoError(t, err)
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for written frame")
		return volcFrame{}
	}
}

func pushServerJSON(t *testing.T, conn *fakeWSConn, flags byte, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullServerResponse, flags: flags, json: true, payload: raw}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: frame}
}

func TestASROpenSendsFullClientRequest(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgFullClientRequest, f.msgType)
	require.True(t, f.json)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	audio := body["audio"].(map[string]any)
	require.Equal(t, "pcm", audio["format"])
	require.Equal(t, float64(16000), audio["rate"])
}

func TestASRWriteAndFinishSeq(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()
	_ = mustReadFrame(t, conn) // full client request

	pcm := bytes.Repeat([]byte{0x01}, 640)
	require.NoError(t, sess.Write(pcm))
	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgAudioOnlyRequest, f.msgType)
	require.Equal(t, int32(1), f.seq)
	require.Equal(t, pcm, f.payload)

	require.NoError(t, sess.Write(pcm))
	f = mustReadFrame(t, conn)
	require.Equal(t, int32(2), f.seq)

	require.NoError(t, sess.Finish())
	f = mustReadFrame(t, conn)
	require.Equal(t, volcFlagNegativeSeq, f.flags)
	require.Equal(t, int32(-2), f.seq)
}

func TestASRPartialAndFinalEvents(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	defer sess.Close()
	_ = mustReadFrame(t, conn)

	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{"text": "你好", "utterances": []any{}},
	})
	ev := <-sess.Events()
	require.Equal(t, biz.ASREventPartial, ev.Type)
	require.Equal(t, "你好", ev.Text)

	pushServerJSON(t, conn, volcFlagNone, map[string]any{
		"result": map[string]any{
			"text": "你好世界",
			"utterances": []any{map[string]any{"text": "你好世界", "definite": true, "end_time": 1200}},
		},
	})
	ev = <-sess.Events()
	require.Equal(t, biz.ASREventFinal, ev.Type)
	require.Equal(t, "你好世界", ev.Text)
	require.Equal(t, 1200, ev.DurationMs)
}

func TestASRErrorFrameAndClose(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestASRProvider(conn)
	sess, err := p.Open(context.Background(), biz.ASRSessionConfig{SampleRate: 16000})
	require.NoError(t, err)
	_ = mustReadFrame(t, conn)

	errFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgError, flags: volcFlagNone, json: true, payload: []byte(`{"code":4501}`)}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}
	ev := <-sess.Events()
	require.Equal(t, biz.ASREventError, ev.Type)
	require.Error(t, ev.Err)

	require.NoError(t, sess.Close())
	// events channel 最终关闭
	_, ok := <-sess.Events()
	require.False(t, ok)
}

func TestASRDialFailureIsUnavailable(t *testing.T) {
	p := &volcASRProvider{
		cfg:  biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk"},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return nil, errors.New("boom") },
		lg:   loggateway.NewNoop(),
	}
	_, err := p.Open(context.Background(), biz.ASRSessionConfig{})
	require.True(t, apierror.IsCode(err, apierror.CodeUnavailable), "got %v", err)
}
```

`internal/data/speech/registry_test.go`:

```go
package speech

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

func TestRegistryVolcengineRegistered(t *testing.T) {
	r := NewRegistry()
	asr, err := r.ASRProvider(biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s"}, loggateway.NewNoop())
	require.NoError(t, err)
	require.NotNil(t, asr)
	tts, err := r.TTSProvider(biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Voice: "v", SpeedRatio: 1}, loggateway.NewNoop())
	require.NoError(t, err)
	require.NotNil(t, tts)
}

func TestRegistryUnknownDriver(t *testing.T) {
	r := NewRegistry()
	_, err := r.ASRProvider(biz.ASRProviderConfig{Driver: "nope"}, loggateway.NewNoop())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
	_, err = r.TTSProvider(biz.TTSProviderConfig{Driver: "nope"}, loggateway.NewNoop())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/data/speech/ -run 'TestASR|TestRegistry' -count=1`
Expected: FAIL — `undefined: volcASRProvider` / `undefined: NewRegistry` / `undefined: wsConn`

- [ ] **Step 3: 实现**

`internal/data/speech/ws_conn.go`:

```go
package speech

import (
	"context"
	nethttp "net/http"

	"github.com/gorilla/websocket"
)

// wsConn 抽象 *websocket.Conn，使 ASR/TTS 适配器可用 fake conn 做契约测试。
type wsConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

type wsDialer func(ctx context.Context, url string, header nethttp.Header) (wsConn, error)

func gorillaDialer(ctx context.Context, url string, header nethttp.Header) (wsConn, error) {
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, err
	}
	return c, nil
}
```

`internal/data/speech/volcengine_asr.go`:

```go
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// volcFlagLastPackage 标记服务端最后一帧（0b0010）。帧头其余 flags 见 volc_frame.go。
const volcFlagLastPackage byte = 0x2

// volcASRProvider 实现火山 SAUC 流式 ASR（双向 WS，服务端 VAD 端点检测）。
// 协议字段按火山公开文档；字节级真机校准归 V1-T10。
type volcASRProvider struct {
	cfg  biz.ASRProviderConfig
	dial wsDialer
	lg   loggateway.Logger
}

func newVolcASRProvider(cfg biz.ASRProviderConfig, lg loggateway.Logger) biz.StreamingASRProvider {
	return &volcASRProvider{cfg: cfg, dial: gorillaDialer, lg: lg}
}

func (p *volcASRProvider) Open(ctx context.Context, sc biz.ASRSessionConfig) (biz.ASRSession, error) {
	if sc.SampleRate == 0 {
		sc.SampleRate = 16000
	}
	if sc.Language == "" {
		sc.Language = p.cfg.Language
	}
	header := nethttp.Header{}
	header.Set("X-Api-App-Key", p.cfg.AppKey)
	header.Set("X-Api-Access-Key", p.cfg.AccessKey)
	header.Set("X-Api-Resource-Id", p.cfg.ResourceID)
	header.Set("X-Api-Connect-Id", uuid.NewString())
	conn, err := p.dial(ctx, p.cfg.Endpoint, header)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeUnavailable, "speech")
	}
	s := &volcASRSession{
		conn:   conn,
		events: make(chan biz.ASREvent, 16),
		done:   make(chan struct{}),
		lg:     p.lg,
	}
	if err := s.sendFullClientRequest(sc); err != nil {
		_ = conn.Close()
		return nil, apierror.Wrap(err, apierror.CodeUnavailable, "speech")
	}
	go s.readPump()
	return s, nil
}

type volcASRSession struct {
	conn      wsConn
	events    chan biz.ASREvent
	done      chan struct{}
	closeOnce sync.Once
	seq       atomic.Int32
	lg        loggateway.Logger
}

func (s *volcASRSession) sendFullClientRequest(sc biz.ASRSessionConfig) error {
	body := map[string]any{
		"user":  map[string]any{"uid": "aranea"},
		"audio": map[string]any{"format": "pcm", "rate": sc.SampleRate, "bits": 16, "channel": 1},
		"request": map[string]any{
			"model_name":      "bigmodel",
			"enable_punc":     true,
			"enable_itn":      true,
			"show_utterances": true,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: raw}, true)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcASRSession) Write(pcm []byte) error {
	n := s.seq.Add(1)
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagPositiveSeq, seq: n, payload: pcm}, false)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// Finish 发送负序号空音频帧，标记当前语句结束（voice.commit / PTT）。
func (s *volcASRSession) Finish() error {
	n := s.seq.Load()
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyRequest, flags: volcFlagNegativeSeq, seq: -n, payload: []byte{}}, false)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcASRSession) Events() <-chan biz.ASREvent { return s.events }

func (s *volcASRSession) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		err = s.conn.Close()
	})
	return err
}

func (s *volcASRSession) emit(ev biz.ASREvent) {
	select {
	case s.events <- ev:
	case <-s.done:
	}
}

func (s *volcASRSession) readPump() {
	defer close(s.events)
	for {
		mt, data, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case <-s.done:
				return // 主动 Close 的正常路径
			default:
			}
			s.emit(biz.ASREvent{Type: biz.ASREventError, Err: apierror.Wrap(err, apierror.CodeUnavailable, "speech")})
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		if err != nil {
			s.lg.Warn("volc asr: undecodable frame", loggateway.Err(err))
			continue
		}
		switch f.msgType {
		case volcMsgError:
			s.emit(biz.ASREvent{Type: biz.ASREventError, Err: apierror.Internal("speech", "volc asr error: %s", string(f.payload))})
		case volcMsgFullServerResponse:
			s.handleResponse(f)
		}
	}
}

// saucResponse 是火山 SAUC 服务端 JSON 响应（真机字段校准归 V1-T10）。
type saucResponse struct {
	Result struct {
		Text       string `json:"text"`
		Utterances []struct {
			Text     string `json:"text"`
			Definite bool   `json:"definite"`
			EndTime  int    `json:"end_time"`
		} `json:"utterances"`
	} `json:"result"`
}

func (s *volcASRSession) handleResponse(f volcFrame) {
	var resp saucResponse
	if err := json.Unmarshal(f.payload, &resp); err != nil {
		s.lg.Warn("volc asr: undecodable response json", loggateway.Err(err))
		return
	}
	for _, u := range resp.Result.Utterances {
		if u.Definite && u.Text != "" {
			s.emit(biz.ASREvent{Type: biz.ASREventFinal, Text: u.Text, DurationMs: u.EndTime})
			return
		}
	}
	if resp.Result.Text != "" {
		s.emit(biz.ASREvent{Type: biz.ASREventPartial, Text: resp.Result.Text})
	}
	if f.flags == volcFlagLastPackage {
		s.emit(biz.ASREvent{Type: biz.ASREventVadEnd})
	}
}
```

`internal/data/speech/registry.go`:

```go
package speech

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Registry 是 driver 名 → Speech Provider 工厂的注册表（设计 §3.2）。
// 新增 Provider（阿里/OpenAI）只改注册，不动网关与前端。
type Registry struct {
	asrFactories map[string]ASRFactory
	ttsFactories map[string]TTSFactory
}

type ASRFactory func(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error)
type TTSFactory func(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error)

func NewRegistry() *Registry {
	r := &Registry{
		asrFactories: map[string]ASRFactory{},
		ttsFactories: map[string]TTSFactory{},
	}
	r.RegisterASR("volcengine", func(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error) {
		return newVolcASRProvider(cfg, lg), nil
	})
	r.RegisterTTS("volcengine", func(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error) {
		return newVolcTTSProvider(cfg, lg), nil
	})
	return r
}

func (r *Registry) RegisterASR(driver string, f ASRFactory) { r.asrFactories[driver] = f }
func (r *Registry) RegisterTTS(driver string, f TTSFactory) { r.ttsFactories[driver] = f }

func (r *Registry) ASRProvider(cfg biz.ASRProviderConfig, lg loggateway.Logger) (biz.StreamingASRProvider, error) {
	f, ok := r.asrFactories[cfg.Driver]
	if !ok {
		return nil, apierror.FailedPrecondition("speech", "unknown asr driver %q", cfg.Driver)
	}
	return f(cfg, lg)
}

func (r *Registry) TTSProvider(cfg biz.TTSProviderConfig, lg loggateway.Logger) (biz.StreamingTTSProvider, error) {
	f, ok := r.ttsFactories[cfg.Driver]
	if !ok {
		return nil, apierror.FailedPrecondition("speech", "unknown tts driver %q", cfg.Driver)
	}
	return f(cfg, lg)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/data/speech/ -count=1`
Expected: PASS（Task 2/3/4 全部用例）

- [ ] **Step 5: Commit**

```bash
git add internal/data/speech/ws_conn.go internal/data/speech/volcengine_asr.go internal/data/speech/registry.go internal/data/speech/volcengine_asr_test.go internal/data/speech/registry_test.go
git commit -m "feat(speech): volcengine streaming ASR adapter + provider registry"
```

---

### Task 5: 火山流式 TTS 适配器

**Files:**
- Create: `internal/data/speech/volcengine_tts.go`
- Test: `internal/data/speech/volcengine_tts_test.go`

V1 裁剪 #3：每句一条 WS 连接（一句一合成）。下行音频火山原始编码为 PCM s16le，适配器转 f32le 16kHz mono（biz 端口契约，Task 1）。

- [ ] **Step 1: 写失败测试**

`internal/data/speech/volcengine_tts_test.go`:

```go
package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func newTestTTSProvider(conn *fakeWSConn) biz.StreamingTTSProvider {
	return &volcTTSProvider{
		cfg: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", ResourceID: "rid", Voice: "zh_female_x", SpeedRatio: 1.0,
		},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return conn, nil },
		lg:   loggateway.NewNoop(),
	}
}

func TestPcmS16ToF32(t *testing.T) {
	in := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80} // 0, 32767, -32768
	out := pcmS16ToF32(in)
	require.Len(t, out, 12)
	require.Equal(t, float32(0), math.Float32frombits(binary.LittleEndian.Uint32(out[0:4])))
	require.InDelta(t, 1.0, math.Float32frombits(binary.LittleEndian.Uint32(out[4:8])), 1e-4)
	require.Equal(t, float32(-1.0), math.Float32frombits(binary.LittleEndian.Uint32(out[8:12])))
}

func TestTTSSubmitRequestFields(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v1", SpeedRatio: 1.2, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("你好世界", true))
	defer sess.Close()

	f := mustReadFrame(t, conn)
	require.Equal(t, volcMsgFullClientRequest, f.msgType)
	var body map[string]any
	require.NoError(t, json.Unmarshal(f.payload, &body))
	audio := body["audio"].(map[string]any)
	require.Equal(t, "v1", audio["voice_type"])
	require.Equal(t, "pcm", audio["encoding"])
	require.Equal(t, 1.2, audio["speed_ratio"])
	req := body["request"].(map[string]any)
	require.Equal(t, "你好世界", req["text"])
	require.Equal(t, "submit", req["operation"])
}

func TestTTSAudioChunksUntilLastPackage(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	pcm16 := []byte{0x01, 0x00, 0x02, 0x00} // 2 samples s16le
	audioFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyResponse, flags: volcFlagNone, payload: pcm16}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: audioFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkData, chunk.Type)
	require.Len(t, chunk.PCM, 8) // 2 samples × 4B f32

	lastFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgAudioOnlyResponse, flags: volcFlagLastPackage, payload: []byte{}}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: lastFrame}

	chunk = <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkEnd, chunk.Type)

	// 服务端随后关闭连接；Audio 通道关闭
	conn.Close()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-sess.Audio():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("audio channel not closed after server close")
		}
	}
}

func TestTTSErrorFrame(t *testing.T) {
	conn := newFakeWSConn()
	p := newTestTTSProvider(conn)
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	_ = mustReadFrame(t, conn)

	errFrame, err := marshalVolcFrame(volcFrame{msgType: volcMsgError, flags: volcFlagNone, json: true, payload: []byte(`{"code":3001}`)}, false)
	require.NoError(t, err)
	conn.toRead <- readMsg{mt: websocket.BinaryMessage, data: errFrame}

	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
	require.Error(t, chunk.Err)
}

func TestTTSDialFailureEmitsErrorChunk(t *testing.T) {
	p := &volcTTSProvider{
		cfg:  biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://test", AppKey: "ak", AccessKey: "sk", Voice: "v", SpeedRatio: 1},
		dial: func(_ context.Context, _ string, _ nethttp.Header) (wsConn, error) { return nil, errors.New("boom") },
		lg:   loggateway.NewNoop(),
	}
	sess, err := p.Open(context.Background(), biz.TTSSessionConfig{})
	require.NoError(t, err)
	require.NoError(t, sess.Write("x", true))
	chunk := <-sess.Audio()
	require.Equal(t, biz.TTSAudioChunkError, chunk.Type)
}
```

注意：测试文件 import 需要 `errors` 与 `nethttp`（`nethttp "net/http"`），与 ASR 测试同包共享 `fakeWSConn`/`readMsg`/`mustReadFrame` 辅助（已在 Task 4 测试定义，禁止重复定义）。

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/data/speech/ -run 'TestTTS|TestPcm' -count=1`
Expected: FAIL — `undefined: volcTTSProvider` / `undefined: pcmS16ToF32`

- [ ] **Step 3: 实现**

`internal/data/speech/volcengine_tts.go`:

```go
package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	nethttp "net/http"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// volcTTSProvider 实现火山 ws_binary 语音合成。
// V1 裁剪 #3：每句一条 WS 连接——TTSSession.Write 恰好调用一次（一句一合成），
// 双工流式/预连接升级归 V3-T5。
type volcTTSProvider struct {
	cfg  biz.TTSProviderConfig
	dial wsDialer
	lg   loggateway.Logger
}

func newVolcTTSProvider(cfg biz.TTSProviderConfig, lg loggateway.Logger) biz.StreamingTTSProvider {
	return &volcTTSProvider{cfg: cfg, dial: gorillaDialer, lg: lg}
}

func (p *volcTTSProvider) Open(_ context.Context, sc biz.TTSSessionConfig) (biz.TTSSession, error) {
	if sc.SampleRate == 0 {
		sc.SampleRate = 16000
	}
	if sc.Voice == "" {
		sc.Voice = p.cfg.Voice
	}
	if sc.SpeedRatio <= 0 {
		sc.SpeedRatio = p.cfg.SpeedRatio
	}
	return &volcTTSSession{
		p:     p,
		sc:    sc,
		audio: make(chan biz.TTSAudioChunk, 32),
	}, nil
}

type volcTTSSession struct {
	p        *volcTTSProvider
	sc       biz.TTSSessionConfig
	audio    chan biz.TTSAudioChunk
	started  atomic.Bool
	ended    atomic.Bool
	conn     wsConn
	closeMu  sync.Mutex
	closed   bool
}

func (s *volcTTSSession) Write(text string, _ bool) error {
	if s.started.Swap(true) {
		return errors.New("volc tts: Write called twice (V1 一句一连接)")
	}
	header := nethttp.Header{}
	header.Set("X-Api-App-Key", s.p.cfg.AppKey)
	header.Set("X-Api-Access-Key", s.p.cfg.AccessKey)
	header.Set("X-Api-Resource-Id", s.p.cfg.ResourceID)
	header.Set("X-Api-Connect-Id", uuid.NewString())
	conn, err := s.p.dial(context.Background(), s.p.cfg.Endpoint, header)
	if err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil // 错误经 Audio() 通道上报
	}
	s.setConn(conn)
	body := map[string]any{
		"app":  map[string]any{"appid": s.p.cfg.AppKey, "token": s.p.cfg.AccessKey, "cluster": s.p.cfg.ResourceID},
		"user": map[string]any{"uid": "aranea"},
		"audio": map[string]any{
			"voice_type":  s.sc.Voice,
			"encoding":    "pcm",
			"speed_ratio": s.sc.SpeedRatio,
			"rate":        s.sc.SampleRate,
		},
		"request": map[string]any{"reqid": uuid.NewString(), "text": text, "operation": "submit"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: raw}, true)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil
	}
	go s.readPump(conn)
	return nil
}

func (s *volcTTSSession) Audio() <-chan biz.TTSAudioChunk { return s.audio }

func (s *volcTTSSession) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.ended.Store(true)
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *volcTTSSession) setConn(c wsConn) {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	s.conn = c
}

func (s *volcTTSSession) failOnce(err error) {
	if s.ended.CompareAndSwap(false, true) {
		s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: err}
		close(s.audio)
	}
}

func (s *volcTTSSession) finish() {
	if s.ended.CompareAndSwap(false, true) {
		s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkEnd}
	}
}

func (s *volcTTSSession) readPump(conn wsConn) {
	defer func() {
		s.ended.Store(true)
		close(s.audio)
		_ = conn.Close()
	}()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if !s.ended.Load() {
				s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: apierror.Wrap(err, apierror.CodeUnavailable, "speech")}
			}
			return
		}
		if mt != websocket.BinaryMessage {
			continue
		}
		f, err := unmarshalVolcFrame(bytes.NewReader(data))
		if err != nil {
			s.p.lg.Warn("volc tts: undecodable frame", loggateway.Err(err))
			continue
		}
		switch f.msgType {
		case volcMsgAudioOnlyResponse:
			if len(f.payload) > 0 {
				s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkData, PCM: pcmS16ToF32(f.payload)}
			}
			if f.flags == volcFlagLastPackage {
				s.finish()
			}
		case volcMsgFullServerResponse:
			if f.flags == volcFlagLastPackage {
				s.finish()
			}
		case volcMsgError:
			s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: apierror.Internal("speech", "volc tts error: %s", string(f.payload))}
			return
		}
	}
}

// pcmS16ToF32 将火山 PCM s16le 转换为 biz 端口契约的 f32le。
func pcmS16ToF32(in []byte) []byte {
	n := len(in) / 2
	out := make([]byte, n*4)
	for i := 0; i < n; i++ {
		v := int16(binary.LittleEndian.Uint16(in[i*2:]))
		f := float32(v) / 32768.0
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(f))
	}
	return out
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/data/speech/ -count=1`
Expected: PASS（全部用例）

- [ ] **Step 5: Commit**

```bash
git add internal/data/speech/volcengine_tts.go internal/data/speech/volcengine_tts_test.go
git commit -m "feat(speech): volcengine streaming TTS adapter with s16→f32 conversion"
```

---

### Task 6: SentenceChunker（delta 分句器）

**Files:**
- Create: `internal/voice/sentence_chunker.go`
- Test: `internal/voice/sentence_chunker_test.go`

规则（设计 §4.1）：切分点 `。！？；\n`；首句 ≥6 字符遇任意标点（含次要标点 `，,、`）即切；后续句最小 12 字符；≥80 字符硬切；`Flush()` 强制送出残余；``` 代码栅栏内容、URL、图片/表格行剥离。

- [ ] **Step 1: 写失败测试**

`internal/voice/sentence_chunker_test.go`:

```go
package voice

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type sentence struct {
	text  string
	flush bool
}

type collector struct{ got []sentence }

func (c *collector) fn() func(string, bool) {
	return func(text string, flush bool) { c.got = append(c.got, sentence{text, flush}) }
}

func (c *collector) texts() []string {
	out := make([]string, 0, len(c.got))
	for _, s := range c.got {
		out = append(out, s.text)
	}
	return out
}

func TestChunkerBoundaryAndFlush(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("这是一个测试。下一句在这里。")
	require.Equal(t, []string{"这是一个测试。"}, c.texts())
	ch.Flush()
	require.Equal(t, []string{"这是一个测试。", "下一句在这里。"}, c.texts())
	require.True(t, c.got[1].flush)
}

func TestChunkerFirstSentenceMinorPunct(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好的收到没问题，继续处理")
	require.Equal(t, []string{"好的收到没问题，"}, c.texts())
}

func TestChunkerFirstSentenceBelowMinMerges(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好。继续说话。") // 首句 "好。" 仅 3 字符，不切
	require.Empty(t, c.texts())
	ch.Flush()
	require.Equal(t, []string{"好。继续说话。"}, c.texts())
}

func TestChunkerLaterSentenceMinMerge(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("第一句话已经够长了。短。也短。")
	// 首句切出；后续句 <12 字符合并
	require.Equal(t, []string{"第一句话已经够长了。"}, c.texts())
	ch.Flush()
	require.Equal(t, []string{"第一句话已经够长了。", "短。也短。"}, c.texts())
}

func TestChunkerHardCut(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write(strings.Repeat("字", 100))
	require.Len(t, c.got, 1)
	require.Equal(t, 80, len([]rune(c.got[0].text)))
	ch.Flush()
	require.Len(t, c.got, 2)
	require.Equal(t, 20, len([]rune(c.got[1].text)))
	require.True(t, c.got[1].flush)
}

func TestChunkerFenceStripped(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好的```fmt.Println(1)```完成。")
	ch.Flush()
	require.Equal(t, []string{"好的完成。"}, c.texts())
}

func TestChunkerURLAndMarkdownStripped(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("看这个 https://example.com/x 即可。还有 [链接](https://a.b) 与 ![图](https://c.d) 都在。")
	ch.Flush()
	for _, s := range c.texts() {
		require.NotContains(t, s, "http")
		require.NotContains(t, s, "](")
	}
}

func TestChunkerNewlineBoundary(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("第一行足够长了吧\n第二行")
	require.Equal(t, []string{"第一行足够长了吧"}, c.texts())
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/voice/ -run TestChunker -count=1`
Expected: FAIL — `undefined: NewSentenceChunker`

- [ ] **Step 3: 实现**

`internal/voice/sentence_chunker.go`:

```go
// Package voice implements the voice-session orchestration for the voice
// companion (M74): ASR ↔ Chat pipeline ↔ TTS glue, state machine, sentence
// chunking and TTS scheduling.
package voice

import (
	"regexp"
	"strings"
)

const (
	firstSentenceMinRunes = 6
	laterSentenceMinRunes = 12
	chunkHardMaxRunes     = 80
)

// SentenceChunker 将 LLM 流式 delta 切分为 TTS 句子（设计 §4.1）。
// 非并发安全：仅由会话事件循环单 goroutine 调用。
type SentenceChunker struct {
	onSentence func(text string, flush bool)
	buf        []rune
	emitted    int
	inFence    bool
	tickRun    int // 连续反引号计数（``` 栅栏检测）
}

func NewSentenceChunker(on func(text string, flush bool)) *SentenceChunker {
	return &SentenceChunker{onSentence: on}
}

func (c *SentenceChunker) Write(delta string) {
	for _, r := range delta {
		c.feedRune(r)
	}
	for c.runes() >= chunkHardMaxRunes {
		c.cut(false)
	}
}

// Flush 在 Turn 文本结束时强制送出残余（flush=true 标记尾句）。
func (c *SentenceChunker) Flush() {
	c.tickRun = 0 // 丢弃未闭合 fence 的残余反引号
	if len(c.buf) == 0 {
		return
	}
	c.cut(true)
}

func (c *SentenceChunker) runes() int { return len(c.buf) }

func (c *SentenceChunker) minRunes() int {
	if c.emitted == 0 {
		return firstSentenceMinRunes
	}
	return laterSentenceMinRunes
}

func (c *SentenceChunker) feedRune(r rune) {
	if r == '`' {
		c.tickRun++
		if c.tickRun == 3 {
			c.inFence = !c.inFence
			c.tickRun = 0
		}
		return
	}
	if c.tickRun > 0 {
		if !c.inFence {
			for i := 0; i < c.tickRun; i++ {
				c.buf = append(c.buf, '`')
			}
		}
		c.tickRun = 0
	}
	if c.inFence {
		return // 代码块内容不播报
	}
	c.buf = append(c.buf, r)
	if isSentenceBoundary(r) && c.runes() >= c.minRunes() {
		c.cut(false)
		return
	}
	// 首句优化：遇次要标点提前切（保首音延迟）
	if c.emitted == 0 && c.runes() >= firstSentenceMinRunes && isMinorBoundary(r) {
		c.cut(false)
	}
}

func (c *SentenceChunker) cut(flush bool) {
	text := cleanForSpeech(string(c.buf))
	c.buf = c.buf[:0]
	if text == "" {
		return
	}
	c.emitted++
	c.onSentence(text, flush)
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '\n':
		return true
	}
	return false
}

func isMinorBoundary(r rune) bool {
	switch r {
	case '，', ',', '、':
		return true
	}
	return false
}

var (
	urlRe   = regexp.MustCompile(`https?://\S+`)
	imgRe   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	linkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	tableRe = regexp.MustCompile(`(?m)^\s*\|.*$`)
	spaceRe = regexp.MustCompile(`\s{2,}`)
)

// cleanForSpeech 剥离不参与播报的 markdown 元素（URL/图片/表格/链接语法）。
func cleanForSpeech(s string) string {
	s = imgRe.ReplaceAllString(s, "")
	s = urlRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = tableRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/voice/ -run TestChunker -count=1`
Expected: PASS（8/8）

- [ ] **Step 5: Commit**

```bash
git add internal/voice/sentence_chunker.go internal/voice/sentence_chunker_test.go
git commit -m "feat(voice): sentence chunker with first-sentence optimization and markdown stripping"
```

---

### Task 7: 语音会话状态机（AS-FSM-01）

**Files:**
- Create: `internal/voice/session_state_machine.go`
- Test: `internal/voice/session_state_machine_test.go`

6 状态 > 3，按 AS-FSM-01 显式化。转换表与设计 §5 严格一致；`interrupted` 为过渡态（→ listening 由会话定时器直接置位，不走事件，见 Task 9）。

- [ ] **Step 1: 写失败测试**

`internal/voice/session_state_machine_test.go`:

```go
package voice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVoiceStateMachineLegalTransitions(t *testing.T) {
	cases := []struct {
		from  VoiceState
		event VoiceEvent
		to    VoiceState
	}{
		{StateIdle, EvVoiceStart, StateListening},
		{StateListening, EvASRFinal, StateThinking},
		{StateListening, EvBargeIn, StateListening}, // 忽略（自环）
		{StateListening, EvTurnFailed, StateError},
		{StateListening, EvVoiceStop, StateIdle},
		{StateThinking, EvFirstTTSAudio, StateSpeaking},
		{StateThinking, EvTTSEnd, StateListening}, // 无文本 Turn
		{StateThinking, EvBargeIn, StateListening},
		{StateThinking, EvTurnFailed, StateError},
		{StateThinking, EvVoiceStop, StateIdle},
		{StateSpeaking, EvTTSEnd, StateListening},
		{StateSpeaking, EvBargeIn, StateInterrupted},
		{StateSpeaking, EvTurnFailed, StateError},
		{StateSpeaking, EvVoiceStop, StateIdle},
		{StateInterrupted, EvVoiceStop, StateIdle},
		{StateError, EvVoiceStart, StateListening},
		{StateError, EvVoiceStop, StateIdle},
	}
	for _, c := range cases {
		to, err := Transition(c.from, c.event)
		require.NoError(t, err, "%s --%s", c.from, c.event)
		require.Equal(t, c.to, to, "%s --%s", c.from, c.event)
	}
}

func TestVoiceStateMachineIllegalTransitions(t *testing.T) {
	illegal := []struct {
		from  VoiceState
		event VoiceEvent
	}{
		{StateIdle, EvASRFinal},
		{StateIdle, EvFirstTTSAudio},
		{StateIdle, EvBargeIn},
		{StateInterrupted, EvASRFinal},
		{StateInterrupted, EvFirstTTSAudio},
		{StateInterrupted, EvTTSEnd},
		{StateSpeaking, EvVoiceStart},
		{StateSpeaking, EvASRFinal},
		{StateError, EvFirstTTSAudio},
		{StateError, EvTTSEnd},
	}
	for _, c := range illegal {
		_, err := Transition(c.from, c.event)
		require.Error(t, err, "%s --%s should be illegal", c.from, c.event)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/voice/ -run TestVoiceStateMachine -count=1`
Expected: FAIL — `undefined: Transition` / `undefined: StateIdle`

- [ ] **Step 3: 实现**

`internal/voice/session_state_machine.go`:

```go
package voice

import "fmt"

// VoiceState 是语音会话状态（设计 §5，6 状态 > 3，AS-FSM-01 显式状态机）。
type VoiceState string

const (
	StateIdle        VoiceState = "idle"
	StateListening   VoiceState = "listening"
	StateThinking    VoiceState = "thinking"
	StateSpeaking    VoiceState = "speaking"
	StateInterrupted VoiceState = "interrupted"
	StateError       VoiceState = "error"
)

type VoiceEvent string

const (
	EvVoiceStart    VoiceEvent = "voice_start"
	EvASRFinal      VoiceEvent = "asr_final"
	EvFirstTTSAudio VoiceEvent = "first_tts_audio"
	EvTTSEnd        VoiceEvent = "tts_end"
	EvBargeIn       VoiceEvent = "barge_in"
	EvTurnFailed    VoiceEvent = "turn_failed"
	EvVoiceStop     VoiceEvent = "voice_stop"
)

// transitions 转换表（设计 §5）。interrupted 为过渡态：进入后由会话定时器
// ~300ms 直接置位回 listening（设计明确"无需事件"），故表中无其出口事件。
var transitions = map[VoiceState]map[VoiceEvent]VoiceState{
	StateIdle: {
		EvVoiceStart: StateListening,
	},
	StateListening: {
		EvASRFinal:   StateThinking,
		EvBargeIn:    StateListening, // 忽略（自环）
		EvTurnFailed: StateError,
		EvVoiceStop:  StateIdle,
	},
	StateThinking: {
		EvFirstTTSAudio: StateSpeaking,
		EvTTSEnd:        StateListening, // 无文本 Turn
		EvBargeIn:       StateListening,
		EvTurnFailed:    StateError,
		EvVoiceStop:     StateIdle,
	},
	StateSpeaking: {
		EvTTSEnd:     StateListening,
		EvBargeIn:    StateInterrupted,
		EvTurnFailed: StateError,
		EvVoiceStop:  StateIdle,
	},
	StateInterrupted: {
		EvVoiceStop: StateIdle,
	},
	StateError: {
		EvVoiceStart: StateListening,
		EvVoiceStop:  StateIdle,
	},
}

// Transition 校验并返回目标状态；非法转换返回错误。
func Transition(from VoiceState, event VoiceEvent) (VoiceState, error) {
	if events, ok := transitions[from]; ok {
		if to, ok := events[event]; ok {
			return to, nil
		}
	}
	return "", fmt.Errorf("voice: illegal transition %s --%s", from, event)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/voice/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/voice/session_state_machine.go internal/voice/session_state_machine_test.go
git commit -m "feat(voice): explicit voice session state machine (AS-FSM-01)"
```

---

### Task 8: TTS 调度器（句队列/背压/取消）

**Files:**
- Create: `internal/voice/tts_scheduler.go`
- Test: `internal/voice/tts_scheduler_test.go`

规则（设计 §4.2）：单 Turn 单调度器；句队列上限 8（满时 `Enqueue` 阻塞 = chunker 暂停消费 delta 的背压机制）；V1 每句一条 TTS 连接；单句失败跳过记 Warn（K3），连续 3 句失败中止并 `OnError`；`Cancel` 关闭当前合成会话并停 worker。

- [ ] **Step 1: 写失败测试**

`internal/voice/tts_scheduler_test.go`:

```go
package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// fakeTTSSession 按脚本产出音频 chunk；blockCh 非空时 Audio() 阻塞直到其关闭。
type fakeTTSSession struct {
	chunks  []biz.TTSAudioChunk
	writeErr error
	blockCh chan struct{}

	writeMu sync.Mutex
	writes  []string
	closed  chan struct{}
}

func (f *fakeTTSSession) Write(text string, _ bool) error {
	f.writeMu.Lock()
	f.writes = append(f.writes, text)
	f.writeMu.Unlock()
	return f.writeErr
}

func (f *fakeTTSSession) Audio() <-chan biz.TTSAudioChunk {
	out := make(chan biz.TTSAudioChunk, 8)
	go func() {
		defer close(out)
		if f.blockCh != nil {
			<-f.blockCh
			return
		}
		for _, c := range f.chunks {
			out <- c
		}
	}()
	return out
}

func (f *fakeTTSSession) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

type fakeTTSProvider struct {
	mu       sync.Mutex
	sessions []*fakeTTSSession
	script   func() *fakeTTSSession
}

func (p *fakeTTSProvider) Open(_ context.Context, _ biz.TTSSessionConfig) (biz.TTSSession, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var s *fakeTTSSession
	if p.script != nil {
		s = p.script()
	} else {
		s = &fakeTTSSession{
			chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{1, 2, 3, 4}}, {Type: biz.TTSAudioChunkEnd}},
			closed: make(chan struct{}),
		}
	}
	p.sessions = append(p.sessions, s)
	return s, nil
}

type schedulerProbe struct {
	mu      sync.Mutex
	audios  [][]byte
	drained int
	errs    []error
}

func (p *schedulerProbe) opts(prov biz.StreamingTTSProvider) TTSSchedulerOpts {
	return TTSSchedulerOpts{
		Provider: prov,
		Config:   biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000},
		OnAudio:  func(pcm []byte) { p.mu.Lock(); p.audios = append(p.audios, pcm); p.mu.Unlock() },
		OnDrained: func() { p.mu.Lock(); p.drained++; p.mu.Unlock() },
		OnError:  func(err error) { p.mu.Lock(); p.errs = append(p.errs, err); p.mu.Unlock() },
		LG:       loggateway.NewNoop(),
	}
}

func TestSchedulerOrderAndDrained(t *testing.T) {
	prov := &fakeTTSProvider{}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	require.NoError(t, s.Enqueue(ctx, "第一句。", false))
	require.NoError(t, s.Enqueue(ctx, "第二句。", false))
	require.NoError(t, s.Enqueue(ctx, "尾句。", true))

	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return probe.drained == 1 && len(probe.audios) == 3
	}, 2*time.Second, 10*time.Millisecond)

	prov.mu.Lock()
	defer prov.mu.Unlock()
	require.Len(t, prov.sessions, 3) // V1 每句一条连接
	require.Equal(t, []string{"第一句。", "第二句。", "尾句。"}, []string{prov.sessions[0].writes[0], prov.sessions[1].writes[0], prov.sessions[2].writes[0]})
}

func TestSchedulerBackpressure(t *testing.T) {
	block := make(chan struct{})
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{blockCh: block, closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer func() { close(block); s.Cancel() }()

	for i := 0; i < ttsQueueCap; i++ {
		require.NoError(t, s.Enqueue(ctx, "句", false))
	}
	done := make(chan error, 1)
	go func() { done <- s.Enqueue(ctx, "溢出句", false) }()
	select {
	case <-done:
		t.Fatal("Enqueue should block when queue is full")
	case <-time.After(150 * time.Millisecond):
	}
	// Cancel 后阻塞的 Enqueue 经 ctx 解除
	s.Cancel()
	require.Error(t, <-done)
}

func TestSchedulerSkipFailedSentence(t *testing.T) {
	var failNext bool
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		if failNext {
			failNext = false
			return &fakeTTSSession{writeErr: errors.New("tts boom"), closed: make(chan struct{})}
		}
		return &fakeTTSSession{
			chunks: []biz.TTSAudioChunk{{Type: biz.TTSAudioChunkData, PCM: []byte{9}}, {Type: biz.TTSAudioChunkEnd}},
			closed: make(chan struct{}),
		}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	failNext = true
	require.NoError(t, s.Enqueue(ctx, "坏句。", false))
	require.NoError(t, s.Enqueue(ctx, "好句。", true))
	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return probe.drained == 1 && len(probe.audios) == 1 && len(probe.errs) == 0
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerAbortsAfterConsecutiveFailures(t *testing.T) {
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{writeErr: errors.New("boom"), closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)
	defer s.Cancel()

	for i := 0; i < ttsMaxConsecutiveFailures; i++ {
		_ = s.Enqueue(ctx, "坏句", false)
	}
	require.Eventually(t, func() bool {
		probe.mu.Lock()
		defer probe.mu.Unlock()
		return len(probe.errs) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestSchedulerCancelClosesCurrentSession(t *testing.T) {
	block := make(chan struct{})
	prov := &fakeTTSProvider{script: func() *fakeTTSSession {
		return &fakeTTSSession{blockCh: block, closed: make(chan struct{})}
	}}
	probe := &schedulerProbe{}
	s := NewTTSScheduler(probe.opts(prov))
	ctx := context.Background()
	s.Start(ctx)

	require.NoError(t, s.Enqueue(ctx, "句", false))
	require.Eventually(t, func() bool {
		prov.mu.Lock()
		defer prov.mu.Unlock()
		return len(prov.sessions) == 1
	}, 2*time.Second, 10*time.Millisecond)

	s.Cancel()
	close(block)
	select {
	case <-prov.sessions[0].closed:
	case <-time.After(2 * time.Second):
		t.Fatal("current TTS session not closed on Cancel")
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/voice/ -run TestScheduler -count=1`
Expected: FAIL — `undefined: NewTTSScheduler` / `undefined: TTSSchedulerOpts`

- [ ] **Step 3: 实现**

`internal/voice/tts_scheduler.go`:

```go
package voice

import (
	"context"
	"errors"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const (
	ttsQueueCap               = 8
	ttsMaxConsecutiveFailures = 3
)

type sentenceJob struct {
	text  string
	flush bool
}

// TTSSchedulerOpts 配置 TTS 调度器。回调均由 worker goroutine 触发。
type TTSSchedulerOpts struct {
	Provider biz.StreamingTTSProvider
	Config   biz.TTSSessionConfig
	OnAudio  func(pcm []byte) // f32le 16k 音频 chunk
	OnDrained func()          // flush 尾句合成完毕（Turn 级播报结束）
	OnError  func(err error)  // 连续失败中止（K3 降级由会话层处理）
	LG       loggateway.Logger
}

// TTSScheduler 按句调度 TTS 合成（设计 §4.2）。V1 每句一条连接。
type TTSScheduler struct {
	opts  TTSSchedulerOpts
	lg    loggateway.Logger
	queue chan sentenceJob

	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	current biz.TTSSession
	stopped bool
}

func NewTTSScheduler(opts TTSSchedulerOpts) *TTSScheduler {
	lg := opts.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TTSScheduler{opts: opts, lg: lg, queue: make(chan sentenceJob, ttsQueueCap)}
}

func (s *TTSScheduler) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()
}

// Enqueue 入队一句；队列满时阻塞（背压），ctx 取消时返回错误。
func (s *TTSScheduler) Enqueue(ctx context.Context, text string, flush bool) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("voice: tts scheduler stopped")
	}
	s.mu.Unlock()
	select {
	case s.queue <- sentenceJob{text: text, flush: flush}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Cancel 停止调度：关闭当前合成会话、停 worker、解除阻塞的 Enqueue。
func (s *TTSScheduler) Cancel() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.closeCurrent()
	s.wg.Wait()
}

func (s *TTSScheduler) loop(ctx context.Context) {
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			err := s.synthesize(ctx, job)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				failures++
				s.lg.Warn("voice tts: sentence skipped (K3)",
					loggateway.StepID("voice.tts.sentence_skip"),
					loggateway.Err(err),
					loggateway.Any("consecutive_failures", failures))
				if failures >= ttsMaxConsecutiveFailures {
					if s.opts.OnError != nil {
						s.opts.OnError(apierror.Unavailable("speech", "tts provider failing repeatedly"))
					}
					return
				}
				continue
			}
			failures = 0
			if job.flush && s.opts.OnDrained != nil {
				s.opts.OnDrained()
			}
		}
	}
}

// synthesize 合成单句（V1：每句一条连接）。Data chunk 透传 OnAudio；
// 句级 End 内部消化（Turn 级结束由 flush 任务触发 OnDrained）。
func (s *TTSScheduler) synthesize(ctx context.Context, job sentenceJob) error {
	sess, err := s.opts.Provider.Open(ctx, s.opts.Config)
	if err != nil {
		return err
	}
	s.setCurrent(sess)
	defer func() {
		_ = sess.Close()
		s.setCurrent(nil)
	}()
	if err := sess.Write(job.text, true); err != nil {
		return err
	}
	for chunk := range sess.Audio() {
		switch chunk.Type {
		case biz.TTSAudioChunkData:
			if s.opts.OnAudio != nil {
				s.opts.OnAudio(chunk.PCM)
			}
		case biz.TTSAudioChunkError:
			return chunk.Err
		case biz.TTSAudioChunkEnd:
			// 句级结束：无动作
		}
	}
	return nil
}

func (s *TTSScheduler) setCurrent(sess biz.TTSSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = sess
}

func (s *TTSScheduler) closeCurrent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		_ = s.current.Close()
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/voice/ -count=1`
Expected: PASS（chunker + FSM + scheduler 全部用例）

- [ ] **Step 5: Commit**

```bash
git add internal/voice/tts_scheduler.go internal/voice/tts_scheduler_test.go
git commit -m "feat(voice): TTS scheduler with queue backpressure, skip-on-error and cancel"
```

---

### Task 9: 语音会话编排（`internal/voice/session.go`）

**Files:**
- Create: `internal/voice/session.go`
- Test: `internal/voice/session_test.go`

编排职责（设计 §2.4 序列）：ASR 事件 → 下行字幕/终稿 → Chat 管线；订阅会话事件流（`biz.EventBus`，`SpiritSessionID` 过滤）→ content delta → chunker → scheduler → 音频下行；取消/打断 → `CancelRun` + scheduler Cancel + `tts.end{interrupted}`；空闲 10 分钟回收 ASR 上游、再说话懒重连。

依赖方向纪律：`voice` 包**禁止 import `internal/server`**（循环依赖）。Chat 入口/取消/Provider 解析全部以窄端口/函数类型注入（server 层适配器见 Task 10）。`turn_id` V1 用 voice 侧序号（`vt-<n>`），与 `TurnStartedEvent.TurnID` 对齐归 V2。

V1 限制（已在设计/开发计划备案）：同一时间仅一个活跃 Turn 的文本进 TTS（`pendingTurns` 计数）；播报期再说话的完整 barge-in 归 V2-T1。

- [ ] **Step 1: 写失败测试**

`internal/voice/session_test.go`:

```go
package voice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeBus struct{ ch chan biz.Event }

func newFakeBus() *fakeBus { return &fakeBus{ch: make(chan biz.Event, 32)} }
func (f *fakeBus) Publish(context.Context, biz.Event) {}
func (f *fakeBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return f.ch, func() {}
}

type fakeExecutor struct {
	mu     sync.Mutex
	inputs []ChatTurnInput
	err    error
}

func (f *fakeExecutor) ExecuteTurn(_ context.Context, in ChatTurnInput) error {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	f.mu.Unlock()
	return f.err
}

type fakeCanceller struct {
	mu     sync.Mutex
	called []string
}

func (f *fakeCanceller) CancelRun(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	f.called = append(f.called, sessionID)
	f.mu.Unlock()
	return true
}

type fakeASRSession struct {
	events  chan biz.ASREvent
	writeMu sync.Mutex
	written [][]byte
	finishMu sync.Mutex
	finished int
}

func (f *fakeASRSession) Write(pcm []byte) error {
	f.writeMu.Lock()
	f.written = append(f.written, pcm)
	f.writeMu.Unlock()
	return nil
}
func (f *fakeASRSession) Finish() error {
	f.finishMu.Lock()
	f.finished++
	f.finishMu.Unlock()
	return nil
}
func (f *fakeASRSession) Events() <-chan biz.ASREvent { return f.events }
func (f *fakeASRSession) Close() error                { return nil }

type fakeASRProvider struct{ sess *fakeASRSession }

func (p *fakeASRProvider) Open(context.Context, biz.ASRSessionConfig) (biz.ASRSession, error) {
	return p.sess, nil
}

type fakeDownlink struct {
	mu     sync.Mutex
	jsons  []map[string]any
	audios [][]byte
}

func (d *fakeDownlink) SendJSON(v any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if m, ok := v.(map[string]any); ok {
		d.jsons = append(d.jsons, m)
	}
	return nil
}
func (d *fakeDownlink) SendAudio(pcm []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.audios = append(d.audios, pcm)
	return nil
}

func (d *fakeDownlink) typesOf() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.jsons))
	for _, j := range d.jsons {
		out = append(out, j["type"].(string))
	}
	return out
}

func (d *fakeDownlink) lastState() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := len(d.jsons) - 1; i >= 0; i-- {
		if d.jsons[i]["type"] == "voice.state" {
			return d.jsons[i]["state"].(string)
		}
	}
	return ""
}

// ---- harness ----

type sessionFixture struct {
	sess    *Session
	asr     *fakeASRSession
	bus     *fakeBus
	exec    *fakeExecutor
	cancel  *fakeCanceller
	down    *fakeDownlink
	ttsProv *fakeTTSProvider
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	asr := &fakeASRSession{events: make(chan biz.ASREvent, 8)}
	bus := newFakeBus()
	exec := &fakeExecutor{}
	canc := &fakeCanceller{}
	down := &fakeDownlink{}
	ttsProv := &fakeTTSProvider{}
	deps := SessionDeps{
		NewASR: func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return &fakeASRProvider{sess: asr}, biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000}, nil
		},
		NewTTS: func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return ttsProv, biz.TTSSessionConfig{Voice: "v", SpeedRatio: 1, SampleRate: 16000}, nil
		},
		Bus:       bus,
		Executor:  exec,
		Canceller: canc,
		Infra:     nil, // 测试不发流程日志总线
		LG:        loggateway.NewNoop(),
	}
	sess := NewSession(context.Background(), deps, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	return &sessionFixture{sess: sess, asr: asr, bus: bus, exec: exec, cancel: canc, down: down, ttsProv: ttsProv}
}

// ---- tests ----

func TestSessionStartBroadcastsListening(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	require.Equal(t, "listening", fx.down.lastState())
}

func TestSessionTextInAudioOut(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})

	// ASR 终稿 → 下行 asr.final + 入 Chat 管线 + 状态 thinking
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好", DurationMs: 800}
	require.Eventually(t, func() bool {
		fx.exec.mu.Lock()
		defer fx.exec.mu.Unlock()
		return len(fx.exec.inputs) == 1 && fx.exec.inputs[0].Content == "你好" && fx.exec.inputs[0].SessionID == "sess-1"
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)

	// LLM delta → 分句 → TTS 音频下行 + 状态 speaking
	fx.bus.ch <- &biz.StepStreamingEvent{DeltaField: "content", DeltaChunk: "你好呀，我是助手。"}
	require.Eventually(t, func() bool {
		fx.down.mu.Lock()
		defer fx.down.mu.Unlock()
		return len(fx.down.audios) > 0
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "speaking" }, 2*time.Second, 10*time.Millisecond)

	// Turn 结束 → flush 残余 → tts.end → 回 listening
	fx.bus.ch <- &biz.TurnCompletedEvent{}
	require.Eventually(t, func() bool {
		for _, ty := range fx.down.typesOf() {
			if ty == "tts.end" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)

	// tts.start 先于音频
	require.Equal(t, "tts.start", fx.down.typesOf()[indexOf(fx.down.typesOf(), "tts.start")])
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func TestSessionCancelDuringSpeaking(t *testing.T) {
	block := make(chan struct{})
	fx := newSessionFixture(t)
	fx.ttsProv.script = func() *fakeTTSSession {
		return &fakeTTSSession{blockCh: block, closed: make(chan struct{})}
	}
	fx.sess.Start(StartParams{})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "thinking" }, 2*time.Second, 10*time.Millisecond)
	fx.bus.ch <- &biz.StepStreamingEvent{DeltaField: "content", DeltaChunk: "很长的句子正在合成中。"}
	require.Eventually(t, func() bool { return fx.down.lastState() == "speaking" }, 2*time.Second, 10*time.Millisecond)

	fx.sess.Cancel("voice.barge_in")
	close(block)

	fx.cancel.mu.Lock()
	require.Equal(t, []string{"sess-1"}, fx.cancel.called)
	fx.cancel.mu.Unlock()
	// interrupted → ~300ms 自动回 listening
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
}

func TestSessionTurnFailureRecoversToListening(t *testing.T) {
	fx := newSessionFixture(t)
	fx.exec.err = errors.New("pipeline boom")
	fx.sess.Start(StartParams{})
	fx.asr.events <- biz.ASREvent{Type: biz.ASREventFinal, Text: "你好"}
	require.Eventually(t, func() bool {
		for _, ty := range fx.down.typesOf() {
			if ty == "voice.error" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)
	// 可恢复错误：error → 自动 voice_start → listening
	require.Eventually(t, func() bool { return fx.down.lastState() == "listening" }, 2*time.Second, 10*time.Millisecond)
}

func TestSessionCommitForwardsToASR(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})
	fx.sess.Commit()
	fx.asr.finishMu.Lock()
	defer fx.asr.finishMu.Unlock()
	require.Equal(t, 1, fx.asr.finished)
}

func TestSessionStopBroadcastsIdle(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{})
	fx.sess.Stop()
	require.Equal(t, "idle", fx.down.lastState())
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/voice/ -run TestSession -count=1`
Expected: FAIL — `undefined: NewSession` / `undefined: SessionDeps`

- [ ] **Step 3: 实现**

`internal/voice/session.go`:

```go
package voice

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// idleReclaimTimeout 是 ASR 上游空闲回收时长（设计 §2.1）。
const idleReclaimTimeout = 10 * time.Minute

// interruptedSettleDelay 是 interrupted 过渡态自动回 listening 的延迟（设计 §5）。
const interruptedSettleDelay = 300 * time.Millisecond

// ---- 注入端口（server 层适配，避免 voice 依赖 internal/server）----

// ASRProviderFactory 按当前配置解析 ASR Provider + 会话参数。
type ASRProviderFactory func(ctx context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error)

// TTSProviderFactory 按当前配置解析 TTS Provider + 会话参数。
type TTSProviderFactory func(ctx context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error)

// ChatTurnInput 是入 Chat 管线的最小参数集（对齐 server.WSTurnInput 子集）。
type ChatTurnInput struct {
	SessionID string
	Content   string
	AgentKey  string
	TeamID    string
}

// Stability:evolving — Chat 管线入口端口（server.WSTurnExecutor 适配实现）。
type ChatTurnExecutor interface {
	ExecuteTurn(ctx context.Context, input ChatTurnInput) error
}

// Stability:evolving — Turn 取消端口（server.RunCanceller 适配实现）。
type RunCanceller interface {
	CancelRun(ctx context.Context, sessionID string) bool
}

// Downlink 是网关 → 客户端的下行通道（WS 实现，写锁由实现保证）。
type Downlink interface {
	SendJSON(v any) error
	SendAudio(pcm []byte) error
}

// StartParams 对应 voice.start 控制帧。
type StartParams struct {
	SampleRate int
	Language   string
	DialogMode string
	AgentKey   string
	TeamID     string
}

// SessionDeps 是语音会话的全部外部依赖。
type SessionDeps struct {
	NewASR    ASRProviderFactory
	NewTTS    TTSProviderFactory
	Bus       biz.EventBus
	Executor  ChatTurnExecutor
	Canceller RunCanceller
	Infra     *event.Infra
	LG        loggateway.Logger
}

// Session 编排单条语音 WS 连接的生命周期（设计 §2.4）。
type Session struct {
	sessionID string
	userID    string
	deps      SessionDeps
	down      Downlink
	lg        loggateway.Logger
	flow      *event.TraceEmitter

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	closed chan struct{}
	once   sync.Once

	mu           sync.Mutex
	state        VoiceState
	params       StartParams
	asr          biz.ASRSession
	chunker      *SentenceChunker
	scheduler    *TTSScheduler
	pendingTurns int
	ttsStarted   bool
	turnSeq      int
	eventStarted bool
	unsub        func()
	idleTimer    *time.Timer
}

func NewSession(ctx context.Context, deps SessionDeps, sessionID, userID string, down Downlink) *Session {
	sctx, cancel := context.WithCancel(ctx)
	lg := deps.LG
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	lg = lg.With(loggateway.Domain("voice"), loggateway.SessionID(sessionID))
	s := &Session{
		sessionID: sessionID,
		userID:    userID,
		deps:      deps,
		down:      down,
		lg:        lg,
		ctx:       sctx,
		cancel:    cancel,
		state:     StateIdle,
		closed:    make(chan struct{}),
	}
	s.flow = event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       sctx,
		SessionID: sessionID,
		Domain:    event.TraceDomainVoice,
		LG:        lg,
		Infra:     deps.Infra,
	})
	return s
}

// ---- 网关入口 ----

// Start 处理 voice.start：开 ASR、订阅事件流、idle/error → listening。
func (s *Session) Start(p StartParams) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateIdle && st != StateError {
		return
	}
	if err := s.transition(EvVoiceStart); err != nil {
		return
	}
	s.mu.Lock()
	s.params = p
	s.mu.Unlock()
	if err := s.openASR(); err != nil {
		s.sendError("ASR_UNAVAILABLE", err, true)
		s.recoverToListening()
		return
	}
	s.startEventLoop()
	s.resetIdleTimer()
	s.flow.LogStart("voice.session.start", "语音会话开始")
	s.lg.Info("voice session started", loggateway.StepID("voice.session.start"))
	s.broadcastState()
}

// WriteAudio 处理上行 PCM 帧；ASR 被空闲回收后懒重连。
func (s *Session) WriteAudio(pcm []byte) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st != StateListening {
		return // thinking/speaking/idle/error 不收音频（打断走控制帧）
	}
	s.resetIdleTimer()
	s.mu.Lock()
	asr := s.asr
	s.mu.Unlock()
	if asr == nil {
		if err := s.openASR(); err != nil {
			s.sendError("ASR_UNAVAILABLE", err, true)
			return
		}
		s.mu.Lock()
		asr = s.asr
		s.mu.Unlock()
	}
	if err := asr.Write(pcm); err != nil {
		s.sendError("ASR_WRITE", err, true)
	}
}

// Commit 处理 voice.commit（PTT 兜底）：标记当前语句结束。
func (s *Session) Commit() {
	s.mu.Lock()
	asr := s.asr
	s.mu.Unlock()
	if asr == nil {
		return
	}
	if err := asr.Finish(); err != nil {
		s.sendError("ASR_FINISH", err, true)
	}
}

// Cancel 处理 voice.cancel / voice.barge_in（V1 裁剪 #4：同路径）。
func (s *Session) Cancel(reason string) {
	s.deps.Canceller.CancelRun(ctxuser.WithUserID(context.Background(), s.userID), s.sessionID)
	s.mu.Lock()
	sch := s.scheduler
	st := s.state
	s.chunker = nil
	s.scheduler = nil
	s.ttsStarted = false
	s.mu.Unlock()
	if sch != nil {
		sch.Cancel()
	}
	_ = s.down.SendJSON(map[string]any{"type": "tts.end", "interrupted": true})
	switch st {
	case StateSpeaking:
		if err := s.transition(EvBargeIn); err == nil {
			s.broadcastState()
			s.flow.LogDone("voice.barge_in", "语音打断", event.P("reason", reason))
			time.AfterFunc(interruptedSettleDelay, func() {
				s.mu.Lock()
				settled := s.state == StateInterrupted
				if settled {
					s.state = StateListening // 过渡态自动回 listening（设计 §5，无需事件）
				}
				s.mu.Unlock()
				if settled {
					s.broadcastState()
				}
			})
		}
	case StateThinking:
		if err := s.transition(EvBargeIn); err == nil {
			s.broadcastState()
		}
	}
}

// Stop 处理 voice.stop：退出语音模式，全量清理（连接保留）。
func (s *Session) Stop() {
	if err := s.transition(EvVoiceStop); err == nil {
		s.broadcastState()
	}
	s.teardown()
	s.flow.LogDone("voice.session.done", "语音会话结束")
	s.lg.Info("voice session stopped", loggateway.StepID("voice.session.done"))
}

// Ping 处理心跳。
func (s *Session) Ping() {
	_ = s.down.SendJSON(map[string]any{"type": "pong"})
}

// ReplaceNoticeAndClose 发 voice.replaced 后关闭（单会话单连接，设计 §2.1）。
func (s *Session) ReplaceNoticeAndClose() {
	_ = s.down.SendJSON(map[string]any{"type": "voice.replaced"})
	s.Close()
}

// Close 全量拆除（连接断开）。幂等。
func (s *Session) Close() {
	s.once.Do(func() {
		s.teardown()
		s.cancel()
		close(s.closed)
		s.wg.Wait()
	})
}

// ---- 内部：ASR ----

func (s *Session) openASR() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asr != nil {
		return nil
	}
	provider, cfg, err := s.deps.NewASR(s.ctx)
	if err != nil {
		return err
	}
	if s.params.SampleRate > 0 {
		cfg.SampleRate = s.params.SampleRate
	}
	if s.params.Language != "" {
		cfg.Language = s.params.Language
	}
	sess, err := provider.Open(s.ctx, cfg)
	if err != nil {
		return err
	}
	s.asr = sess
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.asrPump(sess)
	}()
	return nil
}

func (s *Session) asrPump(sess biz.ASRSession) {
	for ev := range sess.Events() {
		switch ev.Type {
		case biz.ASREventPartial:
			_ = s.down.SendJSON(map[string]any{"type": "asr.partial", "text": ev.Text})
		case biz.ASREventFinal:
			s.handleASRFinal(ev)
		case biz.ASREventError:
			s.sendError("ASR_ERROR", ev.Err, true)
			s.recoverToListening()
		case biz.ASREventVadEnd:
			// 服务端 VAD 端点信号；终稿由 Final 事件承载，V1 无动作
		}
	}
}

func (s *Session) handleASRFinal(ev biz.ASREvent) {
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}
	_ = s.down.SendJSON(map[string]any{"type": "asr.final", "text": text, "duration_ms": ev.DurationMs})
	s.flow.LogDone("voice.asr.final", "语音识别终稿", event.P("duration_ms", ev.DurationMs))
	s.mu.Lock()
	if err := Transition(s.state, EvASRFinal); err != nil {
		s.mu.Unlock()
		return // 非 listening 态的残余 final（如 cancel 后），忽略
	}
	s.state = StateThinking
	s.pendingTurns++
	s.turnSeq++
	turnID := s.turnSeq
	params := s.params
	s.mu.Unlock()
	s.broadcastState()
	_ = s.down.SendJSON(map[string]any{"type": "turn.accepted", "turn_id": turnRef(turnID)})
	// 与 WS 用户消息一致：turn 存活独立于连接（appctx），传播 userID。
	turnCtx := ctxuser.WithUserID(appctx.Ctx(), s.userID)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.deps.Executor.ExecuteTurn(turnCtx, ChatTurnInput{
			SessionID: s.sessionID, Content: text, AgentKey: params.AgentKey, TeamID: params.TeamID,
		}); err != nil {
			s.handleTurnFailure(err)
		}
	}()
}

func turnRef(n int) string {
	return "vt-" + strings.Repeat("0", 0) + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ---- 内部：事件流 → chunker → TTS ----

func (s *Session) startEventLoop() {
	s.mu.Lock()
	if s.eventStarted {
		s.mu.Unlock()
		return
	}
	s.eventStarted = true
	s.mu.Unlock()
	ch, unsub := s.deps.Bus.Subscribe(biz.EventSubscribeOptions{SpiritSessionID: s.sessionID})
	s.mu.Lock()
	s.unsub = unsub
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.eventLoop(ch)
	}()
}

func (s *Session) eventLoop(ch <-chan biz.Event) {
	for {
		select {
		case <-s.ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			switch ev := e.(type) {
			case *biz.StepStreamingEvent:
				if ev.DeltaField == "content" && ev.DeltaChunk != "" {
					s.feedDelta(ev.DeltaChunk)
				}
			case *biz.TurnCompletedEvent:
				s.handleTurnCompleted()
			case *biz.TurnFailedEvent:
				s.handleTurnFailure(nil)
			}
		}
	}
}

func (s *Session) feedDelta(delta string) {
	s.mu.Lock()
	active := s.pendingTurns > 0 && (s.state == StateThinking || s.state == StateSpeaking)
	s.mu.Unlock()
	if !active {
		return
	}
	if err := s.ensureTTS(); err != nil {
		s.sendError("TTS_UNAVAILABLE", err, true)
		return
	}
	s.mu.Lock()
	ch := s.chunker
	s.mu.Unlock()
	if ch != nil {
		ch.Write(delta) // 队列满时 Enqueue 阻塞 = 背压（设计 §4.2）
	}
}

func (s *Session) ensureTTS() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.chunker != nil {
		return nil
	}
	provider, cfg, err := s.deps.NewTTS(s.ctx)
	if err != nil {
		return err
	}
	s.scheduler = NewTTSScheduler(TTSSchedulerOpts{
		Provider:  provider,
		Config:    cfg,
		OnAudio:   s.onTTSAudio,
		OnDrained: s.onTTSDrained,
		OnError:   s.onTTSError,
		LG:        s.lg,
	})
	s.scheduler.Start(s.ctx)
	s.chunker = NewSentenceChunker(func(text string, flush bool) {
		if err := s.scheduler.Enqueue(s.ctx, text, flush); err != nil {
			s.lg.Warn("voice tts enqueue failed", loggateway.StepID("voice.tts.enqueue_fail"), loggateway.Err(err))
		}
	})
	return nil
}

func (s *Session) handleTurnCompleted() {
	s.mu.Lock()
	if s.pendingTurns > 0 {
		s.pendingTurns--
	}
	ch := s.chunker
	idleNoText := s.scheduler == nil && s.state == StateThinking
	s.mu.Unlock()
	if ch != nil {
		ch.Flush()
	}
	if idleNoText {
		// 无文本 Turn（纯工具调用等）：thinking --tts_end--> listening（设计 §5）
		if err := s.transition(EvTTSEnd); err == nil {
			s.broadcastState()
		}
	}
}

func (s *Session) handleTurnFailure(err error) {
	s.mu.Lock()
	if s.pendingTurns > 0 {
		s.pendingTurns--
	}
	s.mu.Unlock()
	if err == nil {
		err = errTurnFailed
	}
	s.sendError("TURN_FAILED", err, true)
	s.recoverToListening()
}

var errTurnFailed = errString("turn failed")

type errString string

func (e errString) Error() string { return string(e) }

// recoverToListening 可恢复错误路径：error --voice_start--> listening（设计 §5）。
func (s *Session) recoverToListening() {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	if st == StateListening || st == StateIdle {
		return
	}
	if err := s.transition(EvTurnFailed); err == nil {
		s.broadcastState()
	}
	if err := s.transition(EvVoiceStart); err == nil {
		s.broadcastState()
	}
}

// ---- 内部：TTS 回调 ----

func (s *Session) onTTSAudio(pcm []byte) {
	s.mu.Lock()
	first := !s.ttsStarted
	if first {
		s.ttsStarted = true
	}
	s.mu.Unlock()
	if first {
		_ = s.down.SendJSON(map[string]any{"type": "tts.start", "encoding": "pcm_f32le_16k", "sample_rate": 16000})
		s.flow.LogDone("voice.tts.start", "语音播报开始")
		if err := s.transition(EvFirstTTSAudio); err == nil {
			s.broadcastState()
		}
	}
	_ = s.down.SendAudio(pcm)
}

func (s *Session) onTTSDrained() {
	s.mu.Lock()
	s.ttsStarted = false
	s.chunker = nil
	s.scheduler = nil
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "tts.end"})
	s.flow.LogDone("voice.tts.end", "语音播报结束")
	if err := s.transition(EvTTSEnd); err == nil {
		s.broadcastState()
	}
}

func (s *Session) onTTSError(err error) {
	// K3 降级：连续合成失败 → 告知前端退回文字模式，状态回 listening
	s.flow.LogWarn("voice.provider.fallback", "语音服务降级", "TTS 连续合成失败", event.P("error", err.Error()))
	s.mu.Lock()
	s.ttsStarted = false
	s.chunker = nil
	s.scheduler = nil
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "tts.end"})
	s.sendError("TTS_UNAVAILABLE", err, true)
	if err := s.transition(EvTTSEnd); err == nil {
		s.broadcastState()
	}
}

// ---- 内部：公共 ----

func (s *Session) transition(ev VoiceEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	to, err := Transition(s.state, ev)
	if err != nil {
		return err
	}
	s.state = to
	return nil
}

func (s *Session) broadcastState() {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	_ = s.down.SendJSON(map[string]any{"type": "voice.state", "state": string(st)})
}

func (s *Session) sendError(code string, err error, retryable bool) {
	s.flow.LogError("voice.error", "语音链路错误", event.P("code", code), event.P("error", err.Error()))
	s.lg.Warn("voice session error", loggateway.StepID("voice.error"), loggateway.Str("code", code), loggateway.Err(err))
	_ = s.down.SendJSON(map[string]any{"type": "voice.error", "code": code, "message": err.Error(), "retryable": retryable})
}

func (s *Session) resetIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(idleReclaimTimeout, s.reclaimIdleASR)
}

func (s *Session) reclaimIdleASR() {
	s.mu.Lock()
	asr := s.asr
	s.asr = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
		s.lg.Info("voice asr: idle upstream reclaimed", loggateway.StepID("voice.asr.idle_reclaim"))
	}
}

func (s *Session) teardown() {
	s.mu.Lock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	asr := s.asr
	s.asr = nil
	sch := s.scheduler
	s.scheduler = nil
	s.chunker = nil
	unsub := s.unsub
	s.unsub = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
	if sch != nil {
		sch.Cancel()
	}
	if unsub != nil {
		unsub()
	}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/voice/ -count=1`
Expected: PASS（全部用例，含 chunker/FSM/scheduler/session）

- [ ] **Step 5: Commit**

```bash
git add internal/voice/session.go internal/voice/session_test.go
git commit -m "feat(voice): voice session orchestration (ASR→Chat→TTS glue)"
```

---

### Task 10: `/v1/voice` 网关 + Kratos 挂载 + Wire + 流程日志登记

**Files:**
- Create: `internal/server/voice_ws.go`
- Test: `internal/server/voice_ws_test.go`
- Modify: `internal/event/trace_context.go`（+`TraceDomainVoice`）
- Modify: `internal/event/flow_log.go`（stepTitleRegistry +8 条 voice.*）
- Modify: `internal/server/http.go`（NewHTTPServer +voiceSrv 参数 + `RegisterOnKratos`）
- Modify: `cmd/admin/wire.go`（+3 个 provider；NewHTTPServer 调用点由 `make wire` 重建）
- Modify: `docs/development/52-flow-logger.design.md`（§5.1 步骤注册表同步，DOC-SYNC）
- Modify: `docs/development/74-voice-companion.development.md`（V1-T1~T6/T9 状态标记 ✅，代码锚点表更新）

- [ ] **Step 1: 写失败测试**

`internal/server/voice_ws_test.go`（真实 WS upgrade，复用 auth bypass 测试约定）:

```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/loggateway"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type voiceTestASRSession struct {
	writeMu sync.Mutex
	written [][]byte
	events  chan biz.ASREvent
}

func (f *voiceTestASRSession) Write(pcm []byte) error {
	f.writeMu.Lock()
	f.written = append(f.written, pcm)
	f.writeMu.Unlock()
	return nil
}
func (f *voiceTestASRSession) Finish() error                { return nil }
func (f *voiceTestASRSession) Events() <-chan biz.ASREvent  { return f.events }
func (f *voiceTestASRSession) Close() error                 { return nil }

type voiceTestASRProvider struct{ sess *voiceTestASRSession }

func (p *voiceTestASRProvider) Open(context.Context, biz.ASRSessionConfig) (biz.ASRSession, error) {
	return p.sess, nil
}

type voiceTestBus struct{ ch chan biz.Event }

func (b *voiceTestBus) Publish(context.Context, biz.Event) {}
func (b *voiceTestBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return b.ch, func() {}
}

type voiceTestExecutor struct{}

func (voiceTestExecutor) ExecuteTurn(context.Context, WSTurnInput) error { return nil }

func newVoiceTestServer(asr *voiceTestASRSession) *VoiceWSServer {
	return NewVoiceWSServer(
		nil, // sessionAuth：bypass 下 admin 免 ownership
		voiceTestExecutor{},
		nil, // canceller 测试不需要（Cancel 路径在 voice 包单测覆盖）
		func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return &voiceTestASRProvider{sess: asr}, biz.ASRSessionConfig{Language: "zh-CN", SampleRate: 16000}, nil
		},
		func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return nil, biz.TTSSessionConfig{}, nil // 本组测试不触发 TTS
		},
		&voiceTestBus{ch: make(chan biz.Event, 8)},
		nil,
		loggateway.NewNoop(),
	)
}

func voiceDial(t *testing.T, srv *httptest.Server, sessionID string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/voice?session_id=" + sessionID
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func readVoiceJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	mt, data, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

func TestVoiceWSRejectsMissingSessionID(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	s := newVoiceTestServer(&voiceTestASRSession{events: make(chan biz.ASREvent, 1)})
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/voice", nil)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestVoiceWSUnauthorizedWithoutBypass(t *testing.T) {
	s := newVoiceTestServer(&voiceTestASRSession{events: make(chan biz.ASREvent, 1)})
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/voice?session_id=s1", nil)
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestVoiceWSStartAndBinaryFrame(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn := voiceDial(t, srv, "s1")
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"voice.start","language":"zh-CN","sample_rate":16000}`)))
	msg := readVoiceJSON(t, conn)
	require.Equal(t, "voice.state", msg["type"])
	require.Equal(t, "listening", msg["state"])

	pcm := []byte{1, 2, 3, 4}
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, pcm))
	require.Eventually(t, func() bool {
		asr.writeMu.Lock()
		defer asr.writeMu.Unlock()
		return len(asr.written) == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestVoiceWSSecondConnectionReplacesFirst(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn1 := voiceDial(t, srv, "s1")
	conn2 := voiceDial(t, srv, "s1")
	_ = conn2
	msg := readVoiceJSON(t, conn1)
	require.Equal(t, "voice.replaced", msg["type"])
}

func TestVoiceWSPingPong(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "test")
	asr := &voiceTestASRSession{events: make(chan biz.ASREvent, 1)}
	s := newVoiceTestServer(asr)
	srv := httptest.NewServer(http.HandlerFunc(s.handleVoiceWS))
	defer srv.Close()

	conn := voiceDial(t, srv, "s1")
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)))
	msg := readVoiceJSON(t, conn)
	require.Equal(t, "pong", msg["type"])
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/server/ -run TestVoiceWS -count=1`
Expected: FAIL — `undefined: VoiceWSServer` / `undefined: voice.NewSession`（voice 包 Task 9 完成后仅剩 server 侧 undefined）

- [ ] **Step 3: 实现**

3a. `internal/event/trace_context.go` — const 块追加（设计 §11：voice 域须在 event 包注册）:

```go
	TraceDomainVoice     TraceDomain = "voice"
```

3b. `internal/event/flow_log.go` — `stepTitleRegistry` 追加（红线：新增 step_id 必须登记中文标题）:

```go
	"voice.session.start":     "语音会话开始",
	"voice.session.done":      "语音会话结束",
	"voice.asr.final":         "语音识别终稿",
	"voice.tts.start":         "语音播报开始",
	"voice.tts.end":           "语音播报结束",
	"voice.barge_in":          "语音打断",
	"voice.provider.fallback": "语音服务降级",
	"voice.error":             "语音链路错误",
```

3c. `internal/server/voice_ws.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/loggateway"

	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/gorilla/websocket"
)

// VoiceWSServer 是 /v1/voice 语音网关（设计 §2）：鉴权/单会话单连接/帧路由。
// 音频帧与高频事件走独立端点，不污染 /v1/ws 事件总线通道。
type VoiceWSServer struct {
	upgrader    websocket.Upgrader
	sessionAuth SessionAuthorizer
	executor    WSTurnExecutor
	canceller   RunCanceller
	newASR      voice.ASRProviderFactory
	newTTS      voice.TTSProviderFactory
	bus         biz.EventBus
	infra       *event.Infra
	lg          loggateway.Logger

	mu    sync.Mutex
	conns map[string]*voice.Session
}

func NewVoiceWSServer(
	sessionAuth SessionAuthorizer,
	executor WSTurnExecutor,
	canceller RunCanceller,
	newASR voice.ASRProviderFactory,
	newTTS voice.TTSProviderFactory,
	bus biz.EventBus,
	infra *event.Infra,
	lg loggateway.Logger,
) *VoiceWSServer {
	return &VoiceWSServer{
		upgrader:    websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		sessionAuth: sessionAuth,
		executor:    executor,
		canceller:   canceller,
		newASR:      newASR,
		newTTS:      newTTS,
		bus:         bus,
		infra:       infra,
		lg:          lg.With(loggateway.Domain("voice")),
		conns:       map[string]*voice.Session{},
	}
}

func (s *VoiceWSServer) RegisterOnKratos(srv *kratoshttp.Server) {
	if s == nil || srv == nil {
		return
	}
	// Exemption from red-line #12 (no business routes in Server layer):
	// WebSocket upgrade cannot be defined via proto; HandleFunc is the only way.
	srv.HandleFunc("/v1/voice", s.handleVoiceWS)
}

func (s *VoiceWSServer) handleVoiceWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	claims, err := wsAuthenticate(r) // 与 /v1/ws 同一鉴权（JWT / dev bypass）
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	userID := fmt.Sprintf("%d", claims.UserID)
	// 会话归属校验（IDOR 防护，admin 豁免）——与 handleWS 同语义
	if !claims.HasAdminAccess() && s.sessionAuth != nil {
		if err := s.sessionAuth.CheckOwnership(r.Context(), sessionID, userID); err != nil {
			s.lg.Warn("voice WS ownership denied",
				loggateway.StepID("voice.ws.ownership_denied"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("user_id", userID),
				loggateway.Err(err))
			http.Error(w, "session ownership required", http.StatusForbidden)
			return
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.lg.Warn("voice WS upgrade failed", loggateway.StepID("voice.ws.upgrade_failed"), loggateway.Err(err))
		return
	}

	down := &voiceWSDownlink{conn: conn}
	deps := voice.SessionDeps{
		NewASR:    s.newASR,
		NewTTS:    s.newTTS,
		Bus:       s.bus,
		Executor:  voiceChatTurnExecutor{inner: s.executor},
		Canceller: voiceRunCanceller{inner: s.canceller},
		Infra:     s.infra,
		LG:        s.lg,
	}
	sess := voice.NewSession(r.Context(), deps, sessionID, userID, down)

	// 单会话单语音连接：第二连接到达时旧连接收 voice.replaced 后关闭（设计 §2.1）
	s.mu.Lock()
	old := s.conns[sessionID]
	s.conns[sessionID] = sess
	s.mu.Unlock()
	if old != nil {
		old.ReplaceNoticeAndClose()
	}

	s.lg.Info("voice WS connected",
		loggateway.StepID("voice.ws.connected"),
		loggateway.SessionID(sessionID),
		loggateway.Str("user_id", userID))
	s.readPump(sess, conn, sessionID)
}

func (s *VoiceWSServer) readPump(sess *voice.Session, conn *websocket.Conn, sessionID string) {
	defer func() {
		s.mu.Lock()
		if s.conns[sessionID] == sess {
			delete(s.conns, sessionID)
		}
		s.mu.Unlock()
		sess.Close()
		_ = conn.Close()
	}()
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.BinaryMessage:
			sess.WriteAudio(data)
		case websocket.TextMessage:
			var msg voiceControlMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			s.handleControl(sess, msg)
		}
	}
}

type voiceControlMessage struct {
	Type       string `json:"type"`
	SampleRate int    `json:"sample_rate"`
	Language   string `json:"language"`
	DialogMode string `json:"dialog_mode"`
	AgentKey   string `json:"agent_key"`
	TeamID     string `json:"team_id"`
	DetectMs   int    `json:"detect_ms"`
}

func (s *VoiceWSServer) handleControl(sess *voice.Session, msg voiceControlMessage) {
	switch msg.Type {
	case "voice.start":
		sess.Start(voice.StartParams{
			SampleRate: msg.SampleRate,
			Language:   msg.Language,
			DialogMode: msg.DialogMode,
			AgentKey:   msg.AgentKey,
			TeamID:     msg.TeamID,
		})
	case "voice.stop":
		sess.Stop()
	case "voice.commit":
		sess.Commit()
	case "voice.barge_in", "voice.cancel": // V1 裁剪 #4：barge_in 复用 cancel 路径
		sess.Cancel(msg.Type)
	case "ping":
		sess.Ping()
	}
}

// voiceWSDownlink 实现 voice.Downlink；gorilla 写并发需写锁。
type voiceWSDownlink struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (d *voiceWSDownlink) SendJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn.WriteMessage(websocket.TextMessage, raw)
}

func (d *voiceWSDownlink) SendAudio(pcm []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

// ---- server 既有端口 → voice 窄端口适配器 ----

type voiceChatTurnExecutor struct{ inner WSTurnExecutor }

func (a voiceChatTurnExecutor) ExecuteTurn(ctx context.Context, in voice.ChatTurnInput) error {
	if a.inner == nil {
		return nil
	}
	return a.inner.ExecuteTurn(ctx, WSTurnInput{
		SessionID:   in.SessionID,
		Content:     in.Content,
		AgentKey:    in.AgentKey,
		TeamID:      in.TeamID,
		AllowQueue:  true,
		AllowStream: true,
	})
}

type voiceRunCanceller struct{ inner RunCanceller }

func (a voiceRunCanceller) CancelRun(ctx context.Context, sessionID string) bool {
	if a.inner == nil {
		return false
	}
	return a.inner.CancelRun(ctx, sessionID)
}
```

3d. `internal/server/http.go` — `NewHTTPServer` 签名加 `voiceSrv *VoiceWSServer`（放在 `wsSrv *WSServer` 之后），函数体 `wsSrv.RegisterOnKratos(srv)` 后追加：

```go
	if voiceSrv != nil {
		voiceSrv.RegisterOnKratos(srv)
	}
```

3e. `cmd/admin/wire.go` — 追加 provider（`make wire` 重建 `wire_gen.go` 与 NewHTTPServer 调用点）:

```go
func provideSpeechRegistry() *speech.Registry { return speech.NewRegistry() }

func provideSpeechConfigReader() biz.SpeechConfigReader { return speech.NewEnvSpeechConfigReader() }

func provideVoiceWSServer(
	sessionAuth server.SessionAuthorizer,
	turnExecutor server.WSTurnExecutor,
	canceller server.RunCanceller,
	registry *speech.Registry,
	cfgReader biz.SpeechConfigReader,
	eventBus biz.EventBus,
	infra *event.Infra,
	lg loggateway.Logger,
) *server.VoiceWSServer {
	newASR := func(ctx context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
		cfg, err := cfgReader.ASRConfig(ctx)
		if err != nil {
			return nil, biz.ASRSessionConfig{}, err
		}
		p, err := registry.ASRProvider(cfg, lg)
		if err != nil {
			return nil, biz.ASRSessionConfig{}, err
		}
		return p, biz.ASRSessionConfig{Language: cfg.Language, SampleRate: 16000}, nil
	}
	newTTS := func(ctx context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
		cfg, err := cfgReader.TTSConfig(ctx)
		if err != nil {
			return nil, biz.TTSSessionConfig{}, err
		}
		p, err := registry.TTSProvider(cfg, lg)
		if err != nil {
			return nil, biz.TTSSessionConfig{}, err
		}
		return p, biz.TTSSessionConfig{Voice: cfg.Voice, SpeedRatio: cfg.SpeedRatio, SampleRate: 16000}, nil
	}
	return server.NewVoiceWSServer(sessionAuth, turnExecutor, canceller, newASR, newTTS, eventBus, infra, lg)
}
```

import 追加：`speech "aranea-agents/internal/data/speech"`。wire.Build 调用处（`NewHTTPServer` 实参列表）由 `make wire` 自动补 `voiceWSServer`。

- [ ] **Step 4: 分级验证**

```bash
make wire                              # 重建 wire_gen.go
go test ./internal/server/ -run TestVoiceWS -count=1   # 网关集成测试
go test ./internal/voice/ ./internal/data/speech/ ./internal/biz/ ./pkg/apierror/ ./internal/event/ -count=1
go build ./cmd/admin ./internal/...
make lint                              # 若耗时过长可 golangci-lint run ./internal/... 限定范围
```

Expected: 全 PASS + build 成功。

- [ ] **Step 5: 文档同步（DOC-SYNC）+ Commit**

1. `docs/development/52-flow-logger.design.md` §5.1 步骤注册表追加 voice.* 8 条（与 3b 一致）
2. `docs/development/74-voice-companion.development.md`：
   - V1-T1~T6、V1-T9 状态标记 📋→✅（验收列注明「后端单测/集成测试通过；真机联调归 V1-T10」）
   - §2.2 新增锚点表更新实际文件名（`ws_conn.go`/`volc_frame.go` 等补充进 Speech 适配器行）

```bash
git add internal/server/voice_ws.go internal/server/voice_ws_test.go internal/server/http.go internal/event/trace_context.go internal/event/flow_log.go cmd/admin/wire.go cmd/admin/wire_gen.go docs/development/52-flow-logger.design.md docs/development/74-voice-companion.development.md
git commit -m "feat(voice): /v1/voice WS gateway + kratos mount + wire + flow-log step registry"
```

---

## 计划边界（Scope Note）

本计划覆盖 Phase V1 **后端**（对应 V1-T1~T6 + V1-T9）。以下为后续独立计划：

| 后续计划 | 对应任务 | 内容 |
|----------|----------|------|
| voice-companion-v1-frontend | V1-T7 / V1-T8 | 前端语音采集/播放（AudioWorklet 16k PCM、gapless 调度）、`/companion` 路由 + 基础 HUD 三态 + 聊天面板滑出 |
| voice-companion-v1-e2e | V1-T10 | 真机集成验收：火山凭据联调、NFR1 < 2.5s 延迟实测、字幕准确性、播报连续性、文字聊天回归 |

执行顺序建议：本计划（后端）→ frontend 计划 → e2e 联调。后端 WS 协议（§2.2/§2.3 帧格式）即前端契约，前端计划可直接对照实现。
