package parallel

import (
	"errors"
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
			waiter.record(err)
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
// Waiter waits for a set of tasks and collects their errors. An optional
// handler sees each error as it happens, from the task's goroutine, so
// callers can print immediately or cancel the rest of the work; the
// collected errors are returned by Wait, so nothing needs to be drained.
type Waiter struct {
	wg      sync.WaitGroup
	mu      sync.Mutex
	errs    []error
	onError func(error)
}

// WaiterOption configures a Waiter.
type WaiterOption func(*Waiter)

// WithErrorHandler calls fn for every task error as it occurs. fn runs on
// the task's goroutine and may be called concurrently.
func WithErrorHandler(fn func(error)) WaiterOption {
	return func(w *Waiter) { w.onError = fn }
}

// NewWaiter creates a new Waiter.
func NewWaiter(opts ...WaiterOption) *Waiter {
	w := &Waiter{}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Waiter) record(err error) {
	w.mu.Lock()
	w.errs = append(w.errs, err)
	w.mu.Unlock()
	if w.onError != nil {
		w.onError(err)
	}
}

// Wait blocks until every task submitted with this waiter has finished and
// returns their errors joined, or nil. It may be called more than once.
func (w *Waiter) Wait() error {
	w.wg.Wait()
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.errs...)
}
