package discovery

// register built-in discoverers via init to make discovery work in production.
// Tests may register custom discoverers; those will override the same provider key.
func init() {
	Register("aliyun", &AliyunDiscoverer{})
	Register("tencent", &TencentDiscoverer{})
	Register("aws", &AWSDiscoverer{})
}
