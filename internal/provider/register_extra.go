package provider

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/config"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcbedrock "trpc.group/trpc-go/trpc-agent-go/model/bedrock"
	trpchuggingface "trpc.group/trpc-go/trpc-agent-go/model/huggingface"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
)

// RegisterExtraProviders registers non-built-in provider constructors
// (huggingface, bedrock) with the trpc-agent-go provider registry.
// Must be called once during application startup (Wire provider).
func RegisterExtraProviders() {
	trpcprovider.Register("huggingface", huggingfaceProvider)
	trpcprovider.Register("bedrock", bedrockProvider)
}

func huggingfaceProvider(opts *trpcprovider.Options) (trpcmodel.Model, error) {
	var hfOpts []trpchuggingface.Option
	if opts.APIKey != "" {
		hfOpts = append(hfOpts, trpchuggingface.WithAPIKey(opts.APIKey))
	}
	if opts.BaseURL != "" {
		hfOpts = append(hfOpts, trpchuggingface.WithBaseURL(opts.BaseURL))
	}
	if opts.HTTPClientTransport != nil {
		hfOpts = append(hfOpts, trpchuggingface.WithHTTPClient(&http.Client{Transport: opts.HTTPClientTransport}))
	}
	if opts.ChannelBufferSize != nil && *opts.ChannelBufferSize > 0 {
		hfOpts = append(hfOpts, trpchuggingface.WithChannelBufferSize(*opts.ChannelBufferSize))
	}
	if opts.EnableTokenTailoring != nil && *opts.EnableTokenTailoring {
		hfOpts = append(hfOpts, trpchuggingface.WithEnableTokenTailoring(true))
	}
	if opts.MaxInputTokens != nil && *opts.MaxInputTokens > 0 {
		hfOpts = append(hfOpts, trpchuggingface.WithMaxInputTokens(*opts.MaxInputTokens))
	}
	return trpchuggingface.New(opts.ModelName, hfOpts...)
}

func bedrockProvider(opts *trpcprovider.Options) (trpcmodel.Model, error) {
	region := ""
	if opts.ExtraFields != nil {
		if v, ok := opts.ExtraFields["aws_region"].(string); ok {
			region = v
		}
	}
	if region == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	var bedrockOpts []trpcbedrock.Option
	bedrockOpts = append(bedrockOpts, trpcbedrock.WithAWSConfig(cfg))
	if opts.ChannelBufferSize != nil && *opts.ChannelBufferSize > 0 {
		bedrockOpts = append(bedrockOpts, trpcbedrock.WithChannelBufferSize(*opts.ChannelBufferSize))
	}
	return trpcbedrock.New(opts.ModelName, bedrockOpts...), nil
}
