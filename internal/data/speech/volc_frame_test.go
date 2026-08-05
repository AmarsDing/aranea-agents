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
