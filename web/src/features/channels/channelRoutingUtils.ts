// Re-export from domain layer for backward compatibility.
// New code should import from domain/channel directly.
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
} from '../../domain/channel/channelRoutingUtils';
