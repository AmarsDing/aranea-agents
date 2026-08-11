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
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// TTS V3 单向流式事件码（火山公开协议「单向流式websocket-V3」）。
const (
	volcTTSEventSessionFinished int32 = 152 // 一次完整合成结束
	volcTTSEventSentenceStart   int32 = 350 // 句内容开始（JSON payload）
	volcTTSEventSentenceEnd     int32 = 351 // 句内容结束
	volcTTSEventResponse        int32 = 352 // 音频数据（payload = 原始音频字节）
)

// volcTTSProvider 实现火山 TTS V3 单向流式语音合成
// （wss://openspeech.bytedance.com/api/v3/tts/unidirectional/stream）。
// V1 裁剪 #3：每句一条 WS 连接——TTSSession.Write 恰好调用一次（一句一合成），
// 双工流式/预连接升级归 V3-T5。
//
// 真机校准（2026-08-08）：凭据双模式（X-Api-Key 或 legacy AppKey/AccessKey 对）；
// resource id 决定模型代际（seed-tts-1.0/2.0、volc.service_type.10029 等）；
// 音色必须与模型代际匹配（2.0 音色如 zh_female_vv_uranus_bigtts 仅配 seed-tts-2.0）。
type volcTTSProvider struct {
	cfg  biz.TTSProviderConfig
	dial wsDialer
	lg   loggateway.Logger

	// L5 温连接槽（单槽）：voice.start 预拨 WS 存槽，首句 Write 弹出复用免
	// 握手。P0-D（2026-08-11）：消费后异步补充——补充在句间空隙完成拨号，
	// 后续句/后续轮次首句继续免握手，同步握手彻底移出用户感知关键路径。
	warmMu   sync.Mutex
	warm     wsConn
	released bool // teardown 后抑制补充；迟到的预热连接到达即关（防泄漏）
}

func newVolcTTSProvider(cfg biz.TTSProviderConfig, lg loggateway.Logger) biz.StreamingTTSProvider {
	return &volcTTSProvider{cfg: cfg, dial: gorillaDialer, lg: lg}
}

// PrewarmTTSConn 预拨一条 TTS WS 连接存入温槽（L5，2026-08-11）。
// 幂等：槽位已被占用（含未消费的死亡连接）时跳过；拨号失败仅 Warn 降级（K3），
// 首句 Write 回退新拨，语音链路不受影响。连接鉴权头在拨号时携带，与
// Write 的文本帧参数无关，故温连接对任意会话参数可复用。
func (p *volcTTSProvider) PrewarmTTSConn(ctx context.Context) {
	p.warmMu.Lock()
	if p.warm != nil || p.released {
		p.warmMu.Unlock()
		return
	}
	p.warmMu.Unlock()
	conn, err := p.dialConn(ctx)
	if err != nil {
		p.lg.Warn("volc tts: prewarm dial failed, first sentence falls back to fresh dial (K3)",
			loggateway.Err(err))
		return
	}
	p.warmMu.Lock()
	if p.warm != nil || p.released {
		// 并发预热竞态 / teardown 后迟到：连接直接关闭，单槽语义不放大负载。
		p.warmMu.Unlock()
		_ = conn.Close()
		return
	}
	p.warm = conn
	p.warmMu.Unlock()
}

// ReleaseWarmTTSConn 关闭未消费的温连接（语音会话 teardown 释放），并抑制
// 后续补充。幂等。
func (p *volcTTSProvider) ReleaseWarmTTSConn() {
	p.warmMu.Lock()
	conn := p.warm
	p.warm = nil
	p.released = true
	p.warmMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// popWarm 弹出温连接；P0-D：成功消费后异步补充温槽（未释放时），
// 下一句/下一轮首写继续免握手。
func (p *volcTTSProvider) popWarm() wsConn {
	p.warmMu.Lock()
	conn := p.warm
	p.warm = nil
	replenish := conn != nil && !p.released
	p.warmMu.Unlock()
	if replenish {
		// 红线 #13：异步补充走 safego（panic 恢复）；补充失败仅少一次预热，
		// 下一句回退同步拨号（K3），不影响正确性。
		safego.GoBackground("voice.tts_warm_replenish", func() {
			p.PrewarmTTSConn(context.Background())
		})
	}
	return conn
}

// dialConn 构造鉴权头并拨号（Prewarm 与 Write 共用）。
func (p *volcTTSProvider) dialConn(ctx context.Context) (wsConn, error) {
	header := nethttp.Header{}
	setVolcAuthHeader(header, p.cfg.APIKey, p.cfg.AppKey, p.cfg.AccessKey)
	header.Set("X-Api-Resource-Id", p.cfg.ResourceID)
	header.Set("X-Api-Request-Id", uuid.NewString())
	return p.dial(ctx, p.cfg.Endpoint, header)
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
	p       *volcTTSProvider
	sc      biz.TTSSessionConfig
	audio   chan biz.TTSAudioChunk
	started atomic.Bool
	ended   atomic.Bool
	conn    wsConn
	closeMu sync.Mutex
	closed  bool
}

func (s *volcTTSSession) Write(text string, _ bool) error {
	if s.started.Swap(true) {
		return errors.New("volc tts: Write called twice (V1 一句一连接)")
	}
	body := map[string]any{
		"user": map[string]any{"uid": "aranea"},
		"req_params": map[string]any{
			"text":    text,
			"speaker": s.sc.Voice,
			"audio_params": map[string]any{
				"format":      "pcm",
				"sample_rate": s.sc.SampleRate,
				"speech_rate": volcTTSSpeechRate(s.sc.SpeedRatio),
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	// V3 上行 SendText：full client request，无 event，JSON 不压缩。
	frame, err := marshalVolcFrame(volcFrame{msgType: volcMsgFullClientRequest, flags: volcFlagNone, json: true, payload: raw}, false)
	if err != nil {
		s.failOnce(err)
		return nil
	}
	// L5：优先复用 voice.start 预热的温连接免握手；温连接死亡（服务端 idle
	// 断连）写失败时回退新拨重试一次——最坏情况 = 现状延迟，首句不丢（K3）。
	if conn := s.p.popWarm(); conn != nil {
		if werr := conn.WriteMessage(websocket.BinaryMessage, frame); werr == nil {
			s.setConn(conn)
			go s.readPump(conn)
			return nil
		}
		_ = conn.Close()
		s.p.lg.Warn("volc tts: warm conn write failed, redialing fresh (K3)")
	}
	conn, err := s.p.dialConn(context.Background())
	if err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil // 错误经 Audio() 通道上报
	}
	s.setConn(conn)
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		s.failOnce(apierror.Wrap(err, apierror.CodeUnavailable, "speech"))
		return nil
	}
	go s.readPump(conn)
	return nil
}

// volcTTSSpeechRate 将 biz 语速倍率（1.0=正常）映射为 V3 speech_rate
// （[-50,100]，0=1x，100=2x，-50=0.5x），超界收敛。
func volcTTSSpeechRate(ratio float64) int {
	v := int(math.Round((ratio - 1.0) * 100))
	if v > 100 {
		return 100
	}
	if v < -50 {
		return -50
	}
	return v
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
			if f.event == volcTTSEventResponse && len(f.payload) > 0 {
				s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkData, PCM: pcmS16ToF32(f.payload)}
			}
		case volcMsgFullServerResponse:
			// 350/351 句界事件无需业务动作；152 标记整次合成完成。
			if f.event == volcTTSEventSessionFinished {
				s.finish()
			}
		case volcMsgError:
			s.audio <- biz.TTSAudioChunk{Type: biz.TTSAudioChunkError, Err: apierror.Internal("speech", "volc tts error: %s", formatVolcError(f))}
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
