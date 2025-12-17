package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ClientFactory interface {
	NewCloudWatchClient(ctx context.Context, region, ak, sk string) (*cloudwatch.Client, error)
	NewS3Client(ctx context.Context, region, ak, sk string) (*s3.Client, error)
}

type defaultClientFactory struct{}

func (f *defaultClientFactory) loadCfg(ctx context.Context, region, ak, sk string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(ak, sk, "")),
	)
}

func (f *defaultClientFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (*cloudwatch.Client, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return cloudwatch.NewFromConfig(cfg), nil
}

func (f *defaultClientFactory) NewS3Client(ctx context.Context, region, ak, sk string) (*s3.Client, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}
