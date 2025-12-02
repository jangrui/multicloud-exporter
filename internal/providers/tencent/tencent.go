// 腾讯云采集器：按配置采集 CVM 等资源的静态属性
package tencent

import (
    "log"

    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
    "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
    cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
    "multicloud-exporter/internal/config"
    "multicloud-exporter/internal/metrics"
)

// Collector 封装腾讯云资源采集逻辑
type Collector struct{}

// NewCollector 创建腾讯云采集器实例
func NewCollector() *Collector { return &Collector{} }

// Collect 根据账号配置遍历区域与资源类型并采集
func (t *Collector) Collect(account config.CloudAccount) {
    regions := account.Regions
    if len(regions) == 0 || (len(regions) == 1 && regions[0] == "*") {
        regions = []string{"ap-guangzhou", "ap-shanghai", "ap-beijing", "ap-chengdu", "ap-chongqing", "ap-nanjing", "ap-hongkong", "ap-singapore", "ap-tokyo", "ap-seoul"}
    }

    for _, region := range regions {
        for _, resource := range account.Resources {
            switch resource {
            case "cvm":
                t.collectCVM(account, region)
            case "cdb":
                t.collectCDB(account, region)
            case "redis":
                t.collectRedis(account, region)
            case "clb":
                t.collectCLB(account, region)
            case "eip":
                t.collectEIP(account, region)
            default:
                log.Printf("Tencent resource type %s not implemented yet", resource)
            }
        }
    }
}

// collectCVM 采集 CVM 的 CPU、内存等基础信息
func (t *Collector) collectCVM(account config.CloudAccount, region string) {
    credential := common.NewCredential(account.AccessKeyID, account.AccessKeySecret)
    cpf := profile.NewClientProfile()
    client, err := cvm.NewClient(credential, region, cpf)
    if err != nil {
        log.Printf("Tencent CVM client error: %v", err)
        return
    }

    request := cvm.NewDescribeInstancesRequest()
    response, err := client.DescribeInstances(request)
    if err != nil {
        log.Printf("Tencent CVM describe error: %v", err)
        return
    }

    if response.Response.InstanceSet != nil {
        for _, instance := range response.Response.InstanceSet {
            metrics.ResourceMetric.WithLabelValues(
                "tencent",
                account.AccountID,
                region,
                "cvm",
                *instance.InstanceId,
                "cpu_cores",
            ).Set(float64(*instance.CPU))

            metrics.ResourceMetric.WithLabelValues(
                "tencent",
                account.AccountID,
                region,
                "cvm",
                *instance.InstanceId,
                "memory_gb",
            ).Set(float64(*instance.Memory))
        }
    }
}

func (t *Collector) collectCDB(account config.CloudAccount, region string) {
    log.Printf("Collecting Tencent CDB in region %s (not implemented)", region)
}

func (t *Collector) collectRedis(account config.CloudAccount, region string) {
    log.Printf("Collecting Tencent Redis in region %s (not implemented)", region)
}

func (t *Collector) collectCLB(account config.CloudAccount, region string) {
    log.Printf("Collecting Tencent CLB in region %s (not implemented)", region)
}

func (t *Collector) collectEIP(account config.CloudAccount, region string) {
    log.Printf("Collecting Tencent EIP in region %s (not implemented)", region)
}
