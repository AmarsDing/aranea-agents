package biz

// credentialSchemaFor builds JSON-Schema-like credential metadata for catalog API consumers.
func credentialSchemaFor(channelType string) map[string]any {
	props := credentialProperties(channelType)
	if len(props) == 0 {
		return map[string]any{
			"type":     "object",
			"required": requiredCredentials(channelType),
		}
	}
	return map[string]any{
		"type":       "object",
		"required":   requiredCredentials(channelType),
		"properties": props,
	}
}

func credentialProperties(channelType string) map[string]any {
	password := "password"
	switch channelType {
	case "telegram":
		return map[string]any{
			"bot_token": propField("telegram_bot_token", password, true),
		}
	case "discord":
		return map[string]any{
			"bot_token": propField("discord_bot_token", password, true),
		}
	case "feishu":
		return map[string]any{
			"app_secret": propField("lark_app_secret", password, true),
		}
	case "dingtalk":
		return map[string]any{
			"client_secret": propField("ding_client_secret", password, false),
			"secret":        propField("secret", password, true),
		}
	case "slack":
		return map[string]any{
			"bot_token":      propField("slack_bot_token", password, true),
			"app_token":      propField("slack_app_token", password, false),
			"signing_secret": propField("signing_secret", password, true),
		}
	case "wechat":
		return map[string]any{
			"app_secret":         propField("wechat_app_secret", password, true),
			"token":              propField("wechat_token", password, false),
			"encoding_aes_key":   propField("wechat_encoding_aes_key", password, false),
		}
	case "wecom", "wecom-app":
		return map[string]any{
			"token":            propField("com_wechat_token", password, true),
			"encoding_aes_key": propField("com_wechat_encoding_aes_key", password, false),
			"corp_secret":      propField("com_wechat_secret", password, false),
		}
	case "qq":
		return map[string]any{
			"app_secret": propField("qq_app_secret", password, true),
		}
	case "personal_qq":
		return map[string]any{
			"receive_token": propField("qq_one_bot_receive_token", password, true),
			"send_token":    propField("qq_one_bot_send_token", password, true),
		}
	default:
		return nil
	}
}

func propField(title, format string, required bool) map[string]any {
	m := map[string]any{
		"type":  "string",
		"title": title,
	}
	if format != "" {
		m["format"] = format
	}
	if required {
		m["x-required"] = true
	}
	return m
}
