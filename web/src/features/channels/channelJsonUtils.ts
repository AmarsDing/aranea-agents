// Re-export from domain layer for backward compatibility.
// New code should import from domain/channel directly.
export { parseJSON } from '../../domain/channel/channelJsonUtils';
