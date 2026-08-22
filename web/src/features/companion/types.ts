/**
 * 语音伴侣共享类型（M74 设计 §7.2）。
 *
 * 展示组件经本文件引类型（红线 #12），不得从 api.ts / store / composable 引。
 */

/** 语音状态机（服务端 voice.state 广播镜像，前端不做本地推测）。V10 +dormant（待命：本地 KWS 监听，ASR 关闭）。 */
export type VoiceState = 'idle' | 'dormant' | 'listening' | 'thinking' | 'speaking' | 'interrupted' | 'error';

/** 语音通道错误（voice.error 帧 / 本地采集错误）。 */
export type VoiceError = {
  code: string;
  message: string;
  retryable: boolean;
};

/**
 * 全息确认卡视图模型（V2-T5）：由 chat v2 Step（kind=confirm）派生，
 * 展示组件只依赖本类型（红线 #12），不接触 v2Types/api。
 */
export type ConfirmCardModel = {
  /** 确认活动 ID（Step.ID，决议回传用）。 */
  id: string;
  /** 所属会话 ID。 */
  sessionId: string;
  /** 工具名（如 client_open_app）。 */
  toolName: string;
  /** 操作目标（从 ToolArgs.target / .url 提取，可能为空）。 */
  target: string;
  /** 确认描述（Step.Content，后端生成的确认提示语）。 */
  description: string;
  /** 参数 pretty JSON（可展开详情；无参数时为空串）。 */
  argsJson: string;
  /** 确认发起时间（ISO；倒计时基准）。 */
  startedAt: string;
  /** 确认来源：`external_coding` 为 M76 编程桥审批中继。 */
  source: string;
  /** 外部编程工具 key（source=external_coding）。 */
  agentKey: string;
  /** 项目名（source=external_coding）。 */
  projectName: string;
};

/** 确认卡决议（展示组件语义动作；由 Page 层映射为 TOOL_CONFIRM_REPLY 令牌）。 */
export type ConfirmDecision = 'approve' | 'deny' | 'always';
