// Domain facade for channel utilities.
// Components and features should import from here instead of features/channels to maintain layering.

export { parseJSON } from './channelJsonUtils';
export { defaultChannelAvatarKey } from './channelIconUi';
export {
  normalizePublicOrigin,
  resolvePublicWebhookOrigin,
  isLocalhostOrigin,
  buildChannelWebhookURL,
} from './publicWebhookOrigin';
export {
  type ChannelRoutingAgent,
  type ChannelRoutingTeam,
  type ChannelRoutingTargetType,
  channelRoutingTargetToggleOptions,
  channelAgentSelectOptions,
  channelTeamSelectOptions,
  resolveChannelAgentSelectValue,
  pickDefaultAgentId,
  inferRoutingTargetType,
  isChannelRoutingValid,
} from './channelRoutingUtils';
export {
  LONG_TASK_FORM_KEYS,
  type LongTaskFormKey,
  LONG_TASK_NUMERIC_KEYS,
  CHANNEL_LONG_TASK_DEFAULTS,
  TURN_TIMEOUT_OPTIONS,
  FIRST_BYTE_TIMEOUT_OPTIONS,
  PROGRESS_QUIET_OPTIONS,
  applyLongTaskFormDefaults,
  isLongTaskFormKey,
} from './channelLongTaskDefaults';
export {
  type ChannelPlatformFieldKind,
  type ChannelPlatformFieldBind,
  type ChannelPlatformField,
  type ChannelPlatformSection,
  type ChannelFieldHelp,
  buildPlatformSections,
  visibleFields,
  catalogCredentialFields,
} from './channelPlatformFields';
