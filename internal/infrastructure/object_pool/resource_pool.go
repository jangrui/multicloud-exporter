package object_pool

// ResourcePool 资源管理器对象池
type ResourcePool struct {
	*ObjectPool
}

// NewResourcePool 创建资源管理器对象池
// newFunc 是创建新 ResourceManager 的函数
func NewResourcePool(newFunc func() interface{}) *ResourcePool {
	return &ResourcePool{
		ObjectPool: NewObjectPool(newFunc),
	}
}
