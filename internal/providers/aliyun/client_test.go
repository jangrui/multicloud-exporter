package aliyun

import (
	"testing"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/stretchr/testify/assert"
)

func TestDefaultClientFactory(t *testing.T) {
	f := &defaultClientFactory{}
	region := "cn-hangzhou"
	ak := "test-ak"
	sk := "test-sk"

	t.Run("NewECSClient", func(t *testing.T) {
		client, err := f.NewECSClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewCMSClient", func(t *testing.T) {
		client, err := f.NewCMSClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewSLBClient", func(t *testing.T) {
		client, err := f.NewSLBClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewVPCClient", func(t *testing.T) {
		client, err := f.NewVPCClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewTagClient", func(t *testing.T) {
		client, err := f.NewTagClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewSTSClient", func(t *testing.T) {
		client, err := f.NewSTSClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewOSSClient", func(t *testing.T) {
		client, err := f.NewOSSClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)

		// Verify type
		_, ok := client.(*oss.Client)
		assert.True(t, ok)
	})
}
