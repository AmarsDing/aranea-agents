package biz

import "encoding/json"

// ChannelStreamingEnabled reports config_json.config.streaming_enabled.
func ChannelStreamingEnabled(configJSON string) bool {
	var env struct {
		Config struct {
			StreamingEnabled bool `json:"streaming_enabled"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(defaultJSON(configJSON)), &env) != nil {
		return false
	}
	return env.Config.StreamingEnabled
}
