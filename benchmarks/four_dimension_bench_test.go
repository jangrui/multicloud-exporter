package benchmarks

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"multicloud-exporter/internal/infrastructure/lock_free"
	"multicloud-exporter/internal/layer"

	"go.uber.org/zap"
)

// ==================== 基准测试配置 ====================

const (
	numAccounts   = 100
	numProducts   = 6
	numRegions    = 10
	numResources  = 100
	numOpsPerLoop = 1000
)

// ==================== 1. 无锁并发模型性能对比 ====================

// 1.1 对比 LockFreeManager vs sync.RWMutex 的吞吐量

// 使用 sync.RWMutex 的传统实现
type MutexManager struct {
	mu    sync.RWMutex
	value int64
}

func (m *MutexManager) Get() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.value
}

func (m *MutexManager) Set(val int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value = val
}

func BenchmarkLockFreeManager_Throughput(b *testing.B) {
	b.Run("LockFreeManager", func(b *testing.B) {
		mgr := lock_free.NewLockFreeManager()
		mgr.Store(int64(0))

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var local int64
			for pb.Next() {
				val := mgr.Load().(int64)
				if val%2 == 0 {
					mgr.Store(val + 1)
				} else {
					local = val
				}
			}
			_ = local
		})
	})

	b.Run("SyncRWMutex", func(b *testing.B) {
		mgr := &MutexManager{value: 0}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var local int64
			for pb.Next() {
				val := mgr.Get()
				if val%2 == 0 {
					mgr.Set(val + 1)
				} else {
					local = val
				}
			}
			_ = local
		})
	})
}

// 1.2 对比 P99 延迟

func BenchmarkLockFreeManager_Latency(b *testing.B) {
	b.Run("LockFreeManager", func(b *testing.B) {
		mgr := lock_free.NewLockFreeManager()
		mgr.Store(int64(0))

		latencies := make([]time.Duration, numOpsPerLoop)
		var i int64

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			start := time.Now()
			mgr.Store(atomic.LoadInt64(&i))
			latency := time.Since(start)
			latencies[n%numOpsPerLoop] = latency
			atomic.AddInt64(&i, 1)
		}

		b.StopTimer()
		reportP99(b, latencies)
	})

	b.Run("SyncRWMutex", func(b *testing.B) {
		mgr := &MutexManager{value: 0}

		latencies := make([]time.Duration, numOpsPerLoop)
		var i int64

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			start := time.Now()
			mgr.Set(atomic.LoadInt64(&i))
			latency := time.Since(start)
			latencies[n%numOpsPerLoop] = latency
			atomic.AddInt64(&i, 1)
		}

		b.StopTimer()
		reportP99(b, latencies)
	})
}

// 1.3 锁竞争测试

func BenchmarkLockFreeManager_Contention(b *testing.B) {
	b.Run("LockFreeManager", func(b *testing.B) {
		mgr := lock_free.NewLockFreeManager()
		mgr.Store(int64(0))

		var contentions int64

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				oldVal := mgr.Load().(int64)
				newVal := oldVal + 1
				if !mgr.CompareAndSwap(oldVal, newVal) {
					atomic.AddInt64(&contentions, 1)
				}
			}
		})

		b.ReportMetric(float64(contentions)/float64(b.N)*100, "contention_pct")
	})

	b.Run("SyncRWMutex", func(b *testing.B) {
		mgr := &MutexManager{value: 0}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				mgr.Set(mgr.Get() + 1)
			}
		})
	})
}

// ==================== 2. 对象池性能对比 ====================

// 2.1 对比对象池 vs 直接分配的 GC 压力

type TestResource struct {
	data [100]int64
	id   int
}

func BenchmarkObjectPool_Allocation(b *testing.B) {
	pool := &sync.Pool{
		New: func() any {
			return &TestResource{id: 0}
		},
	}

	b.Run("WithPool", func(b *testing.B) {
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res := pool.Get().(*TestResource)
			res.id = i
			res.data[0] = int64(i)
			pool.Put(res)
		}

		b.StopTimer()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		allocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024
		b.ReportMetric(allocMB/float64(b.N), "MB/op")
	})

	b.Run("WithoutPool", func(b *testing.B) {
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res := &TestResource{id: i}
			res.data[0] = int64(i)
			_ = res
		}

		b.StopTimer()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		allocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024
		b.ReportMetric(allocMB/float64(b.N), "MB/op")
	})
}

// 2.2 高并发场景下的对象池性能

func BenchmarkObjectPool_Concurrent(b *testing.B) {
	pool := &sync.Pool{
		New: func() any {
			return &TestResource{id: 0}
		},
	}

	b.Run("WithPool", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				res := pool.Get().(*TestResource)
				res.id = i
				res.data[0] = int64(i)
				i++
				pool.Put(res)
			}
		})
	})

	b.Run("WithoutPool", func(b *testing.B) {
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				res := &TestResource{id: i}
				res.data[0] = int64(i)
				_ = res
				i++
			}
		})
	})
}

// ==================== 3. 四维管理器性能测试 ====================

// 3.1 AccountManager 并发访问性能

func BenchmarkAccountManager_ConcurrentStatus(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = am.GetAccountStatus()
		}
	})
}

// 3.2 AccountManager 层次化访问（GetProductManager + GetRegionManager）

func BenchmarkAccountManager_HierarchicalAccess(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	products := []string{"ecs", "rds", "oss", "slb", "vpc", "cdn"}
	regions := make([]string, numRegions)
	for i := 0; i < numRegions; i++ {
		regions[i] = fmt.Sprintf("cn-region-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			product := products[i%len(products)]
			region := regions[i%len(regions)]

			pm, err := am.GetProductManager(product)
			if err != nil {
				continue
			}
			if _, err := pm.GetRegionManager(region); err != nil {
				continue
			}
			i++
		}
	})
}

// 3.3 ProductManager 并发访问性能

func BenchmarkProductManager_ConcurrentAccess(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	products := []string{"ecs", "rds", "oss"}
	regions := make([]string, numRegions)
	for i := 0; i < numRegions; i++ {
		regions[i] = fmt.Sprintf("cn-region-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			product := products[i%len(products)]
			region := regions[i%len(regions)]

			pm, _ := am.GetProductManager(product)
			if pm != nil {
				_ = pm.GetProductStatus()
				_, _ = pm.GetRegionManager(region)
			}
			i++
		}
	})
}

// 3.4 RegionManager 并发访问性能

func BenchmarkRegionManager_ConcurrentAccess(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	products := []string{"ecs", "rds", "oss"}
	regions := make([]string, numRegions)
	for i := 0; i < numRegions; i++ {
		regions[i] = fmt.Sprintf("cn-region-%d", i)
	}

	resources := make([]string, numResources)
	for i := 0; i < numResources; i++ {
		resources[i] = fmt.Sprintf("resource-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			product := products[i%len(products)]
			region := regions[i%len(regions)]
			resource := resources[i%len(resources)]

			pm, _ := am.GetProductManager(product)
			if pm != nil {
				rm, _ := pm.GetRegionManager(region)
				if rm != nil {
					_ = rm.GetRegionStatus()
					_, _ = rm.GetResourceManager(resource)
				}
			}
			i++
		}
	})
}

// 3.5 ResourceManager 并发访问性能

func BenchmarkResourceManager_ConcurrentAccess(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	products := []string{"ecs", "rds", "oss"}
	regions := make([]string, numRegions)
	for i := 0; i < numRegions; i++ {
		regions[i] = fmt.Sprintf("cn-region-%d", i)
	}

	resources := make([]string, numResources)
	for i := 0; i < numResources; i++ {
		resources[i] = fmt.Sprintf("resource-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			product := products[i%len(products)]
			region := regions[i%len(regions)]
			resource := resources[i%len(resources)]

			pm, _ := am.GetProductManager(product)
			if pm != nil {
				rm, _ := pm.GetRegionManager(region)
				if rm != nil {
					resMgr, _ := rm.GetResourceManager(resource)
					if resMgr != nil {
						_ = resMgr.GetResourceStatus()
					}
				}
			}
			i++
		}
	})
}

// ==================== 4. 四维完整流程性能测试 ====================

// 4.1 完整采集流程：Account → Product → Region → Resource

func BenchmarkFourDimensions_CollectWorkflow(b *testing.B) {
	logger := zap.NewNop()
	am := layer.NewAccountManager(layer.AccountManagerConfig{
		AccountID:   "test-account",
		AccountName: "Test Account",
		Provider:    "aliyun",
		Region:      "cn-hangzhou",
		Logger:      logger,
	})
	defer am.Stop()
	defer am.Cleanup()

	products := []string{"ecs", "rds", "oss"}
	regions := make([]string, numRegions)
	for i := 0; i < numRegions; i++ {
		regions[i] = fmt.Sprintf("cn-region-%d", i)
	}

	resources := make([]string, numResources)
	for i := 0; i < numResources; i++ {
		resources[i] = fmt.Sprintf("resource-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var ops int64
		for pb.Next() {
			for _, product := range products {
				for _, region := range regions {
					pm, _ := am.GetProductManager(product)
					if pm == nil {
						continue
					}
					rm, _ := pm.GetRegionManager(region)
					if rm != nil {
						_ = rm.GetRegionStatus()
						for _, resource := range resources {
							resMgr, _ := rm.GetResourceManager(resource)
							if resMgr != nil {
								_ = resMgr.GetResourceStatus()
							}
						}
					}
				}
			}
			atomic.AddInt64(&ops, 1)
		}
	})
}

// ==================== 5. 集群同步性能测试 ====================

// 5.1 消息同步延迟（模拟内存操作）

func BenchmarkClusterSync_Latency(b *testing.B) {
	b.Run("SingleMessage", func(b *testing.B) {
		latencies := make([]time.Duration, numOpsPerLoop)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()

			// 模拟同步操作（直接内存操作）
			data := make(map[string]interface{})
			data["timestamp"] = time.Now()
			data["account"] = fmt.Sprintf("account-%d", i%numAccounts)
			data["type"] = "region_update"

			latency := time.Since(start)
			latencies[i%numOpsPerLoop] = latency
		}

		b.StopTimer()
		reportP99(b, latencies)
	})
}

// 5.2 消息吞吐量

func BenchmarkClusterSync_Throughput(b *testing.B) {
	b.Run("SingleProducer", func(b *testing.B) {
		var totalMessages int64

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				msg := map[string]interface{}{
					"type":      "region_update",
					"timestamp": time.Now(),
					"account":   fmt.Sprintf("account-%d", atomic.LoadInt64(&totalMessages)%numAccounts),
				}
				_ = msg
				atomic.AddInt64(&totalMessages, 1)
			}
		})

		throughput := float64(totalMessages) / b.Elapsed().Seconds()
		b.ReportMetric(throughput, "msg/sec")
	})
}

// ==================== 6. 综合性能对比测试 ====================

// 6.1 吞吐量对比（生成报告用）

func BenchmarkComparison_Throughput(b *testing.B) {
	lockFreeThroughput := 0.0
	mutexThroughput := 0.0

	b.Run("LockFreeManager", func(b *testing.B) {
		mgr := lock_free.NewLockFreeManager()
		mgr.Store(int64(0))

		start := time.Now()
		var count int64

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				val := mgr.Load().(int64)
				mgr.Store(val + 1)
				atomic.AddInt64(&count, 1)
			}
		})

		elapsed := time.Since(start).Seconds()
		lockFreeThroughput = float64(count) / elapsed
		b.ReportMetric(lockFreeThroughput, "ops/sec")
	})

	b.Run("SyncRWMutex", func(b *testing.B) {
		mgr := &MutexManager{value: 0}

		start := time.Now()
		var count int64

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				val := mgr.Get()
				mgr.Set(val + 1)
				atomic.AddInt64(&count, 1)
			}
		})

		elapsed := time.Since(start).Seconds()
		mutexThroughput = float64(count) / elapsed
		b.ReportMetric(mutexThroughput, "ops/sec")
	})

	b.StopTimer()
	if lockFreeThroughput > 0 && mutexThroughput > 0 {
		improvement := lockFreeThroughput / mutexThroughput
		b.Logf("LockFreeManager is %.2fx faster than SyncRWMutex", improvement)
	}
}

// 6.2 内存占用对比

func BenchmarkComparison_Memory(b *testing.B) {
	b.Run("WithObjectPool", func(b *testing.B) {
		pool := &sync.Pool{
			New: func() any {
				return &TestResource{id: 0}
			},
		}

		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		runtime.GC()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res := pool.Get().(*TestResource)
			res.id = i
			res.data[0] = int64(i)
			pool.Put(res)
		}

		b.StopTimer()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		heapMB := float64(m2.HeapInuse) / 1024 / 1024
		b.ReportMetric(heapMB, "MB_heap")
	})

	b.Run("WithoutObjectPool", func(b *testing.B) {
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		runtime.GC()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res := &TestResource{id: i}
			res.data[0] = int64(i)
			_ = res
		}

		b.StopTimer()
		var m2 runtime.MemStats
		runtime.ReadMemStats(&m2)
		heapMB := float64(m2.HeapInuse) / 1024 / 1024
		b.ReportMetric(heapMB, "MB_heap")
	})
}

// ==================== 辅助函数 ====================

// reportP99 计算 P99 延迟
func reportP99(b *testing.B, latencies []time.Duration) {
	if len(latencies) == 0 {
		return
	}

	// 排序（使用简单排序，基准测试不需要最高效）
	n := len(latencies)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if latencies[i] > latencies[j] {
				latencies[i], latencies[j] = latencies[j], latencies[i]
			}
		}
	}

	// P99 = 99th percentile
	p99Index := n * 99 / 100
	p99 := latencies[p99Index].Seconds()

	b.ReportMetric(p99*1000, "ms/P99")
}
