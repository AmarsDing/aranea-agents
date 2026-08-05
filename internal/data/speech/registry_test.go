package speech

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
)

func TestRegistryVolcengineRegistered(t *testing.T) {
	r := NewRegistry()
	asr, err := r.ASRProvider(biz.ASRProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s"}, loggateway.NewNoop())
	require.NoError(t, err)
	require.NotNil(t, asr)
	tts, err := r.TTSProvider(biz.TTSProviderConfig{Driver: "volcengine", Endpoint: "wss://x", AppKey: "k", AccessKey: "s", Voice: "v", SpeedRatio: 1}, loggateway.NewNoop())
	require.NoError(t, err)
	require.NotNil(t, tts)
}

func TestRegistryUnknownDriver(t *testing.T) {
	r := NewRegistry()
	_, err := r.ASRProvider(biz.ASRProviderConfig{Driver: "nope"}, loggateway.NewNoop())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
	_, err = r.TTSProvider(biz.TTSProviderConfig{Driver: "nope"}, loggateway.NewNoop())
	require.True(t, apierror.IsCode(err, apierror.CodeFailedPrecondition), "got %v", err)
}
