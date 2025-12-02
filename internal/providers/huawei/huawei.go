// 华为云采集器：按配置采集 ECS 等资源的静态属性
package huawei

import (
	"log"

	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/metrics"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hwconfig "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	ecs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	ecsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/region"
)

// Collector 封装华为云资源采集逻辑
type Collector struct{}

// NewCollector 创建华为云采集器实例
func NewCollector() *Collector { return &Collector{} }

// Collect 根据账号配置遍历区域与资源类型并采集
func (h *Collector) Collect(account config.CloudAccount) {
	regions := account.Regions
	if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
		regions = []string{"cn-north-4", "cn-north-1", "cn-east-3", "cn-south-1", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3"}
	}

	for _, region := range regions {
		for _, resource := range account.Resources {
			switch resource {
			case "ecs":
				h.collectECS(account, region)
			case "rds":
				h.collectRDS(account, region)
			case "redis":
				h.collectRedis(account, region)
			case "elb":
				h.collectELB(account, region)
			case "eip":
				h.collectEIP(account, region)
			default:
				log.Printf("Huawei resource type %s not implemented yet", resource)
			}
		}
	}
}

// collectECS 采集 ECS 的基础状态示例
func (h *Collector) collectECS(account config.CloudAccount, region string) {
	auth := basic.NewCredentialsBuilder().
		WithAk(account.AccessKeyID).
		WithSk(account.AccessKeySecret).
		Build()

	client := ecs.NewEcsClient(
		ecs.EcsClientBuilder().
			WithRegion(ecsregion.ValueOf(region)).
			WithCredential(auth).
			WithHttpConfig(hwconfig.DefaultHttpConfig()).
			Build())

	request := &ecsmodel.ListServersDetailsRequest{}
	response, err := client.ListServersDetails(request)
	if err != nil {
		log.Printf("Huawei ECS describe error: %v", err)
		return
	}

	if response.Servers != nil {
		for _, server := range *response.Servers {
			metrics.ResourceMetric.WithLabelValues(
				"huawei",
				account.AccountID,
				region,
				"ecs",
				server.Id,
				"status",
			).Set(1)
		}
	}
}

func (h *Collector) collectRDS(account config.CloudAccount, region string) {
	log.Printf("Collecting Huawei RDS in region %s (not implemented)", region)
}

func (h *Collector) collectRedis(account config.CloudAccount, region string) {
	log.Printf("Collecting Huawei Redis in region %s (not implemented)", region)
}

func (h *Collector) collectELB(account config.CloudAccount, region string) {
	log.Printf("Collecting Huawei ELB in region %s (not implemented)", region)
}

func (h *Collector) collectEIP(account config.CloudAccount, region string) {
	log.Printf("Collecting Huawei EIP in region %s (not implemented)", region)
}
