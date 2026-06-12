package biz

// credentialSchemaFor builds JSON-Schema-like credential metadata for catalog API consumers.
func credentialSchemaFor(channelType string) map[string]any {
	spec, ok := defaultRegistry.Get(channelType)
	if !ok {
		return map[string]any{
			"type":     "object",
			"required": []string{},
		}
	}
	return buildCredentialSchema(spec)
}

func credentialProperties(channelType string) map[string]any {
	spec, ok := defaultRegistry.Get(channelType)
	if !ok {
		return nil
	}
	if len(spec.CredentialProps) == 0 {
		return nil
	}
	result := make(map[string]any, len(spec.CredentialProps))
	for _, p := range spec.CredentialProps {
		result[p.Key] = p.ToMap()
	}
	return result
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
