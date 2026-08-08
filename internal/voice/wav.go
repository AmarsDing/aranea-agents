package voice

import "encoding/binary"

// wavHeaderLen 是标准 44 字节 RIFF/WAVE 头长度。
const wavHeaderLen = 44

// EncodeWAV 将 s16le mono PCM 封装为 WAV（RIFF）字节流（M74 V2-T6 语音留档）。
// pcm 为空时返回仅含头部的合法空音频。
func EncodeWAV(pcm []byte, sampleRate int) []byte {
	out := make([]byte, wavHeaderLen+len(pcm))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+len(pcm)))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16) // fmt chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM
	binary.LittleEndian.PutUint16(out[22:24], 1)  // mono
	binary.LittleEndian.PutUint32(out[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(sampleRate*2)) // byte rate: s16le mono
	binary.LittleEndian.PutUint16(out[32:34], 2)                    // block align
	binary.LittleEndian.PutUint16(out[34:36], 16)                   // bits per sample
	copy(out[36:40], "data")
	binary.LittleEndian.PutUint32(out[40:44], uint32(len(pcm)))
	copy(out[wavHeaderLen:], pcm)
	return out
}
