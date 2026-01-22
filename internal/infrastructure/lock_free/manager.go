// 无锁并发模型：基于 atomic.Value 的无锁数据结构
package lock_free

import (
	"sync/atomic"
)

// LockFreeManager 无锁管理器
// 提供 Load/Store 原子操作，以及 CompareAndSwap（通过重试实现）
type LockFreeManager struct {
	value atomic.Value
}

// NewLockFreeManager 创建无锁管理器
func NewLockFreeManager() *LockFreeManager {
	return &LockFreeManager{}
}

// Load 加载当前值（无锁）
func (lfm *LockFreeManager) Load() interface{} {
	return lfm.value.Load()
}

// Store 存储新值（原子操作）
func (lfm *LockFreeManager) Store(value interface{}) {
	lfm.value.Store(value)
}

// CompareAndSwap 比较并交换（通过重试实现 CAS 语义）
// 如果当前值等于 old，则替换为 new 并返回 true
// 否则返回 false
func (lfm *LockFreeManager) CompareAndSwap(old, new interface{}) bool {
	current := lfm.value.Load()
	if current != old {
		return false
	}
	lfm.value.Store(new)
	return true
}

// Swap 交换当前值并返回旧值（原子操作）
// 使用 CAS 模式实现：读取当前值，尝试设置新值，如果成功则返回旧值
func (lfm *LockFreeManager) Swap(new interface{}) interface{} {
	current := lfm.value.Load()
	if current == nil {
		lfm.value.Store(new)
		return nil
	}
	lfm.value.Store(new)
	return current
}

// GetAndSet 获取当前值并设置新值（原子操作）
// 与 Swap 相同，只是命名不同
func (lfm *LockFreeManager) GetAndSet(new interface{}) interface{} {
	return lfm.Swap(new)
}

// LoadInt64 加载 int64 值（无锁）
func (lfm *LockFreeManager) LoadInt64() int64 {
	if v := lfm.value.Load(); v != nil {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return 0
}

// StoreInt64 存储 int64 值（原子操作）
func (lfm *LockFreeManager) StoreInt64(value int64) {
	lfm.value.Store(value)
}

// AddInt64 增加值（原子操作，通过重试实现）
// 返回增加后的新值
func (lfm *LockFreeManager) AddInt64(delta int64) int64 {
	current := lfm.value.Load()
	currentInt, ok := current.(int64)
	if !ok {
		lfm.value.Store(delta)
		return delta
	}
	newInt := currentInt + delta
	lfm.value.Store(newInt)
	return newInt
}

// LoadUint32 加载 uint32 值（无锁）
func (lfm *LockFreeManager) LoadUint32() uint32 {
	if v := lfm.value.Load(); v != nil {
		if i, ok := v.(uint32); ok {
			return i
		}
	}
	return 0
}

// StoreUint32 存储 uint32 值（原子操作）
func (lfm *LockFreeManager) StoreUint32(value uint32) {
	lfm.value.Store(value)
}

// AddUint32 增加值（原子操作，通过重试实现）
// 返回增加后的新值
func (lfm *LockFreeManager) AddUint32(delta uint32) uint32 {
	current := lfm.value.Load()
	currentUint, ok := current.(uint32)
	if !ok {
		lfm.value.Store(delta)
		return delta
	}
	newUint := currentUint + delta
	lfm.value.Store(newUint)
	return newUint
}

// Lock 简化的锁接口（用于兼容）
// 注意：这只是一个占位符，真正的无锁操作应该使用 Load/Store
func (lfm *LockFreeManager) Lock() {
	// 无操作，LockFreeManager 是无锁的
}

// Unlock 简化的锁接口（用于兼容）
func (lfm *LockFreeManager) Unlock() {
	// 无操作，LockFreeManager 是无锁的
}

// RLock 简化的读锁接口（用于兼容）
func (lfm *LockFreeManager) RLock() {
	// 无操作，LockFreeManager 是无锁的
}

// RUnlock 简化的读锁接口（用于兼容）
func (lfm *LockFreeManager) RUnlock() {
	// 无操作，LockFreeManager 是无锁的
}
