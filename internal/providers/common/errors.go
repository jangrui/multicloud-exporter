// Package common 提供云厂商通用的错误处理和重试逻辑
package common

import (
	"fmt"
	"strings"
)

// ErrorClassifier 错误分类器接口
// 所有云厂商的错误分类器都需要实现此接口
type ErrorClassifier interface {
	// Classify 分类错误，返回统一错误状态码
	Classify(err error) string
}

// 统一错误状态码常量
// 这些常量用于标识不同类型的错误，便于统一处理和重试决策
const (
	// ErrorStatusAuth 表示认证错误，通常是由于无效的访问密钥或签名不匹配导致的
	// 此类错误不应重试，应直接返回给调用者
	ErrorStatusAuth = "auth_error"
	// ErrorStatusLimit 表示限流错误，通常是由于 API 调用频率过高导致的
	// 此类错误应该重试，使用指数退避策略
	ErrorStatusLimit = "limit_error"
	// ErrorStatusRegion 表示区域跳过错误，通常是由于不支持的区域或资源不存在导致的
	// 此类错误不应重试，应直接跳过该区域
	ErrorStatusRegion = "region_skip"
	// ErrorStatusNetwork 表示网络错误，通常是由于网络超时或连接失败导致的
	// 此类错误应该重试，使用指数退避策略
	ErrorStatusNetwork = "network_error"
	// ErrorStatusUnknown 表示未知错误，无法明确分类的错误
	ErrorStatusUnknown = "error"
)

// LogUnknownError 记录未知错误到日志（仅用于发现新的错误模式）
func LogUnknownError(err error, provider, api string) string {
	if err == nil {
		return ErrorStatusUnknown
	}
	msg := err.Error()
	status := ErrorStatusUnknown

	// 尝试用各云厂商的错误分类器分类错误
	switch provider {
	case "aliyun":
		status = AliyunClassifier.Classify(err)
	case "tencent":
		status = TencentClassifier.Classify(err)
	case "aws":
		status = AWSClassifier.Classify(err)
	case "huawei":
		status = HuaweiClassifier.Classify(err)
	}

	// 如果仍然是未知错误，记录到日志以便后续分析
	if status == ErrorStatusUnknown {
		fmt.Printf("UNKNOWN ERROR: provider=%s api=%s error=%s\n", provider, api, msg)
	}
	return status
}

// AliyunErrorClassifier 阿里云错误分类器
type AliyunErrorClassifier struct{}

// Classify 分类阿里云错误
func (c *AliyunErrorClassifier) Classify(err error) string {
	if err == nil {
		return ErrorStatusUnknown
	}
	msg := err.Error()
	if strings.Contains(msg, "InvalidAccessKeyId") || strings.Contains(msg, "Forbidden") || strings.Contains(msg, "SignatureDoesNotMatch") {
		return ErrorStatusAuth
	}
	if strings.Contains(msg, "Throttling") || strings.Contains(msg, "flow control") {
		return ErrorStatusLimit
	}
	if strings.Contains(msg, "InvalidRegionId") || strings.Contains(msg, "Unsupported") {
		return ErrorStatusRegion
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "unreachable") || strings.Contains(msg, "Temporary network") {
		return ErrorStatusNetwork
	}
	return ErrorStatusUnknown
}

// TencentErrorClassifier 腾讯云错误分类器
type TencentErrorClassifier struct{}

// Classify 分类腾讯云错误
func (c *TencentErrorClassifier) Classify(err error) string {
	if err == nil {
		return ErrorStatusUnknown
	}
	msg := err.Error()
	if strings.Contains(msg, "AuthFailure") || strings.Contains(msg, "InvalidCredential") {
		return ErrorStatusAuth
	}
	if strings.Contains(msg, "RequestLimitExceeded") {
		return ErrorStatusLimit
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") {
		return ErrorStatusNetwork
	}
	return ErrorStatusUnknown
}

// AWSErrorClassifier AWS 错误分类器
type AWSErrorClassifier struct{}

// Classify 分类 AWS 错误
func (c *AWSErrorClassifier) Classify(err error) string {
	if err == nil {
		return ErrorStatusUnknown
	}
	msg := err.Error()
	if strings.Contains(msg, "ExpiredToken") || strings.Contains(msg, "InvalidClientTokenId") || strings.Contains(msg, "AccessDenied") {
		return ErrorStatusAuth
	}
	if strings.Contains(msg, "Throttling") || strings.Contains(msg, "Rate exceeded") || strings.Contains(msg, "TooManyRequests") {
		return ErrorStatusLimit
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") {
		return ErrorStatusNetwork
	}
	return ErrorStatusUnknown
}

// HuaweiErrorClassifier 华为云错误分类器
type HuaweiErrorClassifier struct{}

// Classify 分类华为云错误
func (c *HuaweiErrorClassifier) Classify(err error) string {
	if err == nil {
		return ErrorStatusUnknown
	}
	msg := err.Error()
	// 认证错误 - 使用精确匹配而非字符串包含
	lowerMsg := strings.ToLower(msg)
	if strings.Contains(lowerMsg, "unauthorized") || strings.Contains(lowerMsg, "authentication failed") ||
		strings.Contains(lowerMsg, "authenticate failed") || strings.Contains(lowerMsg, "invalidaccesskeyid") ||
		strings.Contains(lowerMsg, "invaliddsk") || strings.Contains(lowerMsg, "signaturedoesnotmatch") ||
		strings.Contains(lowerMsg, "verification failed") || strings.Contains(lowerMsg, "access key") && strings.Contains(lowerMsg, "invalid") {
		return ErrorStatusAuth
	}
	// 限流错误 - 华为云特定错误码：APIGW.0308
	if strings.Contains(msg, "APIGW.0308") || strings.Contains(msg, "throttling") ||
		strings.Contains(msg, "too many requests") || strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "429") {
		return ErrorStatusLimit
	}
	// 区域错误 - 区域不存在或不支持
	if strings.Contains(msg, "region not supported") || strings.Contains(msg, "invalid region id") {
		return ErrorStatusRegion
	}
	// 网络错误
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "unreachable") ||
		strings.Contains(msg, "network") || strings.Contains(msg, "connection") ||
		strings.Contains(msg, "temporarily unavailable") {
		return ErrorStatusNetwork
	}
	// 限流错误 - 华为云特定错误码和通用限流关键词
	// APIGW.0308: 华为云 API 网关限流错误码
	// throttling: 通用限流关键词
	// ratelimit: 限流关键词（单词形式）
	// 429: HTTP 限流状态码
	if strings.Contains(msg, "APIGW.0308") || strings.Contains(msg, "throttling") ||
		strings.Contains(msg, "ratelimit") || strings.Contains(msg, "429") ||
		strings.Contains(msg, "TooManyRequests") || strings.Contains(msg, "rate limit") {
		return ErrorStatusLimit
	}
	// 区域错误
	if strings.Contains(msg, "region") || strings.Contains(msg, "endpoint") {
		return ErrorStatusRegion
	}
	// 网络错误
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") ||
		strings.Contains(msg, "connection") {
		return ErrorStatusNetwork
	}
	return ErrorStatusUnknown
}

// 全局错误分类器实例
var (
	AliyunClassifier  = &AliyunErrorClassifier{}
	TencentClassifier = &TencentErrorClassifier{}
	AWSClassifier     = &AWSErrorClassifier{}
	HuaweiClassifier  = &HuaweiErrorClassifier{}
)

// ClassifyAliyunError 分类阿里云错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//
//	err := someAliyunAPI()
//	status := ClassifyAliyunError(err)
//	if status == common.ErrorStatusLimit {
//	    // 处理限流错误
//	}
func ClassifyAliyunError(err error) string {
	return AliyunClassifier.Classify(err)
}

// ClassifyTencentError 分类腾讯云错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//
//	err := someTencentAPI()
//	status := ClassifyTencentError(err)
//	if status == common.ErrorStatusLimit {
//	    // 处理限流错误
//	}
func ClassifyTencentError(err error) string {
	return TencentClassifier.Classify(err)
}

// ClassifyAWSError 分类 AWS 错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//
//	err := someAWSAPI()
//	status := ClassifyAWSError(err)
//	if status == common.ErrorStatusLimit {
//	    // 处理限流错误
//	}
func ClassifyAWSError(err error) string {
	return AWSClassifier.Classify(err)
}

// ClassifyHuaweiError 分类华为云错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//
//	err := someHuaweiAPI()
//	status := ClassifyHuaweiError(err)
//	if status == common.ErrorStatusLimit {
//	    // 处理限流错误
//	}
func ClassifyHuaweiError(err error) string {
	return HuaweiClassifier.Classify(err)
}
