package parallel

import (
	"runtime"
	"sync"
)

const (
	minNumWorkers = 2
)

// Task is a function type for parallel manager.
type Task func() error

// Manager is a structure for running tasks in parallel.
type Manager struct {
	wg        *sync.WaitGroup
	semaphore chan struct{}

	mu     sync.Mutex
	closed bool
}

// New creates a new parallel.Manager.
func New(workercount int) *Manager {
	if workercount < 0 {
		workercount = runtime.NumCPU() * -workercount
	}

	if workercount < minNumWorkers {
		workercount = minNumWorkers
	}

	return &Manager{
		wg:        &sync.WaitGroup{},
		semaphore: make(chan struct{}, workercount),
	}
}

// acquire limits concurrency by trying to acquire the semaphore.
func (p *Manager) acquire() {
	p.semaphore <- struct{}{}
}

// release releases the acquired semaphore to signal that a task is finished.
func (p *Manager) release() {
	p.wg.Done()
	<-p.semaphore
}

// Run runs the given task while limiting the concurrency.
func (p *Manager) Run(fn Task, waiter *Waiter) {
	if fn == nil {
		panic("parallel: Run called with a nil task")
	}
	if waiter == nil {
		panic("parallel: Run called with a nil waiter")
	}

	// Register under the lock so Close cannot mark the manager closed and
	// see an empty WaitGroup while a task is still on its way in. The slot
	// is acquired outside the lock: waiting for a free worker must not
	// block Close or other callers.
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		panic("parallel: Run called after Close")
	}
	p.wg.Add(1)
	waiter.wg.Add(1)
	p.mu.Unlock()

	p.acquire()
	go func() {
		defer waiter.wg.Done()
		defer p.release()

		if err := fn(); err != nil {
			waiter.errch <- err
		}
	}()
}

// Close waits all tasks to finish.
func (p *Manager) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	p.wg.Wait()
	close(p.semaphore)
}

// Waiter is a structure for waiting and reading
// error messages created by Manager.
type Waiter struct {
	wg    sync.WaitGroup
	errch chan error
	once  sync.Once
}

// NewWaiter creates a new parallel.Waiter.
func NewWaiter() *Waiter {
	return &Waiter{
		errch: make(chan error),
	}
}

// Wait blocks until the WaitGroup counter is zero
// and closes error channel.
func (w *Waiter) Wait() {
	w.wg.Wait()
	w.once.Do(func() { close(w.errch) })
}

// Err returns read-only error channel.
func (w *Waiter) Err() <-chan error {
	return w.errch
}
