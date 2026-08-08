package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	"github.com/stretchr/testify/require"
)

func TestASRProviderConfigValidate(t *testing.T) {
	valid := biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Language: "zh-CN"}
	require.NoError(t, valid.Validate())

	missing := valid
	missing.AccessKey = ""
	err := missing.Validate()
	require.Error(t, err)
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "want FAILED_PRECONDITION, got %v", err)

	missingDriver := valid
	missingDriver.Driver = ""
	require.Error(t, missingDriver.Validate())
}

// X-Api-Key 鉴权模式（火山控制台新 API Key）：单 key 即可，无需 AppKey/AccessKey 对。
func TestASRProviderConfigValidateAPIKeyMode(t *testing.T) {
	cfg := biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", APIKey: "ak-1", Language: "zh-CN"}
	require.NoError(t, cfg.Validate())

	none := biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", Language: "zh-CN"}
	require.True(t, apierror.IsCode(none.Validate(), apierror.CodeFailedPrecondition))
}

func TestTTSProviderConfigValidateAPIKeyMode(t *testing.T) {
	cfg := biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", APIKey: "ak-1", Voice: "v", SpeedRatio: 1.0}
	require.NoError(t, cfg.Validate())

	none := biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", Voice: "v", SpeedRatio: 1.0}
	require.True(t, apierror.IsCode(none.Validate(), apierror.CodeFailedPrecondition))
}

func TestTTSProviderConfigValidate(t *testing.T) {
	valid := biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Voice: "v", SpeedRatio: 1.0}
	require.NoError(t, valid.Validate())

	bad := valid
	bad.Voice = ""
	require.True(t, apierror.IsCode(bad.Validate(), apierror.CodeFailedPrecondition))

	badSpeed := valid
	badSpeed.SpeedRatio = 0
	require.True(t, apierror.IsCode(badSpeed.Validate(), apierror.CodeFailedPrecondition))
}
