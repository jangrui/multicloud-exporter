package aliyun

import (
    "log"
    "sort"
    "strings"
    "time"

    "multicloud-exporter/internal/config"
    "multicloud-exporter/internal/metrics"

    "github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
    "github.com/aliyun/alibaba-cloud-sdk-go/services/vpc"
)

func (a *Collector) listCBWPIDs(account config.CloudAccount, region string) []string {
    log.Printf("Aliyun 枚举共享带宽包开始 account_id=%s region=%s", account.AccountID, region)
    client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
    if err != nil {
        return []string{}
    }
    var ids []string
    pageSize := 50
    if a.cfg != nil {
        if a.cfg.Server != nil && a.cfg.Server.PageSize > 0 {
            if a.cfg.Server.PageSize < pageSize {
                pageSize = a.cfg.Server.PageSize
            }
        } else if a.cfg.ServerConf != nil && a.cfg.ServerConf.PageSize > 0 {
            if a.cfg.ServerConf.PageSize < pageSize {
                pageSize = a.cfg.ServerConf.PageSize
            }
        }
    }
    page := 1
    for {
        req := vpc.CreateDescribeCommonBandwidthPackagesRequest()
        req.RegionId = region
        req.PageSize = requests.NewInteger(pageSize)
        req.PageNumber = requests.NewInteger(page)
        start := time.Now()
        resp, err := client.DescribeCommonBandwidthPackages(req)
        if err != nil {
            msg := err.Error()
            if strings.Contains(msg, "InvalidParameter") || strings.Contains(msg, "InvalidPageSize") {
                req.PageSize = requests.NewInteger(pageSize)
                resp, err = client.DescribeCommonBandwidthPackages(req)
                if err == nil {
                    goto HANDLE_RESP
                }
            }
            if strings.Contains(msg, "InvalidRegionId") || strings.Contains(msg, "Unsupported") {
                metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "skip").Inc()
                log.Printf("Aliyun CBWP skip region=%s: %v", region, err)
                break
            }
            metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "error").Inc()
            log.Printf("Aliyun CBWP describe error region=%s page=%d: %v", region, page, err)
            break
        }
    HANDLE_RESP:
        metrics.RequestTotal.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages", "success").Inc()
        metrics.RequestDuration.WithLabelValues("aliyun", "DescribeCommonBandwidthPackages").Observe(time.Since(start).Seconds())
        if resp.CommonBandwidthPackages.CommonBandwidthPackage == nil || len(resp.CommonBandwidthPackages.CommonBandwidthPackage) == 0 {
            break
        }
        for _, pkg := range resp.CommonBandwidthPackages.CommonBandwidthPackage {
            ids = append(ids, pkg.BandwidthPackageId)
        }
        if len(resp.CommonBandwidthPackages.CommonBandwidthPackage) < pageSize {
            break
        }
        page++
        time.Sleep(50 * time.Millisecond)
    }
    log.Printf("Aliyun 枚举共享带宽包完成 account_id=%s region=%s 数量=%d", account.AccountID, region, len(ids))
    return ids
}

func (a *Collector) fetchCBWPTags(account config.CloudAccount, region string, ids []string) map[string]string {
    if len(ids) == 0 {
        return map[string]string{}
    }
    client, err := vpc.NewClientWithAccessKey(region, account.AccessKeyID, account.AccessKeySecret)
    if err != nil {
        return map[string]string{}
    }
    out := make(map[string]string, len(ids))
    const batchSize = 20
    for start := 0; start < len(ids); start += batchSize {
        end := start + batchSize
        if end > len(ids) {
            end = len(ids)
        }
        batch := ids[start:end]
        req := vpc.CreateListTagResourcesRequest()
        req.RegionId = region
        req.ResourceType = "COMMONBANDWIDTHPACKAGE"
        req.MaxResults = requests.NewInteger(50)
        req.ResourceId = &batch
        nextToken := ""
        for {
            if nextToken != "" {
                req.NextToken = nextToken
            }
            startReq := time.Now()
            resp, err := client.ListTagResources(req)
            if err != nil {
                metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "error").Inc()
                break
            }
            metrics.RequestTotal.WithLabelValues("aliyun", "ListTagResources", "success").Inc()
            metrics.RequestDuration.WithLabelValues("aliyun", "ListTagResources").Observe(time.Since(startReq).Seconds())
            if resp.TagResources.TagResource == nil {
                break
            }
            tmp := make(map[string]map[string]string)
            for _, tr := range resp.TagResources.TagResource {
                rid := tr.ResourceId
                if rid == "" {
                    continue
                }
                if _, ok := tmp[rid]; !ok {
                    tmp[rid] = make(map[string]string)
                }
                if tr.TagKey != "" {
                    tmp[rid][tr.TagKey] = tr.TagValue
                }
            }
            for rid, kv := range tmp {
                var keys []string
                for k := range kv {
                    keys = append(keys, k)
                }
                sort.Strings(keys)
                var parts []string
                for _, k := range keys {
                    parts = append(parts, k+"="+kv[k])
                }
                out[rid] = strings.Join(parts, ",")
            }
            if resp.NextToken == "" {
                break
            }
            nextToken = resp.NextToken
            time.Sleep(25 * time.Millisecond)
        }
        time.Sleep(25 * time.Millisecond)
    }
    return out
}

