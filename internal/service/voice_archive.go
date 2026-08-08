package service

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/voice"
	"aranea-agents/pkg/loggateway"
)

// VoiceAudioArchiver 实现 voice.AudioArchiver（M74 V2-T6 语音留档）：
// 开关（speech.archive_user_audio）开启时，将语句 WAV 落 Artifact（audio/wav，
// PreviewKind=audio），返回展示态附件引用。开关关闭/读取失败按零值 Ref 降级
// （K3），存储错误透传给调用方降级，均不阻断 Turn。
type VoiceAudioArchiver struct {
	saver biz.ArtifactSaver
	cfg   biz.SpeechConfigReader
	lg    loggateway.Logger
	seq   atomic.Uint64
}

func NewVoiceAudioArchiver(saver biz.ArtifactSaver, cfg biz.SpeechConfigReader, lg loggateway.Logger) *VoiceAudioArchiver {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VoiceAudioArchiver{saver: saver, cfg: cfg, lg: lg.With(loggateway.Domain("voice"))}
}

var _ voice.AudioArchiver = (*VoiceAudioArchiver)(nil)

// SaveUtteranceAudio 实现 voice.AudioArchiver。
func (a *VoiceAudioArchiver) SaveUtteranceAudio(ctx context.Context, sessionID string, wav []byte, durationMs int) (artifactbiz.Ref, error) {
	if a == nil || a.saver == nil || a.cfg == nil {
		return artifactbiz.Ref{}, nil
	}
	on, err := a.cfg.ArchiveUserAudio(ctx)
	if err != nil {
		// 开关读取失败按关闭降级（K3）：配置故障不应阻断语音对话。
		a.lg.Warn("voice archive switch read failed, skip archiving",
			loggateway.StepID("voice.archive.degraded"), loggateway.Err(err))
		return artifactbiz.Ref{}, nil
	}
	if !on {
		return artifactbiz.Ref{}, nil
	}
	// 文件名逐次唯一（时间戳 + 序号；低分辨率时钟下序号兜底），避免同
	// session+name 产生版本堆叠。
	now := time.Now().UTC()
	name := fmt.Sprintf("voice-%s-%06d-%d.wav", now.Format("20060102-150405"), now.Nanosecond()/1000, a.seq.Add(1))
	saved, err := a.saver.Save(ctx, sessionID, name, "audio/wav", wav)
	if err != nil {
		return artifactbiz.Ref{}, err
	}
	return artifactbiz.Ref{
		ID:       saved.ID,
		Name:     strings.TrimSpace(saved.Name),
		MimeType: saved.MimeType,
		Size:     saved.Size,
	}, nil
}
