// Package common 提供云厂商通用的错误处理和重试逻辑
package common

// 阿里云命名空间常量
const (
	NamespaceAliyunBandwidthPackage = "acs_bandwidth_package"
	NamespaceAliyunSLBDashboard     = "acs_slb_dashboard"
	NamespaceAliyunOSSDashboard     = "acs_oss_dashboard"
	NamespaceAliyunALB              = "acs_alb"
	NamespaceAliyunNLB              = "acs_nlb"
	NamespaceAliyunGWLB             = "acs_gwlb"
)

// 腾讯云命名空间常量
const (
	NamespaceTencentBWP  = "QCE/BWP"
	NamespaceTencentLB   = "QCE/LB"
	NamespaceTencentCOS  = "QCE/COS"
	NamespaceTencentGWLB = "qce/gwlb"
	NamespaceTencentCVM  = "QCE/CVM"
)

// AWS 命名空间常量
const (
	NamespaceAWSS3  = "AWS/S3"
	NamespaceAWSEC2 = "AWS/EC2"
	NamespaceAWSELB = "AWS/ELB"
)
