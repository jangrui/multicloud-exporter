// 资源类型注册表：声明各云平台支持的资源标识
package collector

var SupportedResources = map[string][]string{
    "aliyun": {
        "ecs",
        "bwp",
        "slb",
    },
    "huawei":  {},
    "tencent": {},
}

// GetAllResources 返回指定云平台的全部资源类型
func GetAllResources(provider string) []string {
	if resources, ok := SupportedResources[provider]; ok {
		return resources
	}
	return []string{}
}
