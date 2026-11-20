package cronrunner

import "sync"

// taskMutexRegistry holds per-task mutexes so concurrent cron ticks for the
// same taskID serialize (CRON-01 / AS-COG-01: named type instead of bare sync.Map).
type taskMutexRegistry struct {
	m sync.Map // taskID string -> *sync.Mutex
}

func (r *taskMutexRegistry) lock(taskID string) (unlock func()) {
	v, _ := r.m.LoadOrStore(taskID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// tryLock 不等待：任务正被执行（手动触发 / 定时执行含重试退避，可长达十几分钟）时返回 ok=false。
// 调度 tick 里只能用它——tick 是所有任务共用的一条循环，等一个任务的锁会把全部任务卡住。
func (r *taskMutexRegistry) tryLock(taskID string) (unlock func(), ok bool) {
	v, _ := r.m.LoadOrStore(taskID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, false
	}
	return mu.Unlock, true
}
