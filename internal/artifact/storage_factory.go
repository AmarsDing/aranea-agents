package artifact

import (
	"context"
	"os"

	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpccos "trpc.group/trpc-go/trpc-agent-go/artifact/cos"
	trpcs3 "trpc.group/trpc-go/trpc-agent-go/artifact/s3"
)

type StorageBackend string

const (
	StorageBackendLocal StorageBackend = "local"
	StorageBackendS3    StorageBackend = "s3"
	StorageBackendCOS   StorageBackend = "cos"
)

type StorageConfig struct {
	Backend      StorageBackend
	S3Bucket     string
	S3Endpoint   string
	S3Region     string
	S3AccessKey  string
	S3SecretKey  string
	S3PathStyle  bool
	COSBucketURL string
	COSSecretID  string
	COSSecretKey string
}

func NewArtifactService(ctx context.Context, cfg StorageConfig) (trpcartifact.Service, error) {
	switch cfg.Backend {
	case StorageBackendS3:
		return newS3Service(ctx, cfg)
	case StorageBackendCOS:
		return newCOSService(cfg)
	case StorageBackendLocal, "":
		return nil, nil
	default:
		return nil, nil
	}
}

func newS3Service(ctx context.Context, cfg StorageConfig) (trpcartifact.Service, error) {
	opts := []trpcs3.Option{
		trpcs3.WithRegion(envOr(cfg.S3Region, "us-east-1")),
	}
	if cfg.S3Endpoint != "" {
		opts = append(opts, trpcs3.WithEndpoint(cfg.S3Endpoint))
	}
	accessKey := envOr(cfg.S3AccessKey, os.Getenv("ARTIFACT_S3_ACCESS_KEY"))
	secretKey := envOr(cfg.S3SecretKey, os.Getenv("ARTIFACT_S3_SECRET_KEY"))
	if accessKey != "" && secretKey != "" {
		opts = append(opts, trpcs3.WithCredentials(accessKey, secretKey))
	}
	if cfg.S3PathStyle {
		opts = append(opts, trpcs3.WithPathStyle(true))
	}
	return trpcs3.NewService(ctx, cfg.S3Bucket, opts...)
}

func newCOSService(cfg StorageConfig) (trpcartifact.Service, error) {
	secretID := envOr(cfg.COSSecretID, os.Getenv("ARTIFACT_COS_SECRET_ID"))
	secretKey := envOr(cfg.COSSecretKey, os.Getenv("ARTIFACT_COS_SECRET_KEY"))
	opts := []trpccos.Option{}
	if secretID != "" && secretKey != "" {
		opts = append(opts, trpccos.WithSecretID(secretID), trpccos.WithSecretKey(secretKey))
	}
	return trpccos.NewService("aranea", cfg.COSBucketURL, opts...)
}

func envOr(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}
