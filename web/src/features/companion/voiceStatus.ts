/**
 * 语音服务可用性探测（M74 V2-T8 差距2：麦克风置灰门控数据源）。
 *
 * GET /v1/voice/status —— 后端以 DB-first/env-fallback 配置读取 + Validate
 * 判定 ASR/TTS 是否可用（与 voice.start 的 openASR 判定同源）。
 */

import { kratosApi } from '../../services/axiosHandler';

type VoiceStatusResponse = {
  asr_available?: boolean;
  tts_available?: boolean;
};

/**
 * 拉取语音可用性。返回 null = 探测失败（网络/鉴权异常），调用方按「未知」
 * 处理（不置灰麦克风；若实际未配置，点击后由 voice.error 降级条兜底）。
 */
export async function fetchVoiceAvailability(): Promise<boolean | null> {
  try {
    const { data } = await kratosApi.get<VoiceStatusResponse>('/v1/voice/status');
    return data.asr_available === true && data.tts_available === true;
  } catch {
    return null;
  }
}
