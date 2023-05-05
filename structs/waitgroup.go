package structs

import "sync/atomic"

type TimeoutWaitGroup struct {
	running int64
	done    chan struct{}
}

func NewTimeoutWaitGroup() *TimeoutWaitGroup {
	return &TimeoutWaitGroup{done: make(chan struct{})}
}

func (wg *TimeoutWaitGroup) Add(i int64) {
	select {
	case <-wg.done:
		return
	default:
	}
	atomic.AddInt64(&wg.running, i)
}

func (wg *TimeoutWaitGroup) Done() {
	if atomic.AddInt64(&wg.running, -1) == 0 {
		close(wg.done)
	}
}

func (wg *TimeoutWaitGroup) C() <-chan struct{} {
	return wg.done
}
