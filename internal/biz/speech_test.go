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
