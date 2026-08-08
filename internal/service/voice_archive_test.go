package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type fakeArtifactSaver struct {
	calls []saveCall
	saved artifactbiz.Artifact
	err   error
}

type saveCall struct {
	sessionID string
	name      string
	mimeType  string
	size      int
}

func (f *fakeArtifactSaver) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (artifactbiz.Artifact, error) {
	f.calls = append(f.calls, saveCall{sessionID: sessionID, name: name, mimeType: mimeType, size: len(data)})
	if f.err != nil {
		return artifactbiz.Artifact{}, f.err
	}
	saved := f.saved
	if saved.ID == "" {
		saved.ID = "art-auto"
	}
	if saved.Name == "" {
		saved.Name = name
	}
	if saved.MimeType == "" {
		saved.MimeType = mimeType
	}
	if saved.Size == 0 {
		saved.Size = int64(len(data))
	}
	return saved, nil
}

type fakeSpeechConfigReader struct {
	archiveOn  bool
	archiveErr error
}

func (f *fakeSpeechConfigReader) ASRConfig(context.Context) (biz.ASRProviderConfig, error) {
	return biz.ASRProviderConfig{}, nil
}
func (f *fakeSpeechConfigReader) TTSConfig(context.Context) (biz.TTSProviderConfig, error) {
	return biz.TTSProviderConfig{}, nil
}
func (f *fakeSpeechConfigReader) ArchiveUserAudio(context.Context) (bool, error) {
	return f.archiveOn, f.archiveErr
}

// ---- V2-T6：VoiceAudioArchiver ----

// 开关开启：WAV 落 Artifact（audio/wav），返回附件引用。
func TestVoiceAudioArchiverSavesWhenSwitchOn(t *testing.T) {
	saver := &fakeArtifactSaver{saved: artifactbiz.Artifact{ID: "art-1"}}
	cfg := &fakeSpeechConfigReader{archiveOn: true}
	a := NewVoiceAudioArchiver(saver, cfg, loggateway.NewNoop())

	wav := make([]byte, 44+3200)
	ref, err := a.SaveUtteranceAudio(context.Background(), "sess-1", wav, 1200)
	require.NoError(t, err)
	require.Equal(t, "art-1", ref.ID)
	require.Equal(t, "audio/wav", ref.MimeType)
	require.Equal(t, int64(len(wav)), ref.Size)
	require.NotEmpty(t, ref.Name)

	require.Len(t, saver.calls, 1)
	require.Equal(t, "sess-1", saver.calls[0].sessionID)
	require.Equal(t, "audio/wav", saver.calls[0].mimeType)
	require.Equal(t, len(wav), saver.calls[0].size)
}

// 开关关闭：不落盘，返回零值 Ref（调用方据此跳过附件合并）。
func TestVoiceAudioArchiverSkipsWhenSwitchOff(t *testing.T) {
	saver := &fakeArtifactSaver{}
	cfg := &fakeSpeechConfigReader{archiveOn: false}
	a := NewVoiceAudioArchiver(saver, cfg, loggateway.NewNoop())

	ref, err := a.SaveUtteranceAudio(context.Background(), "sess-1", []byte{1, 2}, 100)
	require.NoError(t, err)
	require.Empty(t, ref.ID)
	require.Empty(t, saver.calls)
}

// 开关读取失败：按关闭降级（K3），不返错、不落盘。
func TestVoiceAudioArchiverSwitchReadFailureDegrades(t *testing.T) {
	saver := &fakeArtifactSaver{}
	cfg := &fakeSpeechConfigReader{archiveErr: errors.New("settings store down")}
	a := NewVoiceAudioArchiver(saver, cfg, loggateway.NewNoop())

	ref, err := a.SaveUtteranceAudio(context.Background(), "sess-1", []byte{1, 2}, 100)
	require.NoError(t, err)
	require.Empty(t, ref.ID)
	require.Empty(t, saver.calls)
}

// 存储失败：错误透传（由 voice.Session 降级为无附件 Turn）。
func TestVoiceAudioArchiverSaveErrorPropagates(t *testing.T) {
	saver := &fakeArtifactSaver{err: errors.New("disk full")}
	cfg := &fakeSpeechConfigReader{archiveOn: true}
	a := NewVoiceAudioArchiver(saver, cfg, loggateway.NewNoop())

	_, err := a.SaveUtteranceAudio(context.Background(), "sess-1", []byte{1, 2}, 100)
	require.Error(t, err)
}

// nil 依赖安全：未接线时等价关闭。
func TestVoiceAudioArchiverNilDepsSafe(t *testing.T) {
	a := NewVoiceAudioArchiver(nil, nil, loggateway.NewNoop())
	ref, err := a.SaveUtteranceAudio(context.Background(), "sess-1", []byte{1, 2}, 100)
	require.NoError(t, err)
	require.Empty(t, ref.ID)
}

// 留档文件名：.wav 后缀且逐次唯一（避免同 session+name 产生版本堆叠）。
func TestVoiceAudioArchiverNameUniquePerUtterance(t *testing.T) {
	saver := &fakeArtifactSaver{}
	cfg := &fakeSpeechConfigReader{archiveOn: true}
	a := NewVoiceAudioArchiver(saver, cfg, loggateway.NewNoop())

	for i := 0; i < 3; i++ {
		_, err := a.SaveUtteranceAudio(context.Background(), "sess-1", []byte{1}, 50)
		require.NoError(t, err)
	}
	require.Len(t, saver.calls, 3)
	names := map[string]struct{}{}
	for _, c := range saver.calls {
		require.Contains(t, c.name, ".wav")
		names[c.name] = struct{}{}
	}
	require.Len(t, names, 3, "留档文件名必须逐次唯一: %v", saver.calls)
}

// ---- V2-T6：prepareTurnUserOptions 语音元数据盖章 ----

// 语音 Turn：options_json 盖 input_modality/asr_provider/asr_duration_ms，
// 留档引用合并进 attachments（展示态）；不进 LLM 附件链路（AttachmentIDs 为空）。
func TestPrepareTurnUserOptionsVoiceMeta(t *testing.T) {
	o := &ChatOrchestrator{}
	input := biz.TurnInput{
		SessionID: "sess-1",
		Content:   "打开微信",
		Voice: &biz.VoiceTurnMeta{
			ASRProvider: "volcengine",
			DurationMs:  1200,
			Archive: &artifactbiz.Ref{
				ID: "art-1", Name: "voice-x.wav", MimeType: "audio/wav", Size: 3264,
			},
		},
	}
	admit := turnAdmissionResult{runID: "run-1", dialogMode: "chat", provider: "p", model: "m"}
	out, err := o.prepareTurnUserOptions(context.Background(), input, biz.Agent{ID: "a1", AgentKey: "k"}, admit, nil, nil, biz.Session{})
	require.NoError(t, err)

	var opts map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &opts))
	require.Equal(t, "voice", opts["input_modality"])
	require.Equal(t, "volcengine", opts["asr_provider"])
	require.Equal(t, float64(1200), opts["asr_duration_ms"])

	atts, ok := opts["attachments"].([]any)
	require.True(t, ok, "attachments missing: %v", opts)
	require.Len(t, atts, 1)
	att := atts[0].(map[string]any)
	require.Equal(t, "art-1", att["id"])
	require.Equal(t, "audio/wav", att["mime_type"])
}

// 非语音 Turn（Voice=nil）：options_json 无语音键。
func TestPrepareTurnUserOptionsNoVoiceMeta(t *testing.T) {
	o := &ChatOrchestrator{}
	input := biz.TurnInput{SessionID: "sess-1", Content: "hi"}
	admit := turnAdmissionResult{runID: "run-1", dialogMode: "chat", provider: "p", model: "m"}
	out, err := o.prepareTurnUserOptions(context.Background(), input, biz.Agent{ID: "a1"}, admit, nil, nil, biz.Session{})
	require.NoError(t, err)

	var opts map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &opts))
	_, ok := opts["input_modality"]
	require.False(t, ok)
	_, ok = opts["attachments"]
	require.False(t, ok)
}
