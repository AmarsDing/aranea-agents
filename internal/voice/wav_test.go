package voice

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeWAVHeader(t *testing.T) {
	pcm := make([]byte, 3200) // 100ms @16kHz s16le mono
	out := EncodeWAV(pcm, 16000)
	require.Len(t, out, 44+len(pcm))
	require.Equal(t, "RIFF", string(out[0:4]))
	require.Equal(t, "WAVE", string(out[8:12]))
	require.Equal(t, "fmt ", string(out[12:16]))
	require.Equal(t, uint32(16), binary.LittleEndian.Uint32(out[16:20])) // fmt chunk size
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(out[20:22]))  // PCM format
	require.Equal(t, uint16(1), binary.LittleEndian.Uint16(out[22:24]))  // mono
	require.Equal(t, uint32(16000), binary.LittleEndian.Uint32(out[24:28]))
	require.Equal(t, uint32(32000), binary.LittleEndian.Uint32(out[28:32])) // byte rate = rate*2
	require.Equal(t, uint16(2), binary.LittleEndian.Uint16(out[32:34]))     // block align
	require.Equal(t, uint16(16), binary.LittleEndian.Uint16(out[34:36]))    // bits
	require.Equal(t, "data", string(out[36:40]))
	require.Equal(t, uint32(len(pcm)), binary.LittleEndian.Uint32(out[40:44]))
	require.Equal(t, uint32(36+len(pcm)), binary.LittleEndian.Uint32(out[4:8])) // riff size
	require.Equal(t, pcm, out[44:])
}

func TestEncodeWAVEmptyPCM(t *testing.T) {
	out := EncodeWAV(nil, 16000)
	require.Len(t, out, 44)
	require.Equal(t, uint32(0), binary.LittleEndian.Uint32(out[40:44]))
	require.Equal(t, uint32(36), binary.LittleEndian.Uint32(out[4:8]))
}

func TestEncodeWAVSampleRate(t *testing.T) {
	out := EncodeWAV([]byte{1, 2}, 8000)
	require.Equal(t, uint32(8000), binary.LittleEndian.Uint32(out[24:28]))
	require.Equal(t, uint32(16000), binary.LittleEndian.Uint32(out[28:32]))
}
