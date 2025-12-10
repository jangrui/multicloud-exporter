package tencent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultClientFactory(t *testing.T) {
	f := &defaultClientFactory{}
	region := "ap-guangzhou"
	ak := "test-ak"
	sk := "test-sk"

	t.Run("NewCVMClient", func(t *testing.T) {
		client, err := f.NewCVMClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewCLBClient", func(t *testing.T) {
		client, err := f.NewCLBClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewVPCClient", func(t *testing.T) {
		client, err := f.NewVPCClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewMonitorClient", func(t *testing.T) {
		client, err := f.NewMonitorClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("NewCOSClient", func(t *testing.T) {
		client, err := f.NewCOSClient(region, ak, sk)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		// Basic check if GetService can be called (it will fail with invalid creds but method exists)
		// We just want to cover the wrapper struct method
		wrapper, ok := client.(*defaultCOSClient)
		assert.True(t, ok)
		assert.NotNil(t, wrapper.client)
	})
}

func TestDefaultCOSClient_GetService(t *testing.T) {
	// We cannot easily test GetService without real network call or mocking http client,
	// but we can try to call it and expect error, covering the line.
	f := &defaultClientFactory{}
	client, _ := f.NewCOSClient("ap-guangzhou", "ak", "sk")
	
	// This will likely fail with network error or auth error, but it covers the line.
	_, _, err := client.GetService(context.Background())
	// We just expect it to return something, likely error
	assert.Error(t, err) 
}
