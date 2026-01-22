package object_pool

// ProductPool 产品管理器对象池
type ProductPool struct {
	*ObjectPool
}

// NewProductPool 创建产品管理器对象池
// newFunc 是创建新 ProductManager 的函数
func NewProductPool(newFunc func() interface{}) *ProductPool {
	return &ProductPool{
		ObjectPool: NewObjectPool(newFunc),
	}
}
