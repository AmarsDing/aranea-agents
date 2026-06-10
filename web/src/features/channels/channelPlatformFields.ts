// Re-export from domain layer for backward compatibility.
// New code should import from domain/channel directly.
export {
  type ChannelPlatformFieldKind,
  type ChannelPlatformFieldBind,
  type ChannelPlatformField,
  type ChannelPlatformSection,
  type ChannelFieldHelp,
  buildPlatformSections,
  visibleFields,
  catalogCredentialFields,
} from '../../domain/channel/channelPlatformFields';
