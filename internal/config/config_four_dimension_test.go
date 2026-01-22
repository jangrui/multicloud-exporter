// 四维配置单元测试
package config

import (
	"testing"
)

// TestDefaultFourDimensionConfig 测试默认配置
func TestDefaultFourDimensionConfig(t *testing.T) {
	cfg := defaultFourDimensionConfig()

	if cfg.ConcurrencyMode != "auto" {
		t.Errorf("expected concurrency_mode=auto, got %s", cfg.ConcurrencyMode)
	}
	if cfg.MaxConcurrency != 20 {
		t.Errorf("expected max_concurrency=20, got %d", cfg.MaxConcurrency)
	}
	if !cfg.PerformanceTuning {
		t.Errorf("expected performance_tuning=true, got %v", cfg.PerformanceTuning)
	}
}

// TestCalculateConcurrency_Conservative 测试保守模式
func TestCalculateConcurrency_Conservative(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "conservative",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 10)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 1 {
		t.Errorf("expected product_concurrency=1, got %d", prodConc)
	}
	if regConc != 1 {
		t.Errorf("expected region_concurrency=1, got %d", regConc)
	}
	if total != 4 {
		t.Errorf("expected total=4, got %d", total)
	}
}

// TestCalculateConcurrency_Standard 测试标准模式
func TestCalculateConcurrency_Standard(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "standard",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 10)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 2 {
		t.Errorf("expected product_concurrency=2, got %d", prodConc)
	}
	if regConc != 2 {
		t.Errorf("expected region_concurrency=2, got %d", regConc)
	}
	if total != 16 {
		t.Errorf("expected total=16, got %d", total)
	}
}

// TestCalculateConcurrency_Aggressive 测试激进模式
func TestCalculateConcurrency_Aggressive(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "aggressive",
		MaxConcurrency:    50,
		PerformanceTuning: true,
	}

	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 10)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 3 {
		t.Errorf("expected product_concurrency=3, got %d", prodConc)
	}
	if regConc != 4 {
		t.Errorf("expected region_concurrency=4, got %d", regConc)
	}
	if total != 48 {
		t.Errorf("expected total=48, got %d", total)
	}
}

// TestCalculateConcurrency_Auto_SmallAccount 测试自动模式（少量账号）
func TestCalculateConcurrency_Auto_SmallAccount(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	// 账号数 ≤ 10，使用 conservative 模式
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 5)

	if accConc != 2 {
		t.Errorf("expected account_concurrency=2, got %d", accConc)
	}
	if prodConc != 2 {
		t.Errorf("expected product_concurrency=2, got %d", prodConc)
	}
	if regConc != 2 {
		t.Errorf("expected region_concurrency=2, got %d", regConc)
	}
	if total != 8 {
		t.Errorf("expected total=8, got %d", total)
	}
}

// TestCalculateConcurrency_Auto_MediumAccount 测试自动模式（中等账号）
func TestCalculateConcurrency_Auto_MediumAccount(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	// 账号数 10-50，使用 standard 模式
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 30)

	if accConc != 3 {
		t.Errorf("expected account_concurrency=3, got %d", accConc)
	}
	if prodConc != 1 {
		t.Errorf("expected product_concurrency=1, got %d", prodConc)
	}
	if regConc != 2 {
		t.Errorf("expected region_concurrency=2, got %d", regConc)
	}
	if total != 6 {
		t.Errorf("expected total=6, got %d", total)
	}
}

// TestCalculateConcurrency_Auto_LargeAccount 测试自动模式（大量账号）
func TestCalculateConcurrency_Auto_LargeAccount(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    50,
		PerformanceTuning: true,
	}

	// 账号数 > 50，使用 aggressive 模式
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 100)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 3 {
		t.Errorf("expected product_concurrency=3, got %d", prodConc)
	}
	if regConc != 4 {
		t.Errorf("expected region_concurrency=4, got %d", regConc)
	}
	if total != 48 {
		t.Errorf("expected total=48, got %d", total)
	}
}

// TestCalculateConcurrency_MaxLimit 测试总并发度限制
func TestCalculateConcurrency_MaxLimit(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "aggressive",
		MaxConcurrency:    10,
		PerformanceTuning: true,
	}

	// 激进模式总并发度 48，但 max_concurrency=10
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 100)

	// 预期：自动调整各维度并发度，使总并发度 ≤ 10
	if total > fd.MaxConcurrency {
		t.Errorf("expected total <= %d, got %d", fd.MaxConcurrency, total)
	}

	// 验证调整后的并发度（accConc=1, prodConc=2, regConc=4, total=8）
	if total != 8 {
		t.Errorf("expected total=8, got %d (accConc=%d, prodConc=%d, regConc=%d)", total, accConc, prodConc, regConc)
	}
}

// TestCalculateConcurrency_NoPerformanceTuning 测试禁用性能优化
func TestCalculateConcurrency_NoPerformanceTuning(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    20,
		PerformanceTuning: false,
	}

	// 账号数 ≤ 10，不启用性能优化
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 3)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 1 {
		t.Errorf("expected product_concurrency=1, got %d", prodConc)
	}
	if regConc != 1 {
		t.Errorf("expected region_concurrency=1, got %d", regConc)
	}
	if total != 4 {
		t.Errorf("expected total=4, got %d", total)
	}
}

// TestCalculateConcurrency_InvalidMode 测试无效并发模式
func TestCalculateConcurrency_InvalidMode(t *testing.T) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "invalid",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	// 无效模式应使用默认（conservative）
	accConc, prodConc, regConc, total := CalculateConcurrency(fd, 10)

	if accConc != 4 {
		t.Errorf("expected account_concurrency=4, got %d", accConc)
	}
	if prodConc != 1 {
		t.Errorf("expected product_concurrency=1, got %d", prodConc)
	}
	if regConc != 1 {
		t.Errorf("expected region_concurrency=1, got %d", regConc)
	}
	if total != 4 {
		t.Errorf("expected total=4, got %d", total)
	}
}

// BenchmarkCalculateConcurrency 基准测试
func BenchmarkCalculateConcurrency(b *testing.B) {
	fd := FourDimensionConfig{
		ConcurrencyMode:   "auto",
		MaxConcurrency:    20,
		PerformanceTuning: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateConcurrency(fd, 100)
	}
}
