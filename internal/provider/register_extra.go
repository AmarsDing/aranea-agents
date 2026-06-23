package provider

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcbedrock "trpc.group/trpc-go/trpc-agent-go/model/bedrock"
	trpchuggingface "trpc.group/trpc-go/trpc-agent-go/model/huggingface"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
)

// bedrockConfigLoadTimeout 限制 AWS SDK 加载默认配置（含 IMDS 元数据查询）的最大耗时，
// 避免在 EC2 环境下因 IMDS 不可达导致 provider 构造阻塞数十秒。
const bedrockConfigLoadTimeout = 10 * time.Second

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
	// 使用带超时的 context，避免 IMDS 元数据查询阻塞。
	// 框架 Options 不携带请求 ctx，这里用 Background + 超时兜底。
	ctx, cancel := context.WithTimeout(context.Background(), bedrockConfigLoadTimeout)
	defer cancel()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
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
