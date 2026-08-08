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

// TTS V3 单向流式下行事件帧布局（火山公开协议）：
// header(4B) + event(4B BE) + sessionID len(4B BE) + sessionID + payload len(4B BE) + payload。
func TestFrameRoundTripEventJSON(t *testing.T) {
	in := volcFrame{
		msgType:   volcMsgFullServerResponse,
		flags:     volcFlagWithEvent,
		event:     350, // TTSSentenceStart
		sessionID: "sess-abc",
		json:      true,
		payload:   []byte(`{"res_params":{"text":"你好"}}`),
	}
	data, err := marshalVolcFrame(in, false)
	require.NoError(t, err)
	require.Equal(t, byte(0x11), data[0])
	require.Equal(t, byte(0x94), data[1], "full server response + event flag")

	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, volcMsgFullServerResponse, out.msgType)
	require.Equal(t, int32(350), out.event)
	require.Equal(t, "sess-abc", out.sessionID)
	require.JSONEq(t, `{"res_params":{"text":"你好"}}`, string(out.payload))
}

func TestFrameRoundTripEventAudio(t *testing.T) {
	pcm := bytes.Repeat([]byte{0x01, 0x02}, 160)
	in := volcFrame{
		msgType:   volcMsgAudioOnlyResponse,
		flags:     volcFlagWithEvent,
		event:     352, // TTSResponse
		sessionID: "sess-abc",
		payload:   pcm,
	}
	data, err := marshalVolcFrame(in, false)
	require.NoError(t, err)
	require.Equal(t, byte(0xB4), data[1], "audio-only response + event flag")

	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, int32(352), out.event)
	require.Equal(t, "sess-abc", out.sessionID)
	require.Equal(t, pcm, out.payload)
}

// 错误响应帧布局（SAUC/TTS V3 同族）：header(4B, msgType=0xF flags=0)
// + error code(4B BE) + payload len(4B BE) + JSON payload。
func TestFrameRoundTripErrorWithCode(t *testing.T) {
	in := volcFrame{
		msgType: volcMsgError,
		flags:   volcFlagNone,
		errCode: 450000001,
		json:    true,
		payload: []byte(`{"code":450000001,"message":"bad speaker"}`),
	}
	data, err := marshalVolcFrame(in, false)
	require.NoError(t, err)
	require.Equal(t, byte(0xF0), data[1], "error frame, no flags")

	out, err := unmarshalVolcFrame(bytes.NewReader(data))
	require.NoError(t, err)
	require.Equal(t, volcMsgError, out.msgType)
	require.Equal(t, uint32(450000001), out.errCode)
	require.JSONEq(t, `{"code":450000001,"message":"bad speaker"}`, string(out.payload))
}
