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
