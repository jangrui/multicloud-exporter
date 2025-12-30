package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"multicloud-exporter/internal/collector"
	"multicloud-exporter/internal/config"
	"multicloud-exporter/internal/discovery"
	"multicloud-exporter/internal/logger"
	"multicloud-exporter/internal/metrics"
)

const (
	// SSE 连接最大超时时间
	maxSSETimeout = 30 * time.Minute
	// 最大并发订阅数（防止资源耗尽）
	maxConcurrentSubs = 100
)

// setupHTTPHandlers 设置所有 HTTP 处理器
func setupHTTPHandlers(cfg *config.Config, coll *collector.Collector, mgr *discovery.Manager) {
	// Prometheus 指标端点
	http.Handle("/metrics", promhttp.Handler())

	// 健康检查端点
	http.HandleFunc("/healthz", handleHealthz(coll, mgr))

	// 创建认证包装器
	authWrapper := createAuthWrapper(cfg)

	// 管理端点（需要认证）
	http.HandleFunc("/collect", authWrapper(handleCollect(coll)))
	http.HandleFunc("/status", authWrapper(handleStatus(coll)))
	http.HandleFunc("/api/discovery/config", authWrapper(handleDiscoveryConfig(mgr)))
	http.HandleFunc("/api/discovery/stream", authWrapper(handleDiscoveryStream(mgr)))
	http.HandleFunc("/api/discovery/status", authWrapper(handleDiscoveryStatus(mgr)))
}

// handleHealthz 健康检查处理器（深度检查）
func handleHealthz(coll *collector.Collector, mgr *discovery.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 基本响应
		w.Header().Set("Content-Type", "application/json")

		health := map[string]interface{}{
			"status": "healthy",
			"time":   time.Now().Unix(),
		}

		// 检查配置是否加载
		if mgr == nil {
			health["status"] = "unhealthy"
			health["error"] = "discovery manager not initialized"
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(health)
			return
		}

		// 检查最后一次采集是否成功（5分钟内）
		status := coll.GetStatus()
		if !status.LastEnd.IsZero() && time.Since(status.LastEnd) > 5*time.Minute {
			health["status"] = "degraded"
			health["warning"] = "last collection completed more than 5 minutes ago"
			health["last_collection"] = status.LastEnd.Format(time.RFC3339)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(health)
	}
}

// handleCollect 手动触发采集处理器
func handleCollect(coll *collector.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		resource := r.URL.Query().Get("resource")
		go coll.CollectFiltered(provider, resource)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":   "triggered",
			"provider": provider,
			"resource": resource,
		})
	}
}

// handleStatus 获取采集状态处理器
func handleStatus(coll *collector.Collector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(coll.GetStatus())
	}
}

// handleDiscoveryConfig 获取发现配置处理器
func handleDiscoveryConfig(mgr *discovery.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := struct {
			Version   int64                       `json:"version"`
			UpdatedAt int64                       `json:"updated_at"`
			Products  map[string][]config.Product `json:"products"`
		}{
			Version:   mgr.Version(),
			UpdatedAt: mgr.UpdatedAt().Unix(),
			Products:  mgr.Get(),
		}
		bs, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(bs)
	}
}

// handleDiscoveryStream 发现配置 SSE 流处理器（添加超时和并发控制）
func handleDiscoveryStream(mgr *discovery.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 检查当前订阅数是否超限
		subsCount := mgr.GetSubscribersCount()
		if subsCount >= maxConcurrentSubs {
			http.Error(w, "too many subscribers", http.StatusServiceUnavailable)
			logger.Log.Warnf("SSE 连接被拒绝：当前订阅数=%d，上限=%d", subsCount, maxConcurrentSubs)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		fl, _ := w.(http.Flusher)

		// 设置超时控制
		ctx, cancel := context.WithTimeout(r.Context(), maxSSETimeout)
		defer cancel()

		ch := mgr.Subscribe()
		defer mgr.Unsubscribe(ch)

		logger.Log.Infof("SSE 连接建立，当前订阅数=%d", subsCount+1)

		// 发送初始版本
		initPayload := struct {
			Version int64 `json:"version"`
		}{Version: mgr.Version()}
		bs, _ := json.Marshal(initPayload)
		_, _ = w.Write([]byte("event: init\n"))
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(bs)
		_, _ = w.Write([]byte("\n\n"))
		if fl != nil {
			fl.Flush()
		}

		// 监听配置变化（带超时控制）
		for {
			select {
			case <-ctx.Done():
				// 超时或客户端断开
				logger.Log.Infof("SSE 连接关闭")
				return

			case <-ch:
				// 配置更新
				payload := struct {
					Version int64 `json:"version"`
				}{Version: mgr.Version()}
				bs, _ := json.Marshal(payload)
				_, _ = w.Write([]byte("event: update\n"))
				_, _ = w.Write([]byte("data: "))
				_, _ = w.Write(bs)
				_, _ = w.Write([]byte("\n\n"))
				if fl != nil {
					fl.Flush()
				}
			}
		}
	}
}

// handleDiscoveryStatus 获取发现状态处理器
func handleDiscoveryStatus(mgr *discovery.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		st := mgr.Status()
		resp := struct {
			discovery.DiscoveryStatus
			APIStats []metrics.APIStat `json:"api_stats"`
		}{
			DiscoveryStatus: st,
			APIStats:        metrics.GetAPIStats(),
		}
		bs, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(bs)
	}
}

// createAuthWrapper 创建 BasicAuth 认证包装器
func createAuthWrapper(cfg *config.Config) func(http.HandlerFunc) http.HandlerFunc {
	return func(h http.HandlerFunc) http.HandlerFunc {
		// 收集所有认证账号对
		pairs := collectAuthPairs(cfg)

		// 如果没有启用认证或没有账号，直接返回原始处理器
		if len(pairs) == 0 {
			return h
		}

		// 返回带认证的处理器
		return func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// 使用常量时间比较防止时序攻击
			authed := false
			for _, pair := range pairs {
				if subtle.ConstantTimeCompare([]byte(u), []byte(pair.Username)) == 1 &&
					subtle.ConstantTimeCompare([]byte(p), []byte(pair.Password)) == 1 {
					authed = true
					break
				}
			}

			if !authed {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			h(w, r)
		}
	}
}

// collectAuthPairs 收集所有认证账号对
func collectAuthPairs(cfg *config.Config) []config.BasicAuth {
	var pairs []config.BasicAuth

	// 1. 从环境变量收集
	if ev := getEnv("ADMIN_AUTH_ENABLED"); ev != "" {
		if ev == "1" || strings.EqualFold(ev, "true") || strings.EqualFold(ev, "yes") {
			// 从 ADMIN_AUTH JSON 配置
			if raw := getEnv("ADMIN_AUTH"); raw != "" {
				var xs []config.BasicAuth
				if json.Unmarshal([]byte(raw), &xs) == nil && len(xs) > 0 {
					pairs = append(pairs, xs...)
				} else {
					// 从逗号分隔的配置
					for _, seg := range strings.Split(raw, ",") {
						kv := strings.SplitN(strings.TrimSpace(seg), ":", 2)
						if len(kv) == 2 && kv[0] != "" {
							pairs = append(pairs, config.BasicAuth{
								Username: kv[0],
								Password: kv[1],
							})
						}
					}
				}
			}

			// 从 ADMIN_USERNAME/ADMIN_PASSWORD 单账号配置
			u := getEnv("ADMIN_USERNAME")
			p := getEnv("ADMIN_PASSWORD")
			if u != "" && p != "" {
				pairs = append(pairs, config.BasicAuth{Username: u, Password: p})
			}
		}
	}

	// 2. 从配置文件收集
	if len(pairs) == 0 && cfg.Server != nil && cfg.Server.AdminAuthEnabled {
		pairs = append(pairs, cfg.Server.AdminAuth...)
	}

	return pairs
}
