/**
 * 语音伴侣共享类型（M74 设计 §7.2）。
 *
 * 展示组件经本文件引类型（红线 #12），不得从 api.ts / store / composable 引。
 */

/** 语音状态机（服务端 voice.state 广播镜像，前端不做本地推测）。 */
export type VoiceState = 'idle' | 'listening' | 'thinking' | 'speaking' | 'interrupted' | 'error';

/** 语音通道错误（voice.error 帧 / 本地采集错误）。 */
export type VoiceError = {
  code: string;
  message: string;
  retryable: boolean;
};
