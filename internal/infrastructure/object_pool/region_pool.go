package object_pool

// RegionPool 区域管理器对象池
type RegionPool struct {
	*ObjectPool
}

// NewRegionPool 创建区域管理器对象池
// newFunc 是创建新 RegionManager 的函数
func NewRegionPool(newFunc func() interface{}) *RegionPool {
	return &RegionPool{
		ObjectPool: NewObjectPool(newFunc),
	}
}
