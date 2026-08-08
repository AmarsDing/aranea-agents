package voice

import (
	"context"

	artifactbiz "aranea-agents/internal/biz/artifact"
)

// maxUtterancePCMBytes 单语句留档 PCM 缓冲上限（8 MiB ≈ 4.4 分钟 @16kHz s16le mono）。
// 超出后停止追加（留档音频截断），防止长语句内存膨胀；封装 WAV 后仍低于
// artifact.MaxUploadBytes（10 MiB）上限。
const maxUtterancePCMBytes = 8 << 20

// Stability:evolving — 语音留档端口（M74 V2-T6）；SessionDeps.Archiver 为 nil 时关闭留档。
// 实现内部负责开关判定（speech.archive_user_audio）：开关关闭返回零值 Ref + nil 错误。
type AudioArchiver interface {
	// SaveUtteranceAudio 保存一条 ASR 终稿语句的 WAV 音频，返回展示态附件引用
	// （合并进用户消息 options_json.attachments 供 UI 回放，不进 LLM 上下文）。
	// 返回零值 Ref（ID 为空）表示未留档（开关关闭）；错误由调用方 Warn 降级，
	// 不得阻断 Turn 派发（K3）。
	SaveUtteranceAudio(ctx context.Context, sessionID string, wav []byte, durationMs int) (artifactbiz.Ref, error)
}
