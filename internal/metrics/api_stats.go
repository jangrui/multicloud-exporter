package metrics

import (
	"sync"
	"time"
)

type apiKey struct {
	Provider string
	API      string
}

type timeBucket struct {
	ts    int64
	count int64
}

type apiStat struct {
	total   int64
	status  map[string]int64
	buckets []timeBucket
	mu      sync.Mutex
}

var (
	apiStatsMu sync.RWMutex
	apiStats   = make(map[apiKey]*apiStat)
	windowSize = int64(300)
)

func RecordRequest(provider, api, status string) {
	now := time.Now().Unix()
	slot := now % windowSize
	k := apiKey{Provider: provider, API: api}
	apiStatsMu.RLock()
	st := apiStats[k]
	apiStatsMu.RUnlock()
	if st == nil {
		st = &apiStat{
			status:  make(map[string]int64),
			buckets: make([]timeBucket, windowSize),
		}
		apiStatsMu.Lock()
		apiStats[k] = st
		apiStatsMu.Unlock()
	}
	st.mu.Lock()
	st.total++
	if status != "" {
		st.status[status]++
	}
	if st.buckets[slot].ts != now {
		st.buckets[slot].ts = now
		st.buckets[slot].count = 0
	}
	st.buckets[slot].count++
	st.mu.Unlock()
}

type APIStat struct {
	Provider    string           `json:"provider"`
	API         string           `json:"api"`
	Total       int64            `json:"total"`
	StatusCount map[string]int64 `json:"status_count"`
	QPS1m       float64          `json:"qps_1m"`
	QPS5m       float64          `json:"qps_5m"`
}

func GetAPIStats() []APIStat {
	now := time.Now().Unix()
	apiStatsMu.RLock()
	out := make([]APIStat, 0, len(apiStats))
	for k, st := range apiStats {
		st.mu.Lock()
		var c1, c5 int64
		for i := int64(0); i < windowSize; i++ {
			b := st.buckets[i]
			age := now - b.ts
			if age >= 0 && age < 60 {
				c1 += b.count
			}
			if age >= 0 && age < 300 {
				c5 += b.count
			}
		}
		q1 := float64(c1) / 60.0
		q5 := float64(c5) / 300.0
		sc := make(map[string]int64, len(st.status))
		for s, v := range st.status {
			sc[s] = v
		}
		out = append(out, APIStat{
			Provider:    k.Provider,
			API:         k.API,
			Total:       st.total,
			StatusCount: sc,
			QPS1m:       q1,
			QPS5m:       q5,
		})
		st.mu.Unlock()
	}
	apiStatsMu.RUnlock()
	return out
}
