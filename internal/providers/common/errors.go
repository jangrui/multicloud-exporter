// Package common 提供云厂商通用的错误处理和重试逻辑
package common

import "strings"

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

// ErrorClassifier 定义错误分类接口
// 实现该接口的类型可以将云厂商特定的错误分类为统一的错误状态码
type ErrorClassifier interface {
	// Classify 将错误分类为统一的错误状态码
	// 参数 err 是要分类的错误
	// 返回值是错误状态码常量（如 ErrorStatusAuth, ErrorStatusLimit 等）
	Classify(err error) string
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

// 全局错误分类器实例
var (
	AliyunClassifier  = &AliyunErrorClassifier{}
	TencentClassifier = &TencentErrorClassifier{}
	AWSClassifier     = &AWSErrorClassifier{}
)

// ClassifyAliyunError 分类阿里云错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//   err := someAliyunAPI()
//   status := ClassifyAliyunError(err)
//   if status == common.ErrorStatusLimit {
//       // 处理限流错误
//   }
func ClassifyAliyunError(err error) string {
	return AliyunClassifier.Classify(err)
}

// ClassifyTencentError 分类腾讯云错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//   err := someTencentAPI()
//   status := ClassifyTencentError(err)
//   if status == common.ErrorStatusLimit {
//       // 处理限流错误
//   }
func ClassifyTencentError(err error) string {
	return TencentClassifier.Classify(err)
}

// ClassifyAWSError 分类 AWS 错误（兼容函数）
// 这是为了保持向后兼容而提供的便捷函数
// 示例：
//   err := someAWSAPI()
//   status := ClassifyAWSError(err)
//   if status == common.ErrorStatusLimit {
//       // 处理限流错误
//   }
func ClassifyAWSError(err error) string {
	return AWSClassifier.Classify(err)
}
