package parallel

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"pgregory.net/rapid"
)

// A random batch of tasks over random workers and waiters: every task runs
// exactly once, no more than workercount run at a time, each waiter gets
// back exactly the errors of its own failing tasks (and its handler sees
// each once), and Close and Wait can be called in any order and again.
//
// Inputs are random, schedules are not: run with -race and -count to shake
// the interleavings; the Gosched calls drawn per task help.
func TestPropertyManagerRunsEveryTaskOnceAndKeepsErrors(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		workers := rapid.IntRange(1, 8).Draw(rt, "workers")
		waiters := rapid.IntRange(1, 3).Draw(rt, "waiters")
		ntasks := rapid.IntRange(0, 64).Draw(rt, "tasks")

		type task struct {
			waiter int
			fails  bool
			yields int
		}
		tasks := make([]task, ntasks)
		for i := range tasks {
			tasks[i] = task{
				waiter: rapid.IntRange(0, waiters-1).Draw(rt, fmt.Sprintf("waiter%d", i)),
				fails:  rapid.Bool().Draw(rt, fmt.Sprintf("fails%d", i)),
				yields: rapid.IntRange(0, 3).Draw(rt, fmt.Sprintf("yields%d", i)),
			}
		}
		closeFirst := rapid.Bool().Draw(rt, "closeFirst")

		var (
			runs    = make([]int32, ntasks)
			running int32
			peak    int32
			handled = make([]int32, waiters)
			ws      = make([]*Waiter, waiters)
			errs    = make([]error, ntasks)
		)
		for i := range ws {
			i := i
			ws[i] = NewWaiter(WithErrorHandler(func(error) { atomic.AddInt32(&handled[i], 1) }))
		}

		m := New(workers)
		limit := cap(m.semaphore) // New raises small counts to minNumWorkers
		for i, tk := range tasks {
			i, tk := i, tk
			if tk.fails {
				errs[i] = fmt.Errorf("task %d", i)
			}
			m.Run(func() error {
				n := atomic.AddInt32(&running, 1)
				for {
					p := atomic.LoadInt32(&peak)
					if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
						break
					}
				}
				for j := 0; j < tk.yields; j++ {
					runtime.Gosched()
				}
				atomic.AddInt32(&runs[i], 1)
				atomic.AddInt32(&running, -1)
				return errs[i]
			}, ws[tk.waiter])
		}

		got := make([]error, waiters)
		wait := func() {
			var wg sync.WaitGroup
			for i := range ws {
				i := i
				wg.Add(1)
				go func() {
					defer wg.Done()
					got[i] = ws[i].Wait()
					ws[i].Wait() // idempotent
				}()
			}
			wg.Wait()
		}
		if closeFirst {
			m.Close()
			wait()
		} else {
			wait()
			m.Close()
		}
		m.Close() // idempotent

		if p := atomic.LoadInt32(&peak); int(p) > limit {
			rt.Fatalf("%d tasks ran at once, want at most %d", p, limit)
		}
		for i := range runs {
			if n := atomic.LoadInt32(&runs[i]); n != 1 {
				rt.Fatalf("task %d ran %d times", i, n)
			}
		}
		for w := 0; w < waiters; w++ {
			var want int32
			for i, tk := range tasks {
				if tk.waiter != w || !tk.fails {
					continue
				}
				want++
				if !errors.Is(got[w], errs[i]) {
					rt.Fatalf("waiter %d: Wait() = %v, missing %v", w, got[w], errs[i])
				}
			}
			if want == 0 && got[w] != nil {
				rt.Fatalf("waiter %d: Wait() = %v, want nil", w, got[w])
			}
			if h := atomic.LoadInt32(&handled[w]); h != want {
				rt.Fatalf("waiter %d: handler called %d times, want %d", w, h, want)
			}
			if got[w] != nil {
				if joined, ok := got[w].(interface{ Unwrap() []error }); !ok || int32(len(joined.Unwrap())) != want {
					rt.Fatalf("waiter %d: Wait() holds %v, want %d errors", w, got[w], want)
				}
			}
		}
	})
}
