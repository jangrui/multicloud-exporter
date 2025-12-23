package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ClientFactory interface {
	NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error)
	NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error)
	NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error)
	NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error)
	NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error)
}

type defaultClientFactory struct{}

func (f *defaultClientFactory) loadCfg(ctx context.Context, region, ak, sk string) (aws.Config, error) {
	return awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(ak, sk, "")),
	)
}

type CWAPI interface {
	GetMetricData(ctx context.Context, params *cloudwatch.GetMetricDataInput, optFns ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
}

type S3API interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
}

func (f *defaultClientFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return cloudwatch.NewFromConfig(cfg), nil
}

func (f *defaultClientFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

func (f *defaultClientFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return elasticloadbalancing.NewFromConfig(cfg), nil
}

func (f *defaultClientFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return elasticloadbalancingv2.NewFromConfig(cfg), nil
}

func (f *defaultClientFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	cfg, err := f.loadCfg(ctx, region, ak, sk)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(cfg), nil
}
