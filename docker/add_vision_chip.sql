UPDATE llm_provider_models
SET config_json = jsonb_set(
    config_json::jsonb,
    '{capability_chips}',
    '[{"key":"vision","label":"Vision","source":"catalog"}]'::jsonb
)::text
WHERE model_key = 'ollama:qwen2.5vl:7b';
SELECT model_key, (config_json::jsonb)->'capability_chips' AS chips
FROM llm_provider_models
WHERE model_key = 'ollama:qwen2.5vl:7b';
