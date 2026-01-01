package utils

import (
	"net"
	"net/http"
	"time"
)

// NewHTTPClient 创建一个配置良好的 HTTP 客户端
// 超时时间、连接池大小都经过优化，适合云服务 API 调用
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			// 代理设置从环境变量读取 (HTTP_PROXY, HTTPS_PROXY, NO_PROXY)
			Proxy: http.ProxyFromEnvironment,

			// 连接池配置
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     0, // 无限制

			// 空闲连接超时
			IdleConnTimeout:   90 * time.Second,
			DisableKeepAlives: false,

			// TLS 握手超时
			TLSHandshakeTimeout: 10 * time.Second,

			// 连接超时
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,

			// 响应头超时
			ResponseHeaderTimeout: 10 * time.Second,

			// 期望继续读取响应体的时间（服务器接收到请求后）
			ExpectContinueTimeout: 1 * time.Second,

			// 强制尝试 HTTP/2
			ForceAttemptHTTP2: true,
		},
	}
}

// NewHTTPClientWithTimeout 创建自定义超时的 HTTP 客户端
func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	client := NewHTTPClient()
	client.Timeout = timeout
	return client
}
