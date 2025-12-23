package aws

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multicloud-exporter/internal/config"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

type mockFactory struct {
	newEC2Err error
}

func (m *mockFactory) NewCloudWatchClient(ctx context.Context, region, ak, sk string) (CWAPI, error) {
	return nil, nil
}
func (m *mockFactory) NewS3Client(ctx context.Context, region, ak, sk string) (S3API, error) {
	return nil, nil
}
func (m *mockFactory) NewELBClient(ctx context.Context, region, ak, sk string) (*elasticloadbalancing.Client, error) {
	return nil, nil
}
func (m *mockFactory) NewELBv2Client(ctx context.Context, region, ak, sk string) (*elasticloadbalancingv2.Client, error) {
	return nil, nil
}
func (m *mockFactory) NewEC2Client(ctx context.Context, region, ak, sk string) (*ec2.Client, error) {
	if m.newEC2Err != nil {
		return nil, m.newEC2Err
	}
	return nil, nil
}

func TestGetAllRegions_FallbackOnError(t *testing.T) {
	c := &Collector{
		clientFactory: &mockFactory{newEC2Err: errors.New("boom")},
	}
	acc := config.CloudAccount{AccountID: "test"}
	got := c.getAllRegions(acc)
	want := []string{"us-east-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback regions mismatch: got=%v want=%v", got, want)
	}
}

func TestCollect_UnknownResource_NoPanic(t *testing.T) {
	c := &Collector{clientFactory: &mockFactory{}}
	acc := config.CloudAccount{AccountID: "test", Resources: []string{"unknown_service"}}
	// Should not panic
	c.Collect(acc)
}

func TestCollect_Wildcard_NoConfig_NoPanic(t *testing.T) {
	c := &Collector{clientFactory: &mockFactory{}}
	acc := config.CloudAccount{AccountID: "test", Resources: []string{"*"}}
	c.Collect(acc)
}

func TestGetDefaultResources(t *testing.T) {
	c := &Collector{}
	rd := c.GetDefaultResources()
	if len(rd) != 1 || rd[0] != "s3" {
		t.Fatalf("GetDefaultResources mismatch: %v", rd)
	}
}

func TestDefaultClientFactory_LoadCfg(t *testing.T) {
	f := &defaultClientFactory{}
	_, err := f.loadCfg(context.Background(), "us-east-1", "", "")
	if err != nil {
		t.Fatalf("loadCfg error: %v", err)
	}
}

func TestDefaultClientFactory_NewClients(t *testing.T) {
	f := &defaultClientFactory{}
	ctx := context.Background()
	cw, err := f.NewCloudWatchClient(ctx, "us-east-1", "ak", "sk")
	if err != nil || cw == nil {
		t.Fatalf("NewCloudWatchClient failed: %v", err)
	}
	s3c, err := f.NewS3Client(ctx, "us-east-1", "ak", "sk")
	if err != nil || s3c == nil {
		t.Fatalf("NewS3Client failed: %v", err)
	}
	elb, err := f.NewELBClient(ctx, "us-east-1", "ak", "sk")
	if err != nil || elb == nil {
		t.Fatalf("NewELBClient failed: %v", err)
	}
	elbv2, err := f.NewELBv2Client(ctx, "us-east-1", "ak", "sk")
	if err != nil || elbv2 == nil {
		t.Fatalf("NewELBv2Client failed: %v", err)
	}
	ec2c, err := f.NewEC2Client(ctx, "us-east-1", "ak", "sk")
	if err != nil || ec2c == nil {
		t.Fatalf("NewEC2Client failed: %v", err)
	}
}
