package object_pool

// AccountPool 账号管理器对象池
type AccountPool struct {
	*ObjectPool
}

// NewAccountPool 创建账号管理器对象池
// newFunc 是创建新 AccountManager 的函数
func NewAccountPool(newFunc func() interface{}) *AccountPool {
	return &AccountPool{
		ObjectPool: NewObjectPool(newFunc),
	}
}
